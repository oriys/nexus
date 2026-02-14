# Phase 3: 安全与稳定性

> 时间窗口：第 3–4 周 | 可验收产物：TLS 终止 + JWT/API Key 鉴权 + 限流 + 超时/重试/熔断

## 3.1 总体目标

在核心流量链路之上叠加安全控制与稳定性保护，确保生产环境的 **接入安全、访问控制、流量整形和故障隔离** 能力。

## 3.2 TLS 终止

### 3.2.1 设计方案

```go
// TLSConfig TLS 配置模型
type TLSConfig struct {
    Enabled    bool   `yaml:"enabled"`
    CertFile   string `yaml:"cert_file"`
    KeyFile    string `yaml:"key_file"`
    MinVersion string `yaml:"min_version"` // tls1.2 | tls1.3
    AutoReload bool   `yaml:"auto_reload"` // 证书文件变更自动重载
}
```

### 3.2.2 证书热更新

利用 `tls.Config.GetCertificate` 回调实现证书动态加载，无需重启：

> **热路径优化**：TLS 握手在每个新连接上触发 `GetCertificate`，使用 `atomic.Pointer` 替代 `sync.RWMutex`，消除读锁竞争。

```go
type CertManager struct {
    cert     atomic.Pointer[tls.Certificate] // 无锁读取，TLS 握手热路径零竞争
    certFile string
    keyFile  string
}

func (cm *CertManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
    return cm.cert.Load(), nil // 无锁读取
}

func (cm *CertManager) Reload() error {
    cert, err := tls.LoadX509KeyPair(cm.certFile, cm.keyFile)
    if err != nil {
        return fmt.Errorf("failed to load certificate: %w", err)
    }
    cm.cert.Store(&cert) // 原子写入
    return nil
}

// 创建 TLS 服务器
func newTLSServer(addr string, handler http.Handler, cm *CertManager) *http.Server {
    return &http.Server{
        Addr:    addr,
        Handler: handler,
        TLSConfig: &tls.Config{
            GetCertificate: cm.GetCertificate,
            MinVersion:     tls.VersionTLS12,
        },
    }
}
```

### 3.2.3 HTTP → HTTPS 重定向

```go
func HTTPSRedirectHandler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        target := "https://" + r.Host + r.RequestURI
        http.Redirect(w, r, target, http.StatusMovedPermanently)
    })
}
```

## 3.3 认证与鉴权

### 3.3.1 认证接口抽象

```go
// Authenticator 认证器接口
type Authenticator interface {
    Authenticate(r *http.Request) (*Identity, error)
}

// Identity 认证身份信息
type Identity struct {
    Subject  string            // 主体标识 (sub)
    Claims   map[string]any    // JWT claims 或其他属性
    Source   string            // jwt | apikey
}
```

### 3.3.2 JWT 校验

```go
type JWTAuthenticator struct {
    keyFunc    jwt.Keyfunc
    issuer     string
    audiences  []string
}

func (j *JWTAuthenticator) Authenticate(r *http.Request) (*Identity, error) {
    tokenStr := extractBearerToken(r)
    if tokenStr == "" {
        return nil, ErrMissingToken
    }

    token, err := jwt.Parse(tokenStr, j.keyFunc,
        jwt.WithIssuer(j.issuer),
        jwt.WithAudience(j.audiences[0]),
        jwt.WithValidMethods([]string{"RS256", "ES256"}),
    )
    if err != nil {
        return nil, fmt.Errorf("invalid token: %w", err)
    }

    claims := token.Claims.(jwt.MapClaims)
    return &Identity{
        Subject: claims["sub"].(string),
        Claims:  claims,
        Source:  "jwt",
    }, nil
}

func extractBearerToken(r *http.Request) string {
    auth := r.Header.Get("Authorization")
    if !strings.HasPrefix(auth, "Bearer ") {
        return ""
    }
    return strings.TrimPrefix(auth, "Bearer ")
}
```

### 3.3.3 API Key 校验

```go
type APIKeyAuthenticator struct {
    keys map[string]*Consumer // key -> consumer 映射
}

func (a *APIKeyAuthenticator) Authenticate(r *http.Request) (*Identity, error) {
    key := r.Header.Get("X-API-Key")
    if key == "" {
        key = r.URL.Query().Get("api_key")
    }
    if key == "" {
        return nil, ErrMissingAPIKey
    }

    consumer, ok := a.keys[key]
    if !ok {
        return nil, ErrInvalidAPIKey
    }

    return &Identity{
        Subject: consumer.Name,
        Claims:  map[string]any{"consumer": consumer.Name, "tier": consumer.Tier},
        Source:  "apikey",
    }, nil
}
```

