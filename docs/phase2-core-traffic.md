# Phase 2: 核心流量链路

> 时间窗口：第 2–3 周 | 可验收产物：完整的路由转发 + 负载均衡 + 访问日志 + 基础指标

## 2.1 总体目标

在 Phase 1 脚手架之上，实现生产可用的路由引擎、负载均衡器和基础可观测性，形成 **接入 → 路由 → 转发 → 观测** 的闭环链路。

## 2.2 路由引擎设计

### 2.2.1 路由模型

```go
// Route 表示一条路由规则
type Route struct {
    Name     string            `yaml:"name"`
    Host     string            `yaml:"host"`               // 精确匹配或通配符 *.example.com
    Paths    []PathRule        `yaml:"paths"`
    Upstream string            `yaml:"upstream"`            // 上游名称引用
    Middlewares []string       `yaml:"middlewares,omitempty"`
}

// PathRule 路径匹配规则
type PathRule struct {
    Path     string   `yaml:"path"`      // /api/v1/users
    Type     string   `yaml:"type"`      // exact | prefix | regex
    Methods  []string `yaml:"methods"`   // GET, POST, ...
    Upstream string   `yaml:"upstream"`  // 可覆盖 Route 级上游
}
```

### 2.2.2 路由匹配优先级

> 参考 Shepherd 的多层路由机制，采用 **Map（哈希表精确匹配）+ Trie（前缀树模糊匹配）** 双层结构，实现毫秒级路径匹配。无变量的固定路径走 Map 直连路由（O(1)），带通配符/前缀的路径走 Trie 前缀路由。

```
1. Map 精确匹配：精确 Host + 精确 Path（最高优先级，O(1) 查找）
2. Trie 前缀匹配：精确 Host + 前缀 Path
3. Trie 通配符匹配：通配符 Host + 精确 Path
4. Trie 通配符前缀：通配符 Host + 前缀 Path
5. 默认路由（兜底 fallback）
```

### 2.2.3 Go 实现要点

> 采用 Shepherd 风格的 Map + Trie 双层路由结构：固定路径走哈希表 O(1) 查找，前缀/通配符路径走前缀树匹配。

```go
// trieNode 前缀树节点
// children 映射常规路径段（如 "api", "v1"），wildcard 处理动态/通配符段（如 ":id", "*"）
type trieNode struct {
    children map[string]*trieNode // 常规路径段子节点
    route    *Route               // 叶子节点挂载路由
    wildcard *trieNode            // 通配符/动态段子节点
}

// routerSnapshot 路由快照（不可变，通过 atomic.Pointer 切换）
// 请求热路径读取快照时无需加锁，消除 sync.RWMutex 读锁竞争
type routerSnapshot struct {
    exactMap   map[string]*Route   // host+path 精确匹配哈希表 O(1)
    prefixTrie *trieNode           // 前缀/通配符匹配前缀树
    hostIndex  map[string][]*Route // host -> routes 索引加速
}

// Router 核心路由器（Map + Trie 双层结构）
// 使用 atomic.Pointer 实现无锁读取，配置变更时整体替换快照
type Router struct {
    snapshot atomic.Pointer[routerSnapshot] // 无锁读取，热路径零竞争
}

// Reload 配置变更时构建新快照并原子替换（仅在配置热加载时调用，非热路径）
func (r *Router) Reload(routes []Route) {
    snap := &routerSnapshot{
        exactMap:   make(map[string]*Route),
        prefixTrie: &trieNode{children: make(map[string]*trieNode)},
        hostIndex:  make(map[string][]*Route),
    }
    for i := range routes {
        snap.addRoute(&routes[i])
    }
    r.snapshot.Store(snap)
}

// ServeHTTP 实现 http.Handler 接口
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    route := r.match(req.Host, req.URL.Path, req.Method)
    if route == nil {
        http.Error(w, "no matching route", http.StatusNotFound)
        return
    }
    // 获取上游并转发
    upstream := r.resolveUpstream(route)
    upstream.ServeHTTP(w, req)
}

// match 双层路由匹配：先查 Map 精确匹配，再查 Trie 前缀匹配
// 热路径优化：通过 atomic.Pointer 读取不可变快照，无锁无竞争
func (r *Router) match(host, path, method string) *Route {
    snap := r.snapshot.Load()
    if snap == nil {
        return nil
    }

    // 1. 精确匹配（O(1) 哈希查找）
    // 优化：使用预计算长度避免 string 拼接分配
    key := host + path
    if route, ok := snap.exactMap[key]; ok {
        if route.matchMethod(method) {
            return route
        }
    }

    // 2. 前缀树匹配（含 method 校验，实现见 router.go）
    return snap.prefixTrie.match(host, path, method)
}
```

