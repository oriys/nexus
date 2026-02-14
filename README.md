# Nexus

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-Ready-326CE5?logo=kubernetes&logoColor=white)](https://kubernetes.io/)

**Nexus** 是一个使用 Go 构建的高性能 API 网关，采用 **控制面 + 配置中心 + 数据面** 三层架构设计，提供生产级的流量接入、路由转发、安全控制、稳定性保护与可观测能力。

```
Client ──HTTPS──▶ Middleware Chain ──▶ Router (Map+Trie) ──▶ Load Balancer ──▶ Upstream Services
                  (Auth → RateLimit → Logging → ...)
```

## ✨ 核心特性

- **高性能路由** — Map + Trie 双层路由匹配，精确路径 O(1) 查找，前缀/通配符前缀树匹配
- **负载均衡** — 支持 Round-Robin、加权轮询，结合健康检查自动摘除异常实例
- **TLS 终止** — HTTPS 接入与证书热更新，基于 `atomic.Pointer` 实现零锁竞争
- **认证鉴权** — JWT 签名校验 / API Key 认证，可对接 OAuth2/OIDC 身份提供商
- **流量控制** — 滑动窗口限流（429 响应）、超时 / 有限重试 / 熔断
- **可观测性** — 结构化日志（`slog`）、Prometheus 指标、OpenTelemetry Trace 上下文透传
- **配置热加载** — `fsnotify` 文件监听 + `atomic.Value` 原子替换路由表，零重启更新
- **插件化架构** — 基于 `http.Handler` 中间件链，可按路由/服务维度启用或禁用组件
- **云原生部署** — 多阶段 Dockerfile（distroless）、Helm Chart、健康探针、滚动更新与回滚

## 🏗️ 架构概览

```
┌─────────────────────────────────────────┐
│            Control Plane                │
│  Admin API · API Lifecycle · Monitoring │
└──────────────────┬──────────────────────┘
                   │
┌──────────────────┼──────────────────────┐
│           Config Center                 │
│  fsnotify · JSON Schema · Version Mgmt  │
└──────────────────┬──────────────────────┘
                   │
┌──────────────────┼──────────────────────┐
│            Data Plane                   │
│  TLS → Middleware Chain → Router → LB   │
│              → Reverse Proxy → Upstream  │
└─────────────────────────────────────────┘
```

## 🚀 快速开始

### 前置条件

- [Go 1.24+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/)（可选，用于容器化部署）
- [Helm 3](https://helm.sh/)（可选，用于 Kubernetes 部署）

### 源码构建

```bash
# 克隆仓库
git clone https://github.com/oriys/nexus.git
cd nexus

# 构建
make build

# 运行
./bin/nexus --config configs/nexus.yaml
```

### Docker 部署

```bash
# 构建镜像
make docker-build

# 运行容器
docker run -d \
  -p 8080:8080 \
  -p 8443:8443 \
  -p 9090:9090 \
  -v $(pwd)/configs/nexus.yaml:/etc/nexus/nexus.yaml \
  ghcr.io/oriys/nexus-gateway:latest
```

### Helm 部署（Kubernetes）

```bash
helm install nexus deployments/helm/nexus \
  --namespace nexus-system \
  --create-namespace
```

## ⚙️ 配置示例

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

routes:
  - name: user-api
    host: "api.example.com"
    paths:
      - path: /api/v1/users
        type: prefix
        methods: [GET, POST, PUT, DELETE]
    upstream: user-service

logging:
  level: info
  format: json

metrics:
  enabled: true
  path: /metrics
```

## 🔌 端口说明

| 端口 | 用途 |
|------|------|
| `8080` | HTTP 流量入口 |
| `8443` | HTTPS 流量入口 |
| `9090` | Admin API / Prometheus 指标 |

## 📁 项目结构

```
nexus/
├── cmd/nexus/              # 应用主入口
├── internal/
│   ├── config/             # 配置中心（加载、校验、版本管理）
│   ├── proxy/              # 数据面（反向代理、路由、上游管理）
│   ├── middleware/          # 可插拔中间件（鉴权、限流、日志、指标）
│   ├── health/             # 健康探针（/healthz, /readyz）
│   └── observability/      # 可观测性（日志、指标、追踪）
├── api/v1/                 # Admin API
├── configs/                # 配置文件示例
├── deployments/helm/       # Helm Chart
├── docs/                   # 技术设计文档
├── Dockerfile
├── Makefile
└── README.md
```

## 📖 技术文档

| 阶段 | 文档 | 内容 |
|------|------|------|
| Phase 1 | [架构定版与技术选型](docs/phase1-architecture-foundation.md) | Go 技术栈选型、核心架构模型、基础路由 PoC |
| Phase 2 | [核心流量链路](docs/phase2-core-traffic.md) | 路由引擎、负载均衡、访问日志、Prometheus 指标 |
| Phase 3 | [安全与稳定性](docs/phase3-security-stability.md) | TLS 终止、JWT/API Key 鉴权、限流、熔断 |
| Phase 4 | [可观测性与配置管理](docs/phase4-observability-config.md) | 三大信号关联、配置校验与回滚、Admin API |
| Phase 5 | [部署交付与验收](docs/phase5-deployment-delivery.md) | Helm Chart、回滚流程、测试策略、压测报告模板 |

更多背景研究见 [research.md](research.md)。

## 🗺️ 路线图

- [x] 架构设计与技术选型
- [x] 技术方案详细设计（Phase 1–5）
- [x] 高可用 / 高并发 / 可扩展性[技术评审](docs/review-high-availability-concurrency.md)
- [ ] Phase 1 — 项目脚手架与基础路由 PoC
- [ ] Phase 2 — 路由引擎、负载均衡与可观测性基座
- [ ] Phase 3 — TLS、鉴权、限流、熔断
- [ ] Phase 4 — 完整可观测体系与配置管理闭环
- [ ] Phase 5 — Helm 打包、回滚流程、压测与上线 Runbook

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！请在提交前：

1. Fork 本仓库
2. 创建特性分支（`git checkout -b feature/amazing-feature`）
3. 提交更改（`git commit -m 'feat: add amazing feature'`）
4. 推送分支（`git push origin feature/amazing-feature`）
5. 提交 Pull Request

## 📄 许可证

本项目采用 [MIT 许可证](LICENSE)。
