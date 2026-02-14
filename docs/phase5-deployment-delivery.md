# Phase 5: 部署交付与验收

> 时间窗口：第 5–6 周 | 可验收产物：Helm Chart + 回滚流程 + 测试集 + 压测报告 + Runbook

## 5.1 总体目标

完成生产级部署包（容器 + Helm）、回滚能力、最小测试集与压测报告，并输出上线运维手册（Runbook），使 MVP 具备 **可交付、可部署、可运维** 的完整闭环。

## 5.2 容器化

### 5.2.1 多阶段 Dockerfile

```dockerfile
# ---- Build Stage ----
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s -X main.version=$(git describe --tags --always)" \
    -o /nexus ./cmd/nexus

# ---- Runtime Stage ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /nexus /nexus
COPY --from=builder /app/configs/nexus.yaml /etc/nexus/nexus.yaml

EXPOSE 8080 8443 9090

USER nonroot:nonroot

ENTRYPOINT ["/nexus"]
CMD ["--config", "/etc/nexus/nexus.yaml"]
```

### 5.2.2 构建命令

```makefile
# Makefile
VERSION ?= $(shell git describe --tags --always --dirty)
IMAGE   ?= nexus-gateway
REGISTRY ?= ghcr.io/oriys

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags="-w -s -X main.version=$(VERSION)" \
		-o bin/nexus ./cmd/nexus

.PHONY: docker-build
docker-build:
	docker build -t $(REGISTRY)/$(IMAGE):$(VERSION) .
	docker tag $(REGISTRY)/$(IMAGE):$(VERSION) $(REGISTRY)/$(IMAGE):latest

.PHONY: docker-push
docker-push: docker-build
	docker push $(REGISTRY)/$(IMAGE):$(VERSION)
	docker push $(REGISTRY)/$(IMAGE):latest

.PHONY: test
test:
	go test -race -coverprofile=coverage.out ./...

.PHONY: lint
lint:
	golangci-lint run ./...
```

## 5.3 Helm Chart

### 5.3.1 Chart 结构

```
deployments/helm/nexus/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── _helpers.tpl
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── configmap.yaml
│   ├── secret.yaml          # TLS 证书 Secret
│   ├── hpa.yaml             # 水平自动扩缩
│   ├── pdb.yaml             # Pod 中断预算
│   ├── serviceaccount.yaml
│   └── servicemonitor.yaml  # Prometheus ServiceMonitor
└── tests/
    └── test-connection.yaml
```

### 5.3.2 核心 values.yaml

```yaml
# values.yaml
replicaCount: 2

image:
  repository: ghcr.io/oriys/nexus-gateway
  tag: "latest"
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  httpPort: 8080
  httpsPort: 8443
  adminPort: 9090

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

podDisruptionBudget:
  minAvailable: 1

probes:
  liveness:
    path: /healthz
    port: 8080
    initialDelaySeconds: 5
    periodSeconds: 10
    failureThreshold: 3
  readiness:
    path: /readyz
    port: 8080
    initialDelaySeconds: 3
    periodSeconds: 5
    failureThreshold: 2

config:
  # 网关配置，会生成 ConfigMap
  nexus.yaml: |
    server:
      listen: ":8080"
    logging:
      level: info
      format: json

tls:
  enabled: false
  # 使用已有 Secret 或 cert-manager
  secretName: nexus-tls

metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 15s
```

### 5.3.3 Deployment 模板（关键片段）

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "nexus.fullname" . }}
spec:
  replicas: {{ .Values.replicaCount }}
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
  selector:
    matchLabels:
      {{- include "nexus.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "nexus.selectorLabels" . | nindent 8 }}
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
    spec:
      terminationGracePeriodSeconds: 30
      containers:
        - name: nexus
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          ports:
            - name: http
              containerPort: 8080
            - name: https
              containerPort: 8443
            - name: admin
              containerPort: 9090
          livenessProbe:
            httpGet:
              path: {{ .Values.probes.liveness.path }}
              port: {{ .Values.probes.liveness.port }}
            initialDelaySeconds: {{ .Values.probes.liveness.initialDelaySeconds }}
            periodSeconds: {{ .Values.probes.liveness.periodSeconds }}
            failureThreshold: {{ .Values.probes.liveness.failureThreshold }}
          startupProbe:
            httpGet:
              path: {{ .Values.probes.liveness.path }}
              port: {{ .Values.probes.liveness.port }}
            failureThreshold: 30
            periodSeconds: 2
          readinessProbe:
            httpGet:
              path: {{ .Values.probes.readiness.path }}
              port: {{ .Values.probes.readiness.port }}
            initialDelaySeconds: {{ .Values.probes.readiness.initialDelaySeconds }}
            periodSeconds: {{ .Values.probes.readiness.periodSeconds }}
            failureThreshold: {{ .Values.probes.readiness.failureThreshold }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
          volumeMounts:
            - name: config
              mountPath: /etc/nexus
              readOnly: true
      volumes:
        - name: config
          configMap:
            name: {{ include "nexus.fullname" . }}-config
```

## 5.4 健康探针

### 5.4.1 Go 实现

```go
// HealthHandler 健康检查处理器
type HealthHandler struct {
    ready    atomic.Bool
    checks   []HealthCheck
}

type HealthCheck struct {
    Name  string
    Check func() error
}

// Healthz liveness 探针：进程存活即 200
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "alive",
    })
}

