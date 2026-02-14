# Phase 1: 架构定版与技术选型

> 时间窗口：第 1–2 周 | 可验收产物：架构设计文档 + 项目脚手架 + 基础路由 PoC

## 1.1 总体目标

确定数据面与控制面的技术选型，完成 Go 项目脚手架搭建，并以一个最小的 HTTP 反向代理 PoC 验证核心链路可行性。

## 1.2 技术栈选型

| 层面 | 选型 | 理由 |
|------|------|------|
| 编程语言 | Go 1.22+ | 高性能网络编程、goroutine 天然并发、标准库 `net/http` 成熟、静态编译部署简单 |
| 数据面代理 | 自研（基于 `net/http/httputil.ReverseProxy`） | MVP 阶段可控，后续可切换至 Envoy xDS 或嵌入式 NGINX |
| 配置格式 | YAML + JSON Schema 校验 | 声明式、人类可读、生态工具链成熟 |
| 配置热加载 | `fsnotify` + atomic swap | 文件变更监听 + 原子替换路由表，零重启更新 |
| 日志 | `slog`（Go 1.21+ 标准库） | 结构化日志，零依赖 |
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
│   ├── config/             # 配置加载与校验
│   │   ├── config.go
│   │   ├── loader.go       # YAML 解析 + 热加载
│   │   └── validator.go    # JSON Schema 校验
│   ├── proxy/              # 数据面核心
│   │   ├── proxy.go        # 反向代理主逻辑
│   │   ├── router.go       # Host/Path 路由匹配
│   │   └── upstream.go     # 上游管理与健康状态
│   ├── middleware/          # 中间件/插件链
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
│   └── v1/                 # 管理 API 定义（可选）
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

```
                    ┌─────────────────────────────────────┐
                    │            Control Plane             │
                    │  ┌───────────┐  ┌────────────────┐  │
                    │  │  Config   │  │   Admin API    │  │
                    │  │  Loader   │  │  (optional)    │  │
                    │  └─────┬─────┘  └───────┬────────┘  │
                    │        │                │            │
                    │        ▼                ▼            │
                    │  ┌──────────────────────────────┐   │
                    │  │      Route Table (atomic)     │   │
                    │  └──────────────┬───────────────┘   │
                    └─────────────────┼───────────────────┘
                                      │
                    ┌─────────────────┼───────────────────┐
                    │   Data Plane    │                    │
  Client ──HTTPS──▶ │  ┌─────────────▼────────────────┐   │
                    │  │    Middleware Chain            │   │
                    │  │  ┌─────┐ ┌─────┐ ┌─────────┐ │   │
                    │  │  │Auth │→│Rate │→│ Logging │ │   │
                    │  │  │     │ │Limit│ │/Metrics │ │   │
                    │  │  └─────┘ └─────┘ └─────────┘ │   │
                    │  └──────────────┬────────────────┘   │
                    │                 │                    │
                    │  ┌──────────────▼────────────────┐   │
                    │  │     Router (Host/Path)         │   │
                    │  └──────────────┬────────────────┘   │
                    │                 │                    │
                    │  ┌──────────────▼────────────────┐   │
                    │  │  Load Balancer (Round-Robin)   │   │
                    │  └──────────────┬────────────────┘   │
                    │                 │                    │
                    │  ┌──────────────▼────────────────┐   │  ┌───────────┐
                    │  │  Reverse Proxy (httputil)      │──┼─▶│ Upstream  │
                    │  └───────────────────────────────┘   │  │ Services  │
                    └──────────────────────────────────────┘  └───────────┘
```

## 1.5 关键设计决策

### 1.5.1 中间件链模式

采用 Go 标准的 `http.Handler` 中间件链模式，每个中间件实现 `http.Handler` 接口：

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

### 1.5.2 配置热加载

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
