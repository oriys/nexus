# Phase 1: 架构定版与技术选型

> 时间窗口：第 1–2 周 | 可验收产物：架构设计文档 + 项目脚手架 + 基础路由 PoC

## 1.1 总体目标

确定数据面与控制面的技术选型，完成 Go 项目脚手架搭建，并以一个最小的 HTTP 反向代理 PoC 验证核心链路可行性。

## 1.2 技术栈选型

| 层面 | 选型 | 理由 |
|------|------|------|
| 编程语言 | Go 1.24+ | 高性能网络编程、goroutine 天然并发、标准库 `net/http` 成熟、静态编译部署简单；1.24 引入 Swiss Table 优化 map 性能、泛型类型别名、`crypto/mlkem` 后量子密码学、`go.mod` tool 指令、`os.Root` 目录隔离等增强 |
| 数据面代理 | 自研（基于 `net/http/httputil.ReverseProxy`） | MVP 阶段可控，后续可切换至 Envoy xDS 或嵌入式 NGINX |
| 配置格式 | YAML + JSON Schema 校验 | 声明式、人类可读、生态工具链成熟 |
| 配置热加载 | `fsnotify` + atomic swap | 文件变更监听 + 原子替换路由表，零重启更新 |
| 日志 | `slog`（标准库，Go 1.21 引入） | 结构化日志，零依赖 |
| 指标 | `prometheus/client_golang` | Prometheus 生态事实标准 |
| 构建 | Go Modules + Makefile | 标准依赖管理 + 可重复构建 |
| 容器 | 多阶段 Dockerfile（`scratch` / `distroless`） | 最小攻击面、快速启动 |
| 编排 | Helm 3 Chart | Kubernetes 标准部署包 |

## 1.3 项目目录结构

```
nexus/
├── cmd/
│   └── nexus/              # 主入口
│       └── main.go
├── internal/
│   ├── config/             # 配置中心（Config Center）
│   │   ├── config.go
│   │   ├── loader.go       # YAML 解析 + 热加载
│   │   ├── validator.go    # JSON Schema 校验
│   │   └── version.go      # 配置版本管理与回滚
│   ├── proxy/              # 数据面核心（Data Plane）
│   │   ├── proxy.go        # 反向代理主逻辑
│   │   ├── router.go       # Trie + Map 多层路由匹配
│   │   └── upstream.go     # 上游管理与健康状态
│   ├── middleware/          # 组件化插件链（可插拔组件）
│   │   ├── chain.go        # 中间件编排
│   │   ├── auth.go         # 认证中间件
│   │   ├── ratelimit.go    # 限流中间件
│   │   ├── logging.go      # 访问日志中间件
│   │   └── metrics.go      # 指标采集中间件
│   ├── health/             # 健康检查
│   │   └── health.go       # /healthz, /readyz
│   └── observability/      # 可观测性
│       ├── logger.go       # 结构化日志
│       ├── metrics.go      # Prometheus 指标定义
│       └── trace.go        # Trace context 透传
├── api/
│   └── v1/                 # 控制面管理 API（Control Plane）
│       └── admin.go
├── configs/
│   └── nexus.yaml          # 示例配置文件
├── deployments/
│   └── helm/
│       └── nexus/          # Helm Chart
├── scripts/
│   ├── build.sh
│   └── test.sh
├── docs/                   # 技术文档
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── README.md
```

## 1.4 核心架构模型