### 3.3.4 认证中间件

```go
func AuthMiddleware(authenticator Authenticator) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            identity, err := authenticator.Authenticate(r)
            if err != nil {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusUnauthorized)
                json.NewEncoder(w).Encode(map[string]string{
                    "error":   "unauthorized",
                    "message": err.Error(),
                })
                return
            }

            // 将身份信息注入 context，供下游中间件/路由使用
            ctx := context.WithValue(r.Context(), identityKey, identity)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

## 3.4 限流

### 3.4.1 限流模型

```go
// RateLimitConfig 限流配置
type RateLimitConfig struct {
    Enabled  bool   `yaml:"enabled"`
    Rate     int    `yaml:"rate"`       // 每窗口允许请求数
    Window   string `yaml:"window"`     // 1m | 1h | 1s
    KeyFunc  string `yaml:"key_func"`   // client_ip | consumer | api_key | header:X-Tenant
    Strategy string `yaml:"strategy"`   // local | (future: distributed)
}
```

### 3.4.2 本地滑动窗口限流器

> **热路径优化**：原始设计使用单个 `sync.Mutex` 保护所有 key 的限流窗口，在高并发场景下成为单点瓶颈。改用分片锁（sharded lock）设计：按 key 哈希分配到 N 个桶，每桶独立加锁，将锁竞争降低为 1/N。

```go
const numShards = 256 // 分片数，2 的幂次便于位运算

// ShardedSlidingWindowLimiter 分片滑动窗口限流器
// 按 key 哈希分配到独立分片，消除全局锁竞争瓶颈
type ShardedSlidingWindowLimiter struct {
    shards [numShards]shard
    rate   int
    window time.Duration
}

type shard struct {
    mu      sync.Mutex
    windows map[string]*window
}

type window struct {
    count     int
    prevCount int
    currStart time.Time
}

func NewShardedSlidingWindowLimiter(rate int, window time.Duration) *ShardedSlidingWindowLimiter {
    l := &ShardedSlidingWindowLimiter{rate: rate, window: window}
    for i := range l.shards {
        l.shards[i].windows = make(map[string]*window)
    }
    return l
}

// getShard 根据 key 哈希选择分片（FNV-1a，位运算取模）
func (l *ShardedSlidingWindowLimiter) getShard(key string) *shard {
    h := fnv32a(key)
    return &l.shards[h&(numShards-1)]
}

func (l *ShardedSlidingWindowLimiter) Allow(key string) bool {
    s := l.getShard(key)
    s.mu.Lock()
    defer s.mu.Unlock()

    now := time.Now()
    w, ok := s.windows[key]
    if !ok {
        w = &window{currStart: now}
        s.windows[key] = w
    }

    // 窗口滑动
    elapsed := now.Sub(w.currStart)
    if elapsed >= l.window {
        w.prevCount = w.count
        w.count = 0
        w.currStart = now
        elapsed = 0
    }

    // 加权计算：前一窗口的剩余权重 + 当前窗口计数
    weight := 1.0 - float64(elapsed)/float64(l.window)
    estimate := float64(w.prevCount)*weight + float64(w.count)

    if estimate >= float64(l.rate) {
        return false
    }

    w.count++
    return true
}

// fnv32a 快速哈希（内联友好，无堆分配）
func fnv32a(s string) uint32 {
    const offset32 = 2166136261
    const prime32  = 16777619
    h := uint32(offset32)
    for i := 0; i < len(s); i++ {
        h ^= uint32(s[i])
        h *= prime32
    }
    return h
}
```

### 3.4.3 限流中间件

```go
func RateLimitMiddleware(limiter *ShardedSlidingWindowLimiter, keyFunc KeyExtractor) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            key := keyFunc(r)

            if !limiter.Allow(key) {
                w.Header().Set("Content-Type", "application/json")
                w.Header().Set("Retry-After", "60")
                w.WriteHeader(http.StatusTooManyRequests)
                json.NewEncoder(w).Encode(map[string]string{
                    "error":   "rate_limit_exceeded",
                    "message": "Too many requests, please try again later",
                })
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}