// Readyz readiness 探针：所有依赖就绪才 200
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    if !h.ready.Load() {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "not_ready",
        })
        return
    }

    // 运行健康检查
    for _, check := range h.checks {
        if err := check.Check(); err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
            json.NewEncoder(w).Encode(map[string]any{
                "status": "not_ready",
                "check":  check.Name,
                "error":  err.Error(),
            })
            return
        }
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "ready",
    })
}
```

## 5.5 发布与回滚

### 5.5.1 发布流程

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ 1.构建   │────▶│ 2.推送   │────▶│ 3.部署   │────▶│ 4.验证   │
│ 镜像     │     │ 镜像仓库 │     │ Helm     │     │ 探针+    │
│          │     │          │     │ upgrade  │     │ 冒烟测试 │
└──────────┘     └──────────┘     └──────────┘     └────┬─────┘
                                                        │
                                          ┌─────────────┼─────────────┐
                                          │ 通过        │             │ 失败
                                          ▼             │             ▼
                                    ┌──────────┐        │       ┌──────────┐
                                    │ 5.完成   │        │       │ 6.回滚   │
                                    │ 上线     │        │       │ helm     │
                                    │          │        │       │ rollback │
                                    └──────────┘        │       └──────────┘
```

### 5.5.2 部署与回滚命令

```bash
# 部署新版本（--atomic：部署失败自动回滚到上一版本）
helm upgrade nexus deployments/helm/nexus \
  --namespace nexus-system \
  --set image.tag=v1.2.0 \
  --wait \
  --atomic \
  --timeout 5m

# 查看发布历史
helm history nexus --namespace nexus-system

# 回滚到上一版本
helm rollback nexus --namespace nexus-system

# 回滚到指定版本
helm rollback nexus 3 --namespace nexus-system

# Kubernetes 原生回滚
kubectl rollout undo deployment/nexus --namespace nexus-system
```

## 5.6 测试策略

### 5.6.1 测试金字塔

```
          ┌──────────┐
          │  E2E     │  Helm 部署后冒烟测试
          │  Tests   │  (~5 个关键场景)
         ┌┴──────────┴┐
         │ Integration │  中间件链 + 路由 + 上游
         │ Tests       │  (~20 个场景)
        ┌┴────────────┴┐
        │  Unit Tests   │  各模块独立测试
        │               │  (~50+ 用例)
        └───────────────┘
```

### 5.6.2 关键测试用例

```go
// 路由测试示例
func TestRouter_MatchExactPath(t *testing.T) {
    router := NewRouter()
    router.AddRoute(Route{
        Name: "user-api",
        Host: "api.example.com",
        Paths: []PathRule{
            {Path: "/api/v1/users", Type: "prefix"},
        },
        Upstream: "user-service",
    })

    req := httptest.NewRequest("GET", "/api/v1/users/123", nil)
    req.Host = "api.example.com"

    route := router.Match(req)
    assert.NotNil(t, route)
    assert.Equal(t, "user-api", route.Name)
}

// 限流测试示例
func TestRateLimiter_ReturnsHTTP429(t *testing.T) {
    limiter := NewSlidingWindowLimiter(5, time.Minute)
    middleware := RateLimitMiddleware(limiter, ClientIPKeyExtractor)

    handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    // 前 5 个请求应通过
    for i := 0; i < 5; i++ {
        rr := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/test", nil)
        req.RemoteAddr = "192.168.1.1:12345"
        handler.ServeHTTP(rr, req)
        assert.Equal(t, http.StatusOK, rr.Code)
    }

    // 第 6 个请求应被限流
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/test", nil)
    req.RemoteAddr = "192.168.1.1:12345"
    handler.ServeHTTP(rr, req)
    assert.Equal(t, http.StatusTooManyRequests, rr.Code)
}

// 熔断器测试示例
func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
    cb := NewCircuitBreaker(3, 2, 10*time.Second)

    // 正常状态
    assert.True(t, cb.Allow())

    // 连续 3 次失败，触发熔断
    cb.RecordFailure()
    cb.RecordFailure()
    cb.RecordFailure()

    // 熔断状态，请求被拒绝
    assert.False(t, cb.Allow())
}

// JWT 鉴权测试示例
func TestJWTAuth_RejectsExpiredToken(t *testing.T) {
    auth := NewJWTAuthenticator(testKeyFunc, "issuer", []string{"audience"})
    expiredToken := generateExpiredJWT(t)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("Authorization", "Bearer "+expiredToken)

    _, err := auth.Authenticate(req)
    assert.Error(t, err)
}
```