## 2.3 上游管理与负载均衡

### 2.3.1 上游模型

```go
// Upstream 上游服务定义
type Upstream struct {
    Name       string    `yaml:"name"`
    Algorithm  string    `yaml:"algorithm"`  // round_robin | random | weighted
    Targets    []Target  `yaml:"targets"`
    HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty"`
}

// Target 上游实例
type Target struct {
    Address string `yaml:"address"` // host:port
    Weight  int    `yaml:"weight"`
    healthy int32  // atomic: 0=unhealthy, 1=healthy
}
```

### 2.3.2 负载均衡器接口

```go
// Balancer 负载均衡器接口
type Balancer interface {
    Pick(targets []*Target) *Target
}

// RoundRobinBalancer 轮询负载均衡
// 优化：防御性边界检查，避免 filterHealthy 返回后健康状态变化导致 index 越界
type RoundRobinBalancer struct {
    counter atomic.Uint64
}

func (b *RoundRobinBalancer) Pick(targets []*Target) *Target {
    healthy := filterHealthy(targets)
    n := uint64(len(healthy))
    if n == 0 {
        return nil
    }
    idx := b.counter.Add(1) % n
    // 防御性校验：并发场景下 healthy 切片可能已缩小
    if idx >= n {
        idx = 0
    }
    return healthy[idx]
}

// RandomBalancer 随机负载均衡
type RandomBalancer struct{}

func (b *RandomBalancer) Pick(targets []*Target) *Target {
    healthy := filterHealthy(targets)
    if len(healthy) == 0 {
        return nil
    }
    return healthy[rand.IntN(len(healthy))]
}
```

### 2.3.3 反向代理转发

> **热路径优化**：避免每次请求创建 `httputil.ReverseProxy` 对象（GC 压力），改为预创建代理实例 + `Rewrite` 动态修改目标地址。增加 semaphore 并发上限保护，防止突发流量耗尽资源。

```go
// ProxyHandler 封装 httputil.ReverseProxy
// 优化点：
//   1. 预创建 ReverseProxy，避免每次请求 new 对象（消除 GC 热点）
//   2. 使用 semaphore 限制最大并发转发数（背压保护）
type ProxyHandler struct {
    balancer  Balancer
    upstream  *Upstream
    proxy     *httputil.ReverseProxy  // 预创建，复用
    semaphore chan struct{}            // 并发上限信号量
}

func NewProxyHandler(balancer Balancer, upstream *Upstream, transport http.RoundTripper, maxConcurrent int) *ProxyHandler {
    p := &ProxyHandler{
        balancer:  balancer,
        upstream:  upstream,
        semaphore: make(chan struct{}, maxConcurrent),
    }
    p.proxy = &httputil.ReverseProxy{
        Rewrite: func(pr *httputil.ProxyRequest) {
            target := pr.Out.Context().Value(targetKey).(*Target)
            pr.SetURL(&url.URL{
                Scheme: "http",
                Host:   target.Address,
            })
            pr.Out.Host = target.Address
        },
        Transport:    transport,
        ErrorHandler: p.handleError,
    }
    return p
}