// KeyExtractor 限流键提取函数
type KeyExtractor func(r *http.Request) string

func ClientIPKeyExtractor(r *http.Request) string {
    return r.RemoteAddr
}

func ConsumerKeyExtractor(r *http.Request) string {
    if id, ok := r.Context().Value(identityKey).(*Identity); ok {
        return id.Subject
    }
    return r.RemoteAddr
}
```

## 3.5 超时、重试与熔断

### 3.5.1 超时配置

```go
// TimeoutConfig 超时配置
type TimeoutConfig struct {
    Connect time.Duration `yaml:"connect"` // 连接超时
    Read    time.Duration `yaml:"read"`    // 读取超时
    Write   time.Duration `yaml:"write"`   // 写入超时
    Idle    time.Duration `yaml:"idle"`    // 空闲超时
}

// 应用到 http.Transport
// 优化：增加 MaxConnsPerHost 防止单一上游耗尽连接资源
func newTransport(cfg TimeoutConfig) *http.Transport {
    return &http.Transport{
        DialContext: (&net.Dialer{
            Timeout: cfg.Connect,
        }).DialContext,
        ResponseHeaderTimeout: cfg.Read,
        IdleConnTimeout:       cfg.Idle,
        MaxIdleConnsPerHost:   100,
        MaxConnsPerHost:       200,  // 单上游最大连接数上限，防止连接耗尽
    }
}
```

### 3.5.2 重试机制

```go
// RetryConfig 重试配置
type RetryConfig struct {
    MaxRetries     int           `yaml:"max_retries"`     // 最大重试次数
    RetryOn        []int         `yaml:"retry_on"`        // 重试的 HTTP 状态码 [502, 503, 504]
    Backoff        time.Duration `yaml:"backoff"`         // 退避间隔
    IdempotentOnly bool          `yaml:"idempotent_only"` // 仅对幂等请求重试
}

func isIdempotent(method string) bool {
    switch method {
    case http.MethodGet, http.MethodHead, http.MethodOptions,
        http.MethodPut, http.MethodDelete:
        return true
    }
    return false
}

func shouldRetry(cfg RetryConfig, method string, statusCode int) bool {
    if cfg.IdempotentOnly && !isIdempotent(method) {
        return false
    }
    for _, code := range cfg.RetryOn {
        if statusCode == code {
            return true
        }
    }
    return false
}
```

### 3.5.3 熔断器

采用经典的三态模型（Closed → Open → Half-Open）：

```go
type CircuitState int

const (
    StateClosed   CircuitState = iota // 正常
    StateOpen                         // 熔断（快速失败）
    StateHalfOpen                     // 半开（试探）
)

// CircuitBreaker 熔断器
// 优化：增加状态变化回调，发出可观测指标
type CircuitBreaker struct {
    mu               sync.Mutex
    state            CircuitState
    failureCount     int
    successCount     int
    failureThreshold int           // 触发熔断的失败次数
    successThreshold int           // 半开态恢复的成功次数
    timeout          time.Duration // 熔断持续时间
    lastFailure      time.Time
    name             string        // 上游名称，用于指标标签
    onStateChange    func(name string, from, to CircuitState) // 状态变化回调
}

func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    switch cb.state {
    case StateClosed:
        return true
    case StateOpen:
        if time.Since(cb.lastFailure) > cb.timeout {
            cb.transition(StateHalfOpen)
            cb.successCount = 0
            return true
        }
        return false
    case StateHalfOpen:
        return true
    }
    return false
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if cb.state == StateHalfOpen {
        cb.successCount++
        if cb.successCount >= cb.successThreshold {
            cb.transition(StateClosed)
            cb.failureCount = 0
        }
    }
    cb.failureCount = 0
}

func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.failureCount++
    cb.lastFailure = time.Now()

    if cb.failureCount >= cb.failureThreshold {
        cb.transition(StateOpen)
    }
}

// transition 状态转换 + 回调通知（用于发出 Prometheus 指标）
func (cb *CircuitBreaker) transition(to CircuitState) {
    from := cb.state
    cb.state = to
    if cb.onStateChange != nil {
        cb.onStateChange(cb.name, from, to)
    }
}
```

## 3.6 健康检查

### 3.6.1 主动健康检查

```go
// HealthChecker 主动健康检查器
type HealthChecker struct {
    interval time.Duration
    timeout  time.Duration
    path     string
    client   *http.Client
}

