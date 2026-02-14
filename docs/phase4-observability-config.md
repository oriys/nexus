# Phase 4: 可观测性与配置管理

> 时间窗口：第 4–5 周 | 可验收产物：完整可观测体系 + 配置管理闭环 + Admin API

## 4.1 总体目标

构建完整的可观测性体系（日志 / 指标 / 追踪），实现配置管理的声明式闭环（校验 → 加载 → 生效 → 回滚），并提供可选的 Admin API 用于运行时管理。

## 4.2 可观测性体系

### 4.2.1 三大信号关联

```
┌─────────────────────────────────────────────────────┐
│                  可观测性体系                         │
│                                                     │
│  ┌──────────┐   ┌──────────┐   ┌──────────────┐    │
│  │  Logs    │   │ Metrics  │   │   Traces     │    │
│  │ (slog)  │   │(Prometheus)│  │(OpenTelemetry)│   │
│  └────┬─────┘   └────┬─────┘   └──────┬───────┘    │
│       │              │                │             │
│       └──────────────┼────────────────┘             │
│                      │                              │
│              request_id / trace_id                   │
│              （统一关联标识）                          │
└─────────────────────────────────────────────────────┘
```

### 4.2.2 结构化日志增强

```go
// 日志上下文注入
type LogContext struct {
    RequestID  string `json:"request_id"`
    TraceID    string `json:"trace_id,omitempty"`
    SpanID     string `json:"span_id,omitempty"`
    Method     string `json:"method"`
    Path       string `json:"path"`
    Host       string `json:"host"`
    RemoteAddr string `json:"remote_addr"`
    UserAgent  string `json:"user_agent"`
    Consumer   string `json:"consumer,omitempty"`
}

// RequestIDMiddleware 注入或提取 Request-ID
func RequestIDMiddleware() Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            requestID := r.Header.Get("X-Request-ID")
            if requestID == "" {
                requestID = uuid.New().String()
            }

            // 设置响应头
            w.Header().Set("X-Request-ID", requestID)

            // 注入 context
            ctx := context.WithValue(r.Context(), requestIDKey, requestID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### 4.2.3 Prometheus 指标体系

```go
// MetricsRegistry 指标注册中心
type MetricsRegistry struct {
    // 流量指标
    RequestsTotal    *prometheus.CounterVec
    RequestDuration  *prometheus.HistogramVec
    RequestSize      *prometheus.HistogramVec
    ResponseSize     *prometheus.HistogramVec

    // 上游指标
    UpstreamRequests  *prometheus.CounterVec
    UpstreamDuration  *prometheus.HistogramVec
    UpstreamHealth    *prometheus.GaugeVec

    // 稳定性指标
    RateLimitHits     *prometheus.CounterVec
    CircuitBreakerState *prometheus.GaugeVec
    RetryTotal        *prometheus.CounterVec

    // 安全指标
    AuthFailures      *prometheus.CounterVec

    // 系统指标
    ActiveConnections *prometheus.Gauge
    ConfigReloads     *prometheus.CounterVec
}

func NewMetricsRegistry() *MetricsRegistry {
    return &MetricsRegistry{
        RequestsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: "nexus",
                Name:      "requests_total",
                Help:      "Total number of HTTP requests",
            },
            []string{"method", "route", "status_code"},
        ),
        RequestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: "nexus",
                Name:      "request_duration_seconds",
                Help:      "HTTP request duration in seconds",
                Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "route"},
        ),
        RateLimitHits: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: "nexus",
                Name:      "rate_limit_hits_total",
                Help:      "Total number of rate-limited requests (429)",
            },
            []string{"route", "key"},
        ),
        AuthFailures: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: "nexus",
                Name:      "auth_failures_total",
                Help:      "Total number of authentication failures",
            },
            []string{"reason"}, // expired, invalid_signature, missing_token
        ),
        ConfigReloads: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: "nexus",
                Name:      "config_reloads_total",
                Help:      "Total number of configuration reloads",
            },
            []string{"result"}, // success, failure
        ),
    }
}
```

### 4.2.4 Trace Context 透传

MVP 阶段先实现 trace header 透传（W3C Trace Context），后续再做自动 span 生成：

```go
// TraceContext W3C Trace Context 透传
var traceHeaders = []string{
    "traceparent",
    "tracestate",
    "X-Request-ID",
    "X-B3-TraceId",
    "X-B3-SpanId",
    "X-B3-ParentSpanId",
    "X-B3-Sampled",
}