> 参考美团 Shepherd API 网关（[百亿规模API网关服务Shepherd的设计与实现](https://tech.meituan.com/2021/05/20/shepherd-api-gateway.html)），采用 **控制面 + 配置中心 + 数据面** 三层架构，实现管理与转发职责分离、配置热更新与全生命周期治理。

```
┌───────────────────────────────────────────────────────────────────┐
│                        Control Plane                              │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────────┐  │
│  │  Admin API     │  │ API Lifecycle  │  │   Monitoring &     │  │
│  │  管理面 API    │  │ 创建/发布/灰度 │  │   Alerting Center  │  │
│  │                │  │ /回滚/下线     │  │   监控与告警中心    │  │
│  └───────┬────────┘  └───────┬────────┘  └────────┬───────────┘  │
│          │                   │                     │              │
│          └───────────────────┼─────────────────────┘              │
│                              │                                    │
└──────────────────────────────┼────────────────────────────────────┘
                               │
┌──────────────────────────────┼────────────────────────────────────┐
│                    Config Center                                  │
│  ┌───────────────────────────▼──────────────────────────────────┐ │
│  │  Route Table (atomic swap) + DSL Config + Local Cache        │ │
│  │  路由表（原子替换） + DSL 声明式配置 + 本地缓存               │ │
│  │  ┌──────────┐  ┌──────────────┐  ┌───────────────────┐      │ │
│  │  │ fsnotify │  │ JSON Schema  │  │ Version History   │      │ │
│  │  │ 文件监听  │  │ 配置校验     │  │ 版本历史 & 回滚    │      │ │
│  │  └──────────┘  └──────────────┘  └───────────────────┘      │ │
│  └──────────────────────────┬───────────────────────────────────┘ │
└─────────────────────────────┼────────────────────────────────────┘
                              │
┌─────────────────────────────┼────────────────────────────────────┐
│               Data Plane    │                                     │
│                             │                                     │
│  Client ──HTTPS──▶ ┌────────▼───────────────────────────────┐    │
│                    │       Middleware/Plugin Chain            │    │
│                    │  ┌───────┐ ┌───────┐ ┌───────┐ ┌─────┐ │    │
│                    │  │ Auth  │→│ Rate  │→│Logging│→│ ... │ │    │
│                    │  │       │ │ Limit │ │/Metric│ │组件  │ │    │
│                    │  └───────┘ └───────┘ └───────┘ └─────┘ │    │
│                    └────────────────┬────────────────────────┘    │
│                                    │                              │
│                    ┌───────────────▼─────────────────┐            │
│                    │  Router (Trie + Map 多层路由)    │            │
│                    │  前缀树 + 精确匹配哈希表          │            │
│                    └───────────────┬─────────────────┘            │
│                                   │                               │
│                    ┌──────────────▼────────────────┐              │
│                    │  Load Balancer (Round-Robin)   │              │
│                    └──────────────┬────────────────┘              │
│                                  │                                │
│                    ┌─────────────▼──────────────────┐  ┌────────┐│
│                    │ Reverse Proxy (httputil) async  │─▶│Upstream││
│                    └────────────────────────────────┘  │Services││
│                                                        └────────┘│
└──────────────────────────────────────────────────────────────────┘
```

## 1.5 关键设计决策

### 1.5.1 中间件链模式（组件化可插拔）

> 参考 Shepherd 的组件化可插拔设计，功能组件（鉴权、限流、熔断、日志、参数校验等）与核心路由松耦合，可按路由/服务维度启用或禁用。采用 Go 标准的 `http.Handler` 中间件链模式：

```go
// Middleware 定义
type Middleware func(http.Handler) http.Handler

// Chain 将多个中间件串联
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middlewares[i](handler)
    }
    return handler
}
```

### 1.5.2 配置热加载（配置中心模式）

> 参考 Shepherd 的配置中心设计，采用本地缓存 + 文件监听实现配置热更新，支持全量/增量配置同步，数据面可缓存最后一次有效配置，配置中心故障不影响转发。

使用 `atomic.Value` 存储当前路由表，配置变更时构建新路由表并原子替换：

```go
type RouteTable struct {
    routes atomic.Value // 存储 []Route
}

func (rt *RouteTable) Swap(newRoutes []Route) {
    rt.routes.Store(newRoutes)
}

func (rt *RouteTable) Match(host, path string) *Route {
    routes := rt.routes.Load().([]Route)
    // 匹配逻辑
}
```

### 1.5.3 优雅关闭

利用 Go 1.8+ 的 `http.Server.Shutdown`，确保在进程退出前完成在途请求：

```go
srv := &http.Server{Addr: ":8080", Handler: handler}
go srv.ListenAndServe()

quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
srv.Shutdown(ctx)
```

## 1.6 PoC 验收标准

| 验收项 | 标准 | 验证方式 |
|--------|------|----------|
| HTTP 反向代理 | 客户端请求经 nexus 转发至上游并返回响应 | `curl http://localhost:8080/api/v1/hello` |
| Host/Path 路由 | 不同 Host 或 Path 路由到不同上游 | 配置两条路由，分别验证 |
| 配置加载 | 从 YAML 文件加载路由配置 | 修改配置文件，验证路由变化 |
| 健康检查端点 | `/healthz` 返回 200 | `curl http://localhost:8080/healthz` |
| 结构化日志 | 请求日志包含 method, path, status, latency | 检查 stdout 日志格式 |

## 1.7 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Go `net/http` 性能不足 | 高并发下延迟升高 | 预留 Envoy/NGINX 数据面切换点；先压测验证 |
| 配置模型过早复杂化 | 开发周期膨胀 | MVP 仅支持 YAML 文件，Admin API 延后 |
| 中间件链顺序耦合 | 逻辑错误难排查 | 明确中间件执行顺序文档 + 集成测试 |