func (hc *HealthChecker) Start(ctx context.Context, targets []*Target) {
    ticker := time.NewTicker(hc.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for _, target := range targets {
                go hc.check(target)
            }
        }
    }
}

func (hc *HealthChecker) check(target *Target) {
    url := fmt.Sprintf("http://%s%s", target.Address, hc.path)

    ctx, cancel := context.WithTimeout(context.Background(), hc.timeout)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    resp, err := hc.client.Do(req)

    if err != nil || resp.StatusCode >= 500 {
        atomic.StoreInt32(&target.healthy, 0)
        return
    }
    resp.Body.Close()
    atomic.StoreInt32(&target.healthy, 1)
}
```

### 3.6.2 被动健康检查

```go
// PassiveHealthCheck 被动健康检查（基于请求结果）
type PassiveHealthCheck struct {
    failureThreshold int
    window           time.Duration
    failures         map[string]*failureRecord
    mu               sync.Mutex
}

type failureRecord struct {
    count     int
    firstFail time.Time
}

func (phc *PassiveHealthCheck) RecordResult(target *Target, statusCode int) {
    if statusCode < 500 {
        phc.reset(target.Address)
        return
    }

    phc.mu.Lock()
    defer phc.mu.Unlock()

    rec, ok := phc.failures[target.Address]
    if !ok {
        rec = &failureRecord{firstFail: time.Now()}
        phc.failures[target.Address] = rec
    }

    if time.Since(rec.firstFail) > phc.window {
        rec.count = 0
        rec.firstFail = time.Now()
    }

    rec.count++
    if rec.count >= phc.failureThreshold {
        atomic.StoreInt32(&target.healthy, 0)
    }
}
```

## 3.7 配置示例（安全与稳定性）

```yaml
tls:
  enabled: true
  cert_file: /etc/nexus/certs/tls.crt
  key_file: /etc/nexus/certs/tls.key
  min_version: tls1.2
  auto_reload: true

auth:
  jwt:
    enabled: true
    issuer: "https://auth.example.com"
    audiences: ["nexus-gateway"]
    jwks_url: "https://auth.example.com/.well-known/jwks.json"
  api_key:
    enabled: true
    header: "X-API-Key"

rate_limit:
  enabled: true
  rate: 100
  window: 1m
  key_func: consumer
  strategy: local

resilience:
  timeout:
    connect: 5s
    read: 30s
    write: 30s
  retry:
    max_retries: 2
    retry_on: [502, 503, 504]
    backoff: 100ms
    idempotent_only: true
  circuit_breaker:
    failure_threshold: 5
    success_threshold: 3
    timeout: 30s
  health_check:
    interval: 10s
    timeout: 3s
    path: /healthz
```

## 3.8 中间件执行顺序

安全与稳定性中间件应按如下顺序执行：

```
请求入站 → TLS 终止
  → Request ID 注入
  → 访问日志（开始计时）
  → 指标采集（开始）
  → 限流检查（429 快速拒绝）
  → 认证（401 快速拒绝）
  → 路由匹配
  → 熔断检查（503 快速失败）
  → 超时控制
  → 反向代理（含重试逻辑）
  → 被动健康检查（记录结果）
  → 指标采集（结束）
  → 访问日志（记录完成）
响应出站 ←
```

## 3.9 验收标准

| 验收项 | 标准 | 验证方式 |
|--------|------|----------|
| TLS 终止 | HTTPS 可用，证书可替换 | 用测试证书部署，`curl --cacert` 验证 |
| 证书热更新 | 替换证书文件后新连接使用新证书 | 替换证书，验证新握手使用新证书 |
| JWT 鉴权 | 无效/过期 token 返回 401 | 构造三组 token（过期/错误签名/合法）测试 |
| API Key | 无效 key 返回 401，有效 key 放行 | 构造无效和有效 key 测试 |
| 限流 | 超额返回 429，包含 Retry-After | 爆发请求触发 429 |
| 超时 | 上游超时时返回 504 | 模拟慢上游 |
| 重试 | 上游 503 时重试，达到上限后返回错误 | 模拟间歇性上游故障 |
| 熔断 | 连续失败后快速失败，恢复后重新放行 | 模拟上游故障→恢复序列 |
| 健康检查 | 不健康实例被摘除 | 关闭上游实例，验证流量不再发往 |