### 5.6.3 E2E 冒烟测试（Helm 部署后）

```bash
#!/bin/bash
# scripts/smoke-test.sh
set -e

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"

echo "=== Nexus Gateway Smoke Test ==="

# 1. 健康检查
echo -n "Health check... "
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/healthz")
[ "$STATUS" = "200" ] && echo "PASS" || { echo "FAIL ($STATUS)"; exit 1; }

# 2. Readiness
echo -n "Readiness check... "
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/readyz")
[ "$STATUS" = "200" ] && echo "PASS" || { echo "FAIL ($STATUS)"; exit 1; }

# 3. Metrics 端点
echo -n "Metrics endpoint... "
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/metrics")
[ "$STATUS" = "200" ] && echo "PASS" || { echo "FAIL ($STATUS)"; exit 1; }

# 4. 404 for unknown route
echo -n "Unknown route returns 404... "
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/nonexistent")
[ "$STATUS" = "404" ] && echo "PASS" || { echo "FAIL ($STATUS)"; exit 1; }

echo "=== All smoke tests passed ==="
```

## 5.7 压测报告模板

```markdown
# Nexus Gateway 压测报告

## 环境
- 硬件：4 vCPU / 8GB RAM
- OS：Linux 6.x
- Go 版本：1.24.x
- 部署方式：Docker / Kubernetes (2 replicas)

## 测试工具
- hey v0.1.x / wrk v4.x

## 测试场景
| 场景 | 并发数 | 总请求数 | 持续时间 |
|------|--------|----------|----------|
| 基准（无中间件） | 200 | 100,000 | - |
| 完整链路 | 200 | 100,000 | - |
| 高并发 | 1000 | 500,000 | - |

## 结果
| 指标 | 基准 | 完整链路 | 高并发 |
|------|------|----------|--------|
| RPS | - | - | - |
| P50 延迟 | - | - | - |
| P95 延迟 | - | - | - |
| P99 延迟 | - | - | - |
| 错误率 | - | - | - |
| CPU 使用率 | - | - | - |
| 内存使用 | - | - | - |

## 结论
（待填写）
```

## 5.8 运维手册（Runbook）大纲

```markdown
# Nexus Gateway Runbook

## 1. 部署
### 1.1 首次部署
### 1.2 升级
### 1.3 回滚

## 2. 配置变更
### 2.1 修改路由
### 2.2 添加上游
### 2.3 调整限流阈值
### 2.4 配置回滚

## 3. 故障处理
### 3.1 网关无响应
### 3.2 上游全部不可用
### 3.3 证书过期
### 3.4 限流误触发
### 3.5 内存 / CPU 告警

## 4. 日常运维
### 4.1 日志查看
### 4.2 指标监控
### 4.3 健康检查
### 4.4 容量评估
```

## 5.9 验收标准

| 验收项 | 标准 | 验证方式 |
|--------|------|----------|
| 容器构建 | 多阶段构建成功，镜像 < 30MB | `docker images` 查看 |
| Helm 部署 | `helm install` 成功，Pod Running | `kubectl get pods` |
| 滚动更新 | 零中断升级 | 持续请求中 `helm upgrade` |
| Helm 回滚 | `helm rollback` 恢复服务 | 回滚后验证功能 |
| 冒烟测试 | 4 项冒烟测试全部通过 | 运行 `smoke-test.sh` |
| 单元测试 | 覆盖率 ≥ 70% | `go test -cover` |
| 压测 | P99 < 50ms（单实例, 简单路由） | `hey` 压测报告 |
| liveness | 进程存活返回 200 | `curl /healthz` |
| readiness | 就绪返回 200，未就绪返回 503 | 验证探针行为 |
| Runbook | 完成运维手册并通过 review | 文档审阅 |