func TraceContextMiddleware() Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 确保 traceparent 存在
            if r.Header.Get("traceparent") == "" {
                traceID := generateTraceID()
                spanID := generateSpanID()
                r.Header.Set("traceparent",
                    fmt.Sprintf("00-%s-%s-01", traceID, spanID))
            }

            // 将 trace 信息注入 context 供日志使用
            traceID := extractTraceID(r.Header.Get("traceparent"))
            ctx := context.WithValue(r.Context(), traceIDKey, traceID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

## 4.3 配置管理

### 4.3.1 配置生命周期

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│  编辑    │───▶│  校验    │───▶│  加载    │───▶│  生效    │
│ (YAML)   │    │ (Schema) │    │ (atomic) │    │ (路由表) │
└──────────┘    └────┬─────┘    └──────────┘    └──────────┘
                     │ 失败                          │
                     ▼                              │
                ┌──────────┐                   ┌────▼─────┐
                │ 拒绝加载 │                   │ 版本记录 │
                │ + 告警   │                   │ (回滚用) │
                └──────────┘                   └──────────┘
```

### 4.3.2 配置加载器

```go
// ConfigLoader 配置加载器
type ConfigLoader struct {
    path       string
    current    atomic.Value  // *GatewayConfig
    versions   []ConfigVersion
    maxHistory int
    watcher    *fsnotify.Watcher
    onChange   []func(*GatewayConfig)
    logger     *slog.Logger
}

// GatewayConfig 网关全局配置
type GatewayConfig struct {
    Server     ServerConfig     `yaml:"server"`
    TLS        TLSConfig        `yaml:"tls"`
    Upstreams  []Upstream       `yaml:"upstreams"`
    Routes     []Route          `yaml:"routes"`
    Auth       AuthConfig       `yaml:"auth"`
    RateLimit  RateLimitConfig  `yaml:"rate_limit"`
    Resilience ResilienceConfig `yaml:"resilience"`
    Logging    LoggingConfig    `yaml:"logging"`
    Metrics    MetricsConfig    `yaml:"metrics"`
}

// ConfigVersion 配置版本记录
type ConfigVersion struct {
    Version   int
    Hash      string
    Timestamp time.Time
    Config    *GatewayConfig
}

func (cl *ConfigLoader) Load() error {
    data, err := os.ReadFile(cl.path)
    if err != nil {
        return fmt.Errorf("read config file: %w", err)
    }

    var cfg GatewayConfig
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return fmt.Errorf("parse config: %w", err)
    }

    if err := cl.validate(&cfg); err != nil {
        return fmt.Errorf("validate config: %w", err)
    }

    // 记录版本
    cl.saveVersion(&cfg, data)

    // 原子替换
    cl.current.Store(&cfg)

    // 触发回调
    for _, fn := range cl.onChange {
        fn(&cfg)
    }

    cl.logger.Info("config loaded",
        slog.Int("version", len(cl.versions)),
        slog.String("hash", cl.versions[len(cl.versions)-1].Hash),
    )
    return nil
}

func (cl *ConfigLoader) Rollback() error {
    if len(cl.versions) < 2 {
        return fmt.Errorf("no previous version to rollback to")
    }
    prev := cl.versions[len(cl.versions)-2]
    cl.current.Store(prev.Config)
    cl.logger.Info("config rolled back",
        slog.Int("to_version", prev.Version),
    )
    return nil
}
```

### 4.3.3 配置校验

```go
// ConfigValidator 配置校验器
type ConfigValidator struct{}

func (v *ConfigValidator) Validate(cfg *GatewayConfig) error {
    var errs []error

    // 路由引用的上游必须存在
    upstreamNames := make(map[string]bool)
    for _, u := range cfg.Upstreams {
        if upstreamNames[u.Name] {
            errs = append(errs, fmt.Errorf("duplicate upstream name: %s", u.Name))
        }
        upstreamNames[u.Name] = true
    }

    for _, r := range cfg.Routes {
        if r.Upstream != "" && !upstreamNames[r.Upstream] {
            errs = append(errs, fmt.Errorf(
                "route %q references unknown upstream %q", r.Name, r.Upstream))
        }
    }

    // TLS 配置校验
    if cfg.TLS.Enabled {
        if cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "" {
            errs = append(errs, fmt.Errorf("TLS enabled but cert_file or key_file is empty"))
        }
    }

    // 限流配置校验
    if cfg.RateLimit.Enabled && cfg.RateLimit.Rate <= 0 {
        errs = append(errs, fmt.Errorf("rate_limit.rate must be positive"))
    }

    return errors.Join(errs...)
}
```

### 4.3.4 文件监听热加载

```go
func (cl *ConfigLoader) Watch(ctx context.Context) error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }
    cl.watcher = watcher

    go func() {
        defer watcher.Close()
        for {
            select {
            case <-ctx.Done():
                return
            case event := <-watcher.Events:
                if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
                    cl.logger.Info("config file changed, reloading",
                        slog.String("file", event.Name))
                    if err := cl.Load(); err != nil {
                        cl.logger.Error("config reload failed",
                            slog.String("error", err.Error()))
                    }
                }
            case err := <-watcher.Errors:
                cl.logger.Error("config watcher error",
                    slog.String("error", err.Error()))
            }
        }
    }()

    return watcher.Add(cl.path)
}
```

## 4.4 Admin API（可选）

### 4.4.1 API 设计

```go
// AdminServer 管理面 API 服务器
type AdminServer struct {
    configLoader *ConfigLoader
    metrics      *MetricsRegistry
    mux          *http.ServeMux
}