func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    target := p.balancer.Pick(p.upstream.Targets)
    if target == nil {
        http.Error(w, "no healthy upstream", http.StatusServiceUnavailable)
        return
    }

    // 背压：超过并发上限时快速拒绝
    select {
    case p.semaphore <- struct{}{}:
        defer func() { <-p.semaphore }()
    default:
        http.Error(w, "service overloaded", http.StatusServiceUnavailable)
        return
    }

    // 将 target 注入 context，供 Rewrite 回调使用
    ctx := context.WithValue(r.Context(), targetKey, target)
    p.proxy.ServeHTTP(w, r.WithContext(ctx))
}
```

## 2.4 访问日志

### 2.4.1 日志格式

采用结构化 JSON 日志，字段对齐 Envoy 访问日志格式：

```go
// AccessLog 访问日志字段
type AccessLog struct {
    Timestamp     string `json:"timestamp"`
    RequestID     string `json:"request_id"`
    Method        string `json:"method"`
    Path          string `json:"path"`
    Host          string `json:"host"`
    StatusCode    int    `json:"status_code"`
    Latency       string `json:"latency"`
    BytesSent     int64  `json:"bytes_sent"`
    BytesReceived int64  `json:"bytes_received"`
    UserAgent     string `json:"user_agent"`
    RemoteAddr    string `json:"remote_addr"`
    Upstream      string `json:"upstream"`
    TraceID       string `json:"trace_id,omitempty"`
}
```

### 2.4.2 日志中间件

```go
func LoggingMiddleware(logger *slog.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            requestID := r.Header.Get("X-Request-ID")
            if requestID == "" {
                requestID = generateRequestID()
            }

            // 包装 ResponseWriter 以捕获状态码
            wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

            next.ServeHTTP(wrapped, r)

            logger.Info("access",
                slog.String("request_id", requestID),
                slog.String("method", r.Method),
                slog.String("path", r.URL.Path),
                slog.Int("status", wrapped.statusCode),
                slog.Duration("latency", time.Since(start)),
                slog.String("remote_addr", r.RemoteAddr),
            )
        })
    }
}
```

## 2.5 基础指标

### 2.5.1 核心指标定义

```go
var (
    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nexus_requests_total",
            Help: "Total number of requests processed",
        },
        // 使用 route 名称替代 path，防止动态路径导致基数爆炸
        []string{"method", "route", "status", "upstream"},
    )

    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "nexus_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "route", "upstream"},
    )

    upstreamHealthGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "nexus_upstream_health",
            Help: "Upstream target health status (1=healthy, 0=unhealthy)",
        },
        []string{"upstream", "target"},
    )

    activeConnections = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "nexus_active_connections",
            Help: "Number of active connections",
        },
    )
)
```

### 2.5.2 指标暴露端点

```go
// /metrics 端点，Prometheus 标准格式
mux.Handle("/metrics", promhttp.Handler())
```

## 2.6 配置文件示例

```yaml
# configs/nexus.yaml
server:
  listen: ":8080"
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

upstreams:
  - name: user-service
    algorithm: round_robin
    targets:
      - address: "127.0.0.1:9001"
        weight: 1
      - address: "127.0.0.1:9002"
        weight: 1
    health_check:
      interval: 10s
      timeout: 3s
      path: /healthz

  - name: order-service
    algorithm: round_robin
    targets:
      - address: "127.0.0.1:9003"
        weight: 1

routes:
  - name: user-api
    host: "api.example.com"
    paths:
      - path: /api/v1/users
        type: prefix
        methods: [GET, POST, PUT, DELETE]
    upstream: user-service

  - name: order-api
    host: "api.example.com"
    paths:
      - path: /api/v1/orders
        type: prefix
        methods: [GET, POST]
    upstream: order-service

logging:
  level: info
  format: json

metrics:
  enabled: true
  path: /metrics
```

## 2.7 验收标准

| 验收项 | 标准 | 验证方式 |
|--------|------|----------|
| Host/Path 路由 | 不同路径路由到不同上游，无匹配返回 404 | 多路由配置 + curl 验证 |
| 负载均衡 | Round-Robin 算法，请求均匀分布到上游实例 | 多次请求后检查各实例请求数 |
| 健康摘除 | 不健康实例不再接收请求 | 关闭一个上游实例，验证请求不再发往该实例 |
| 访问日志 | 每条请求产生结构化日志，包含 request_id | 检查 stdout 日志 |
| Prometheus 指标 | `/metrics` 端点可被 Prometheus 抓取 | `curl /metrics` 验证指标存在 |
| 配置热加载 | 修改 YAML 后路由表更新，无需重启 | 修改配置文件，验证新路由生效 |

## 2.8 性能基线

Phase 2 完成后应进行基准压测，建立性能基线：

```bash
# 使用 hey 进行压测
hey -n 100000 -c 200 http://localhost:8080/api/v1/users
```

关注指标：
- P50 / P95 / P99 延迟
- 最大 RPS
- CPU / Memory 使用
- goroutine 数量