func NewAdminServer(cl *ConfigLoader, m *MetricsRegistry) *AdminServer {
    s := &AdminServer{configLoader: cl, metrics: m}
    s.mux = http.NewServeMux()
    s.registerRoutes()
    return s
}

func (s *AdminServer) registerRoutes() {
    s.mux.HandleFunc("GET /api/v1/config", s.getConfig)
    s.mux.HandleFunc("GET /api/v1/config/versions", s.listVersions)
    s.mux.HandleFunc("POST /api/v1/config/rollback", s.rollbackConfig)
    s.mux.HandleFunc("GET /api/v1/routes", s.listRoutes)
    s.mux.HandleFunc("GET /api/v1/upstreams", s.listUpstreams)
    s.mux.HandleFunc("GET /api/v1/upstreams/{name}/health", s.getUpstreamHealth)
    s.mux.HandleFunc("GET /api/v1/status", s.getStatus)
}
```

### 4.4.2 API 端点说明

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/config` | 获取当前生效配置 |
| GET | `/api/v1/config/versions` | 列出配置版本历史 |
| POST | `/api/v1/config/rollback` | 回滚到上一版本 |
| GET | `/api/v1/routes` | 列出所有路由规则 |
| GET | `/api/v1/upstreams` | 列出所有上游服务 |
| GET | `/api/v1/upstreams/{name}/health` | 查看指定上游的健康状态 |
| GET | `/api/v1/status` | 网关运行状态摘要 |

## 4.5 Grafana Dashboard 模板

MVP 阶段提供最小仪表盘模板，包含以下面板：

```
┌─────────────────────────────────────────────────────────┐
│                   Nexus Gateway Dashboard                │
├──────────────────────┬──────────────────────────────────┤
│  Total RPS          │  Error Rate (4xx/5xx)            │
│  [实时折线图]        │  [实时折线图]                    │
├──────────────────────┼──────────────────────────────────┤
│  P50/P95/P99 延迟   │  Active Connections              │
│  [分位数折线图]      │  [仪表盘]                        │
├──────────────────────┼──────────────────────────────────┤
│  Rate Limit 429s    │  Auth Failures                   │
│  [计数器]            │  [计数器]                        │
├──────────────────────┼──────────────────────────────────┤
│  Upstream Health     │  Circuit Breaker State           │
│  [状态矩阵]          │  [状态指示器]                    │
├──────────────────────┴──────────────────────────────────┤
│  Per-Route Request Rate [按路由维度的 RPS 折线图]         │
└─────────────────────────────────────────────────────────┘
```

## 4.6 告警规则模板

```yaml
# Prometheus 告警规则示例
groups:
  - name: nexus-gateway
    rules:
      - alert: HighErrorRate
        expr: |
          sum(rate(nexus_requests_total{status_code=~"5.."}[5m]))
          / sum(rate(nexus_requests_total[5m])) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate (>5%) on Nexus Gateway"

      - alert: HighLatency
        expr: |
          histogram_quantile(0.99,
            rate(nexus_request_duration_seconds_bucket[5m])) > 2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P99 latency > 2s on Nexus Gateway"

      - alert: RateLimitSpike
        expr: |
          sum(rate(nexus_rate_limit_hits_total[5m])) > 100
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High rate of rate-limited requests"
```

## 4.7 验收标准

| 验收项 | 标准 | 验证方式 |
|--------|------|----------|
| 日志关联 | 日志包含 request_id 和 trace_id | 检查日志输出 |
| Prometheus 指标 | 所有核心指标可被 Prometheus 抓取 | `curl /metrics` |
| Trace 透传 | traceparent header 在上下游一致 | 检查上游收到的 header |
| 配置校验 | 无效配置被拒绝，不影响运行 | 提交错误配置验证 |
| 配置热加载 | 修改文件后自动重载，不中断服务 | 修改文件，验证路由变化 |
| 配置回滚 | 回滚到上一版本后恢复正常 | 触发回滚验证 |
| Admin API | 端点可用并返回正确数据 | curl 各端点验证 |
| 仪表盘 | Grafana 模板可导入并展示数据 | 导入模板验证面板 |
