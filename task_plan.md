# Task Plan — dual-era-mcp-gateway（v2，详细实施计划）

- 版本：2.0
- 日期：2026-11-25
- 状态：B1 MVP **库级/本地级**完成；T8 真实 Envoy 拓扑已 PASS（Envoy 1.40.2）
- 上位文档：[`../envoy-mcp-gateway-plan-a-prd.md`](../envoy-mcp-gateway-plan-a-prd.md)
- 细化规格：
  - [`docs/superpowers/specs/2026-11-24-dual-era-b1-design.md`](docs/superpowers/specs/2026-11-24-dual-era-b1-design.md)
  - [`docs/spec/requirements.md`](docs/spec/requirements.md)
  - [`docs/spec/protocol-matrix.md`](docs/spec/protocol-matrix.md)
  - [`docs/spec/tool-sources.md`](docs/spec/tool-sources.md)
  - [`docs/spec/providers.md`](docs/spec/providers.md)
  - [`docs/spec/acceptance-tests.md`](docs/spec/acceptance-tests.md)
  - [`docs/spec/traceability.md`](docs/spec/traceability.md)

> 状态图例：
> - `[x]` 已实现，且有 Go 测试或可复现命令。
> - `[~]` 部分实现：库已存在但未接入运行路径，或只覆盖子集。
> - `[ ]` 未开始。
> - `[!]` 被环境阻塞（Docker/Envoy/凭据等），不是“已完成”。

---

## 0. 一页纸结论

### 0.1 要解决的问题

同时服务两类 MCP 客户端，且不把“旧协议能跑”包装成“新协议也支持”：

| 客户端 | 入口 | 协议 | 关键约束 |
|---|---|---|---|
| legacy 2025-xx | `/mcp` | 2025-03-26 / 2025-06-18 | `initialize`、`Mcp-Session-Id`、SSE 兼容 |
| modern 2026-07-28 | `/modern/mcp` | 2026-07-28 Stateless Core | `_meta`、header 校验、`server/discover`、无 session、无 GET |

Gate 1 已证明：单个 Envoy listener 用 `mcp_router` 做 terminal 时，无法干净地同时服务 modern client。因此采用 **B1 版本化双端点**，而不是单端点。

### 0.2 当前交付的实际形态

```
Client
  │
  ├─ /mcp ──────────────▶ legacy endpoint
  │                        本地：Go legacyendpoint
  │                        生产：Envoy mcp_router + mcp_multicluster
  │
  └─ /modern/mcp ───────▶ Go modern facade
                           └─ LegacyClient ──▶ legacy endpoint / internal router
```

- 两种工具源：native MCP + REST。
- 两层配置抽象：Provider（static/OpenAPI/K8s manifest/Nacos）+ ToolSource（native/REST）。
- 控制面：配置校验、SHA-256 版本、快照、历史/回滚、ACK/NACK 模型、Envoy 配置生成、public CDS/RDS/LDS xDS。
- 策略：API key、tenant 派生、allowlist、进程内固定窗口限流。
- 测试：T0–T7 + ADS CDS/RDS/LDS + ACK/NACK + ext_authz/ratelimit 测试通过；T8 Envoy 1.40.2 PASS。

### 0.3 不能夸大的部分

以下能力**没有**在真实 Envoy/K8s 运行时验证：

- TLS/HTTP2 listener、`mcp` filter、`ext_authz`、`ratelimit`、`mcp_router`、`mcp_multicluster` 的真实行为；
- Envoy 配置是否 `--mode validate` 通过（本机无 Envoy）；
- SSE 长连接、LDS drain、零停机；
- internal `mcp_router` listener 的 LDS 资源（public LDS/RDS 已做）；
- K8s CRD/controller/admission webhook；
- ext_authz/ratelimit gRPC 服务、Redis、JWT；
- 生产级可观测、Helm/kind、压测/混沌。

这些必须在 README、简历、答辩中明确为“未完成/环境阻塞”，不能写成“已上线”。

### 0.4 完成定义（本计划）

MVP 代码级完成：

- T0–T7 全绿；
- `go vet` 全绿；
- ADS CDS/RDS/LDS streaming + ACK/NACK callback 测试绿；
- 所有 P0 库级需求有测试；
- 文档明确区分“已验证/未验证”。

生产级完成（尚未达到）：

- T8 真实 Envoy 拓扑 + AC-01..AC-30；
- TLS/mTLS、ext_authz/ratelimit/Redis；
- 零停机/SSE/压测/混沌；
- 可观测与 runbook；
- K8s/Helm/kind。

---

## 1. 范围与阶段

### 1.1 Phase 1（MVP，P0）

| 域 | MVP 内容 | 当前状态 |
|---|---|---|
| 协议 | legacy `/mcp` + modern `/modern/mcp` | `[x]` 本地；Envoy 运行时 `[!]` |
| 控制面 | 配置模型、校验、版本、快照、回滚、ACK 模型 | `[x]` |
| xDS | public CDS/RDS/LDS + ACK/NACK | `[~]` public 完成；internal listener 静态 |
| 数据面 | public path routing、legacy 聚合、modern facade | `[~]` 本地 Go；Envoy `[!]` |
| 策略 | API key/tenant/allowlist/内存限流 | `[x]` 库；ext_authz/Redis `[ ]` |
| 工具源 | native MCP + REST | `[x]` 库；复杂 REST `[~]`（OAuth/HMAC 已做） |
| Provider | static/OpenAPI/K8s manifest/Nacos | `[x]` 库；live informer/discovery `[ ]` |
| 可观测 | metrics/logs/traces | `[ ]` 基本未做 |
| 运维 | CI、runbook、Helm/kind | `[~]` CI/runbook/Helm 骨架；kind/集群未验 |

### 1.2 Phase 2（Stretch）

- modern MCP upstream（直连 2026-07-28 后端）。
- internal `mcp_router` listener 的 LDS 资源生成。
- canary（node metadata / 独立 Deployment）。
- JWT/OIDC、Redis 分布式限流、并发限制。
- stdio 进程池/自动重启、SSE。
- 复杂 REST：multipart/二进制/长任务/复杂分页（OAuth/HMAC 已做）。
- OpenAPI 3.1 完整 importer、`$ref` 解析。
- K8s informer/CRD/controller、Nacos 服务发现。
- 多 gateway/多 listener/多租户物理隔离。
- 控制面 UI/CLI、审计防篡改。

### 1.3 Out of Scope

- 完整 MCP resources/prompts/sampling/roots/tasks/MRTR。
- OAuth 授权码流程、动态客户端注册。
- 计费/账单/配额结算。
- 多租户物理隔离（独立 Envoy/NetworkPolicy per tenant）。
- 生产级 WIP Envoy filter 加固。
- Windows 生产运行时（开发/测试可以）。
- 公网暴露；Phase 1 假定可信网络 + TLS。

### 1.4 假设与约束

| 项 | 约束 |
|---|---|
| Go | 1.26+ |
| Envoy | 必须锁 digest；当前 Gate 1 锁 `envoyproxy/envoy:dev` 的 digest，1.41.0 final 后 re-lock |
| K8s | Phase 2；当前未实现 CRD/controller |
| Redis | Phase 2；当前限流是进程内固定窗口 |
| 环境 | Docker 可用（需 named-pipe 全权限）；无 Envoy binary、无 gcc/clang；`-race` 走 CI |
| 凭据 | GitHub push 在当前会话可能失败（`SEC_E_NO_CREDENTIALS`），不代表代码问题 |
| 网络 | Phase 1 可信内网；不承诺公网安全 |
| MCP | HTTP/SSE；stdio 单进程桥已做，进程池 Phase 2 |

---

## 2. 架构

### 2.1 B1 组件图

```
                         ┌──────────────────────────────┐
                         │ Go control plane             │
                         │ validator → snapshot → ADS   │
                         │ revision store → rollback    │
                         └──────────────┬───────────────┘
                                        │ ADS (gRPC, mTLS planned)
                                        ▼
┌──────────────┐   HTTPS/HTTP2   ┌──────────────────────────────┐
│ MCP Client   │────────────────▶│ Public Envoy                 │
└──────────────┘                 │  /mcp          /modern/mcp   │
                                 └───────┬──────────────┬───────┘
                                         │              │
                                         ▼              ▼
                              ┌────────────────┐  ┌──────────────────┐
                              │ Internal Envoy │  │ Go modern facade │
                              │ mcp_router     │  │ 2026-07-28       │
                              │ mcp_multicluster│ └────────┬─────────┘
                              └───────┬────────┘          │
                                      │                   │ LegacyClient
                                      ▼                   ▼
                         ┌────────────────────┐   ┌────────────────┐
                         │ native MCP backends│   │ internal legacy │
                         │ REST bridge/adapter│   │ endpoint/router │
                         └────────────────────┘   └────────────────┘
```

生产目标：public Envoy 只做 L7/TLS/auth/ratelimit/path split；internal Envoy 拥有 terminal `mcp_router`；modern facade 只依赖 legacy 内部接口。

### 2.2 组件职责

| 组件 | 职责 | 不做什么 | 当前 |
|---|---|---|---|
| `cmd/controlplane` | 配置 API、版本/回滚、Envoy 配置生成、xDS CDS | 不转发 MCP 请求 | `[x]` 本地 |
| `internal/xds` | ADS CDS/RDS/LDS + ACK/NACK | 不生成 internal listener LDS | `[~]` public 完成 |
| `cmd/gateway` | 本地双端点 + 聚合 | 不是生产 Envoy 替代 | `[x]` 本地 |
| `internal/facade` | 2026-07-28 入口 | 不处理 legacy session | `[x]` |
| `internal/legacyendpoint` | 本地 legacy `/mcp` | 不替代 Envoy `mcp_router` | `[x]` 测试/本地 |
| `internal/aggregator` | 多源前缀/路由 | 不做策略 | `[x]` |
| `internal/policy` | API key/tenant/allowlist/限流 | 不是 ext_authz gRPC 服务 | `[x]` 库 |
| `internal/source` | static/OpenAPI/K8s/Nacos Provider | 不做运行时发现 | `[~]` |
| `internal/restadapter` | REST SourceSpec → legacy MCP client | 不做 OAuth/HMAC/multipart | `[~]` |
| `internal/envoy` | 生成 draft Envoy 拓扑 | 不是运行时验证 | `[~]` |
| `internal/edge` | 本地 path dispatch | 不是 Envoy | `[x]` |

### 2.3 请求流：`tools/call`

本地测试路径：

1. Client → `edge`（`/mcp` 或 `/modern/mcp`）。
2. modern：`facade` 校验 `_meta`/header → `aggregator` → `legacy.HTTPClient` → mock/legacy endpoint。
3. legacy：`legacyendpoint` → `aggregator` → mock/legacy endpoint。
4. `aggregator` 前缀 `backend__tool`，`registry` 排序/冲突检测。
5. `facade` 为 modern 结果补 `resultType: complete`。
6. 策略（若配置）：`facade` 调 `Policy.Authenticate/Authorize/Allow`。

生产目标路径：

1. Client → public Envoy TLS/HTTP2。
2. `mcp` filter 解析 header/body → ext_authz → ratelimit。
3. `/mcp` → internal Envoy `mcp_router`；`/modern/mcp` → Go facade。
4. `mcp_router` 按前缀路由到 native MCP/REST bridge。
5. Access log/OTel/审计。

### 2.4 配置流

1. 读取 static/OpenAPI/K8s/Nacos Provider → `[]SourceSpec`。
2. `controlplane.Validate`：source/prefix/tool/消费者/限流引用校验。
3. 规范化 + SHA-256 → `Snapshot.Version`。
4. `Manager.Apply` 保存 current/history；非法配置保持上一版。
5. `xds.Apply` 生成 CDS snapshot；`cmd/controlplane` 同时提供 `/api/v1/envoy` 静态拓扑。
6. 回滚：按 version 找到历史 → 重新 apply。
7. ACK/NACK：`AckStore` 模型；尚未接 `go-control-plane` callback。

### 2.5 端口/协议（当前与目标）

| 组件 | 端口 | 协议 | 状态 |
|---|---|---|---|
| 本地 gateway | 8080 | HTTP | `[x]` |
| controlplane admin | 18080 | HTTP | `[x]` |
| controlplane xDS | 18000 | gRPC/ADS | `[x]` CDS |
| legacy mock | 9001/9002 | HTTP | `[x]` |
| public Envoy | 8443 | HTTPS/HTTP2 | `[~]` TLS overlay 配置；未运行 |
| internal Envoy | 10001 | HTTP | `[~]` 配置草案 |
| ext_authz | 19001 | gRPC | `[x]` 服务 |
| ratelimit | 19002 | gRPC | `[x]` 服务 |
| Redis | 6379 | Redis | `[~]` 代码 + miniredis；未部署 |
| stdio bridge | 18081 | HTTP | `[x]` 单进程 |

> 端口以部署配置为准；本地 mock 占用 9001/9002，避免与策略服务冲突。

### 2.6 部署拓扑

当前：

- Windows 本地 `cmd/gateway` + `cmd/legacy-mock`；
- `cmd/controlplane` + xDS CDS；
- 无 Docker/Envoy/K8s 运行。

目标：

- 本地 `docker-compose`：controlplane/envoy/ext-authz/ratelimit/redis/mock MCP/REST mock；
- K8s：`mcp-system` 控制面/策略面，`mcp-demo` 后端；Envoy 2 副本；控制面 2 副本且都跑 xDS；NetworkPolicy 禁止公网直连后端。

---

## 3. 工作分解结构（WBS）

> 依赖列只列关键依赖；状态以“当前仓库真实状态”为准。

### WS-A 规格与契约

| ID | 任务 | 优先级 | 依赖 | 验收 | 状态 |
|---|---|---|---|---|---|
| A-01 | PRD → 仓库 traceability | P0 | PRD | `docs/spec/traceability.md` 覆盖 FR/AC | `[x]` |
| A-02 | B1 设计文档 | P0 | Gate 1 | 设计评审通过 | `[x]` |
| A-03 | 协议矩阵 | P0 | B1 | legacy/modern 字段/错误矩阵 | `[x]` |
| A-04 | 验收测试 T0–T8 | P0 | A-02 | 测试用例可执行 | `[x]` |
| A-05 | ADR-001..007 | P1 | 关键决策 | 每个决策有上下文/取舍 | `[x]` `docs/adr/` |
| A-06 | SourceSpec schema | P0 | B1 | `api/spec/source-v1alpha1.yaml` | `[x]` |
| A-07 | 文档一致性检查 | P1 | A-01 | README/spec/plan 不互相矛盾 | `[~]` |

### WS-B 控制面

| ID | 任务 | 优先级 | 依赖 | 验收 | 状态 |
|---|---|---|---|---|---|
| B-01 | Config 领域模型 | P0 | A-06 | `controlplane.Config` | `[x]` |
| B-02 | 静态校验 | P0 | B-01 | 非法 source/ref/prefix 被拒 | `[x]` |
| B-03 | SHA-256 版本 | P0 | B-01 | 同配置版本稳定 | `[x]` |
| B-04 | Snapshot | P0 | B-03 | current/history 可查 | `[x]` |
| B-05 | 回滚 | P0 | B-04 | 按 version 恢复 | `[x]` |
| B-06 | ACK/NACK 模型 | P0 | B-04 | ack/nack/converged 测试 | `[x]` 模型 + xDS callback |
| B-07 | ADS/CDS/RDS/LDS | P0 | B-04 | gRPC ADS 返回 cluster/route/listener | `[x]` public 资源 |
| B-08 | LDS/RDS xDS | P0 | B-07 | listener/route 资源 + Envoy 验证 | `[ ]` 仅 public；internal `mcp_router` 仍静态 |
| B-09 | 管理 API | P0 | B-04 | status/config/revisions/apply/rollback/envoy | `[~]` 无鉴权/dataplanes/tools |
| B-10 | CRD + controller | P0 | B-01 | `kubectl apply` → snapshot | `[~]` CRD schema + ConfigMap 目录 controller；CRD informer 未做 |
| B-11 | Admission webhook | P0 | B-10 | 非法 CRD 拒绝 | `[~]` handler/单测/manifests；无集群/cert-manager 验证 |
| B-12 | 2 副本 + leader election + mTLS | P0 | B-07 | kill 1 副本不断流 | `[ ]` |
| B-13 | Tool Registry 服务 | P0 | B-01 | `/api/v1/tools`、stale 标记 | `[~]` 代码内有 catalog |
| B-14 | Canary | P1 | B-12 | node metadata 分 channel | `[ ]` |
| B-15 | 配置审计 | P0 | B-09 | 谁/何时/旧新 version | `[~]` actor/reason/old-new/audit API；actor 身份未鉴权 |
| B-16 | Dry-run diff | P1 | B-09 | 不写 snapshot 返回 diff | `[x]` API + 测试 |

### WS-C 数据面

| ID | 任务 | 优先级 | 依赖 | 验收 | 状态 |
|---|---|---|---|---|---|
| C-01 | Public path routing | P0 | B-07 | `/mcp` 与 `/modern/mcp` 分流 | `[x]` T8 PASS |
| C-02 | Legacy `/mcp` | P0 | C-01 | initialize/list/call | `[x]` T8 initialize/list PASS |
| C-03 | Modern `/modern/mcp` | P0 | C-01 | discover/list/call | `[x]` T8 discover/list PASS |
| C-04 | HCM filter chain | P0 | C-01 | mcp→ext_authz→ratelimit→router | `[~]` T8 public 有 ext_authz+ratelimit；mcp 在 internal；无 TLS |
| C-05 | TLS/HTTP2 | P0 | C-01 | `curl --http2` | `[x]` T8 `-TLS` PASS，TLS1.3+ALPN h2，`h2check` 实际 HTTP/2 200 |
| C-06 | `mcp_multicluster` | P0 | B-07 | 后端列表走 CDS | `[~]` 配置生成 |
| C-07 | SSE/长连接 | P0 | C-02 | `curl -N` 不 buffer | `[x]` T8 `sse-shim` PASS；raw `mcp_router` GET 仍 405 |
| C-08 | retry/outlier/circuit | P0 | C-06 | 故障注入 | `[~]` public cluster/route 配置；未运行故障注入 |
| C-09 | body/header 限制 | P0 | C-03 | 413/431 | `[~]` body 只有 facade |
| C-10 | Origin 校验 | P0 | C-03 | 非法 403 | `[x]` facade |
| C-11 | 零停机 LDS/CDS | P0 | C-06 | 100 次变更 + SSE | `[ ]` |
| C-12 | 优雅 drain | P0 | C-01 | 滚动更新 | `[ ]` |
| C-13 | Access log JSON | P0 | C-01 | 字段稳定 | `[x]` T8 实际输出 JSON access log |
| C-14 | OTel Trace | P0 | C-01 | traceparent 传播 | `[~]` X-Trace-Id/traceparent；无 OTel exporter |

### WS-D 策略面

| ID | 任务 | 优先级 | 依赖 | 验收 | 状态 |
|---|---|---|---|---|---|
| D-01 | API Key SHA-256 | P0 | B-01 | 正确 200/错误 401 | `[x]` |
| D-02 | tenant 只从凭证 | P0 | D-01 | 伪造 `x-tenant-id` 无效 | `[x]` |
| D-03 | 工具 allowlist | P0 | D-01 | 越权 403 | `[x]` |
| D-04 | 内存 RPM 限流 | P0 | D-01 | 超限 429/layer | `[x]` |
| D-05 | Redis Lua 限流 | P0 | D-04 | 原子、跨副本 | `[x]` miniredis 单测 + T8 真实 Redis（60×200/10×429） |
| D-06 | ext_authz gRPC | P0 | D-01 | Envoy 可调用 | `[x]` 服务 + 单测 + T8 实际调用 |
| D-07 | ratelimit gRPC | P0 | D-05 | Envoy 可调用 | `[x]` 服务 + 单测 + T8 实际 429；未接 Redis |
| D-08 | 429 JSON-RPC body/Retry-After | P0 | D-07 | 响应体/头正确 | `[x]` T8 实际 429（60×200 / 10×429） |
| D-09 | JWT/JWKS | P0 | D-06 | 过期/错 audience 401 | `[x]` HS256/RS256/JWKS 缓存 + 测试；无密钥轮换/审计 |
| D-10 | 审计 JSONL | P0 | D-06 | 无 payload/密钥 | `[x]` `internal/audit` + ext_authz/ratelimit |
| D-11 | 策略指标 | P0 | D-06 | decisions/degraded | `[~]` facade 计数器；gRPC 指标未做 |
| D-12 | 失败模式 | P0 | D-06 | auth fail-closed；Redis fail-open | `[x]` ext_authz deny + T8 Redis 停机 fallback |
| D-13 | 接入 `cmd/gateway` | P0 | D-01 | 本地网关真正启用策略 | `[x]` `-policy` 已接线；legacy 路径仍由 Envoy/ext_authz 负责 |

### WS-E 工具源 / Provider

| ID | 任务 | 优先级 | 依赖 | 验收 | 状态 |
|---|---|---|---|---|---|
| E-01 | `SourceSpec` | P0 | A-06 | source/tool/headers | `[x]` |
| E-02 | Static Provider | P0 | E-01 | YAML load/reload | `[x]` |
| E-03 | OpenAPI Provider | P1 | E-01 | operationId/parameters/body → tools | `[~]` 最小、无 $ref |
| E-04 | K8s manifest Provider | P1 | E-01 | ConfigMap/McpSource | `[x]` |
| E-05 | Nacos Config Provider | P1 | E-01 | dataId/group/tenant | `[x]` |
| E-06 | Native MCP client | P0 | C-02 | initialize/session/list/call | `[x]` |
| E-07 | REST adapter | P0 | E-01 | path/query/body | `[x]` Go adapter + T8 `mcp_json_rest_bridge`；OAuth/HMAC 见 E-08 |
| E-08 | 复杂 REST | P2 | E-07 | OAuth/HMAC/multipart/长任务 | `[~]` OAuth2 client credentials + HMAC；multipart/长任务未做 |
| E-09 | stdio bridge | P2 | C-02 | 进程池/重启 | `[~]` 单进程 JSON-RPC 桥 + HTTP handler；无进程池/自动重启 |
| E-10 | Registry 前缀/排序/冲突 | P0 | E-06/E-07 | 单源也带前缀 | `[x]` |
| E-11 | modern MCP upstream | P2 | C-03 | 直连 2026-07-28 | `[x]` `internal/modernclient` + `cmd/gateway -modern`；Envoy 路由未做 |
| E-12 | Provider/Adapter 接入控制面 | P0 | E-02/E-07 | 从配置构建真实 catalog | `[~]` 库有，命令未接线 |
| E-13 | SSRF/URL allowlist | P0 | E-07 | metadata IP/内网拒绝 | `[x]` metadata host + allowedHosts |

### WS-F 可观测 / 运维

| ID | 任务 | 优先级 | 依赖 | 验收 | 状态 |
|---|---|---|---|---|---|
| F-01 | 控制面指标 | P0 | B-09 | Prometheus | `[x]` apply/rollback/ACK/NACK `/metrics`；无部署 |
| F-02 | Envoy 指标 | P0 | C-01 | QPS/latency/5xx | `[~]` admin `/stats/prometheus` + scrape 配置；未部署 |
| F-03 | 策略指标 | P0 | D-06 | authz/ratelimit | `[~]` facade 计数器；无 gRPC 指标 |
| F-04 | 结构化日志 | P0 | C-01 | JSON/字段稳定 | `[~]` JSON logger 库；`cmd/gateway` 仍用 log |
| F-05 | Trace | P0 | C-14 | Tempo/Jaeger | `[~]` trace id 传播；无 OTel |
| F-06 | Grafana 看板 | P0 | F-01..F-03 | 四块看板 | `[~]` starter JSON；未部署 |
| F-07 | 告警规则 | P0 | F-01 | NACK/5xx/429/Redis/证书 | `[~]` 4 条 starter 规则；未接 Prometheus |
| F-08 | Runbook | P0 | C-11 | 启动/变更/故障/回滚 | `[x]` `docs/runbook.md` |
| F-09 | kind + Helm | P0 | C-05 | 5 分钟起 | `[~]` Helm 骨架；无 kind/集群验证 |
| F-10 | docker-compose | P1 | C-05 | 本地一键 | `[x]` T8 compose 实际运行 |
| F-11 | CI | P0 | G-01..G-08 | build/vet/test/race/lint/integration | `[~]` test/vet/build/race；无 lint/集成 |
| F-12 | T8 harness | P0 | F-09/F-10 | Envoy 拓扑测试 | `[x]` Envoy 1.40.2 PASS |
| F-13 | 依赖/镜像扫描 | P1 | F-11 | govulncheck/trivy | `[~]` `scripts/scan.ps1`；本机未安装工具，SKIP |

### WS-G 测试与文档

| ID | 任务 | 优先级 | 依赖 | 验收 | 状态 |
|---|---|---|---|---|---|
| G-01 | T0 codec/spec | P0 | A-03 | PASS | `[x]` |
| G-02 | T1 modern boundary | P0 | C-03 | PASS | `[x]` |
| G-03 | T2 tools aggregation | P0 | E-10 | PASS | `[x]` |
| G-04 | T3 legacy session | P0 | E-06 | PASS | `[x]` |
| G-05 | T4 dual-era/mixed | P0 | C-01 | PASS | `[x]` |
| G-06 | T5 policy | P0 | D-01..D-04 | PASS | `[x]` |
| G-07 | T6 control plane | P0 | B-04..B-07 | PASS | `[x]` |
| G-08 | T7 resilience/concurrency | P0 | C-03 | PASS | `[x]` |
| G-09 | T8 Envoy runtime | P0 | F-12 | 真实拓扑 | `[x]` PASS |
| G-10 | AC-01..AC-30 | P0 | T0..T8 | 全部通过或有明确 skip | `[~]` 子集 |
| G-11 | 覆盖率报告 | P1 | G-01..G-08 | `docs/test-report.md` | `[x]` |
| G-12 | 压测 | P0 | F-12 | p99/5xx/SSE | `[~]` T8 5s/10 并发 3.1k RPS，p95 4.8ms，0 error；无 500RPS/10min/SSE |
| G-13 | 混沌 | P0 | F-12 | 7.10 行为 | `[~]` T8 实测 backend 部分失败/恢复、ext_authz fail-closed、Redis 停机 fail-open；无 NACK/SSE/HA |
| G-14 | 安全测试 | P0 | D-06 | 401/403/429/413/SSRF | `[~]` 401/403/429/Origin/SSRF/body；无 413/431 Envoy 实测 |
| G-15 | 5 分钟 demo | P1 | F-09 | 可录制 | `[x]` `docs/demo.md`；未录视频 |
| G-16 | ADR-001..007 | P1 | A-05 | 3 篇以上 | `[x]` 7 篇 |
| G-17 | PRD traceability | P0 | A-01 | FR/AC 映射 | `[x]` |
| G-18 | commit/push | P0 | G-01..G-08 | 远端同步 | `[~]` P2 变更待提交/推送 |

### 3.1 2026-11-25 P2 增量（本轮完成）

| 项 | 结果 | 证据 |
|---|---|---|
| ext_authz gRPC | API key/tenant/allowlist，401/403 | `internal/extauthz` 单测 |
| ratelimit gRPC | tenant/tool RPM、429 body、layer、Retry-After | `internal/ratelimit` 单测 |
| gateway 策略接线 | `cmd/gateway -policy` | 编译 + facade 策略测试 |
| public xDS | CDS + RDS + LDS | `internal/xds/full_test.go` |
| ACK/NACK | go-control-plane callback → AckStore | `internal/xds/callbacks_test.go` |
| 可观测 | `/metrics`、JSON logger、trace id | `internal/obs` 单测 |
| REST SSRF | metadata host 拒绝 + allowedHosts | `internal/restadapter` 单测 |
| T8 harness | public/internal Envoy + policy + mocks | `deploy/t8/`、`scripts/t8.ps1`、`docs/t8-report.md`（PASS） |
| Helm | 5 个 Deployment + Service + ConfigMap | `deploy/helm/dual-era`（无 helm/集群验证） |
| 微基准 | facade list/call | `docs/benchmarks/2026-11-25.md` |
| modern MCP client | `ListTools`/`Call`/`Discover` + headers/meta | `internal/modernclient` 单测；`cmd/gateway -modern` |
| trace 传播 | `X-Trace-Id`/traceparent 到 legacy upstream | `internal/legacy` 测试 |
| ADR/runbook/demo | 7 篇 ADR、runbook、5 分钟 demo、k6 脚本 | `docs/adr`, `docs/runbook.md`, `docs/demo.md`, `scripts/k6` |
| audit | JSONL auth/rate-limit events + drop counter | `internal/audit` 单测 |
| JWT | HS256 + RS256/JWKS 缓存，issuer/audience/exp/nbf + tenant claim | `internal/policy` 单测 |
| dry-run | config diff API | `controlplane.DiffConfigs` + `/api/v1/dry-run` |
| stdio bridge | 子进程 JSON-RPC + `/mcp` handler | `internal/stdio` helper-process tests |
| REST auth | OAuth2 client credentials + HMAC | `internal/restadapter` 测试 |
| scan/resume | 依赖扫描脚本 + 简历/面试问答 | `scripts/scan.ps1`, `docs/resume.md` |
| admission | AdmissionReview 校验 + 单测 | `internal/admission`, `cmd/admission` |
| controller | ConfigMap 目录 → 控制面 apply，内容哈希去重 | `internal/controller` 测试 |
| load | 本地 Go-mode 7.8k RPS；T8 Envoy 2.9k RPS / p95 5.34ms / 0 error | `docs/benchmarks/2026-11-25-{local-load,t8-load}.md` |
| config audit/metrics | actor/reason/old-new + apply/rollback/ACK/NACK counters | `/api/v1/audit`, `/metrics`, manager tests |
| dashboards/alerts | starter Grafana JSON + Prometheus rules | `deploy/observability`（未部署） |

仍未完成：T8 的 TLS/SSE/ratelimit/Redis 覆盖、drain、CRD informer/controller、internal `mcp_router` LDS、k6/混沌。

---

## 4. Gate 与里程碑

| Gate | 目标 | 通过条件 | 当前 |
|---|---|---|---|
| Gate 0 | 仓库/规格/计划 | README/spec/plan/CI | **PASS** |
| Gate 1 | Envoy 能力 Spike | 版本/digest、filter/router/REST、CDS/LDS 结论 | **PASS（条件性）**；WIP |
| Gate 2 | 控制面骨架 | ADS + ACK/NACK + 单后端 call | **PARTIAL**：CDS + 模型；无 Envoy ACK |
| Gate 3 | 聚合 | 2 后端 + 前缀 + list/call | **PARTIAL**：Go 库测试通过；Envoy 未跑 |
| Gate 4 | 策略面 | ext_authz/ratelimit/Redis/401/403/429 | **PARTIAL**：库策略；无 gRPC/Redis |
| Gate 5 | modern + REST | conformance + REST 工具 | **PARTIAL**：facade/REST adapter；无 Envoy conformance |
| Gate 6 | 零停机 | 100 次变更 + 500 RPS + SSE | **NOT RUN** |
| Gate 7 | 可观测/安全/混沌 | 看板/告警/Trace/chaos | **NOT STARTED** |
| Final | 交付 | AC-01..30 + demo + report | **NOT DONE** |

### 4.1 8 周计划（按 PRD，映射当前状态）

| 周 | PRD 目标 | 当前状态 |
|---|---|---|
| W1 | Spike + 脚手架 | `[x]` Gate 1、仓库、CI |
| W2 | 控制面骨架 | `[~]` 配置/版本/CDS；CRD/controller 未做 |
| W3 | 聚合 + stdio | `[~]` native/REST 聚合；stdio 单进程桥已做 |
| W4 | 策略面 | `[~]` ext_authz/ratelimit 服务已做；Redis/TLS/Envoy 运行时未验 |
| W5 | 2026-07-28 + REST | `[~]` facade/REST adapter；Envoy conformance 未做 |
| W6 | 零停机 | `[ ]` |
| W7 | 可观测/安全/混沌 | `[~]` metrics/trace/审计/扫描脚本/chaos 矩阵；未部署/未注入 |
| W8 | 收尾/demo | `[~]` README/test report；demo/ADR 未完成 |

---

## 5. 需求追踪（摘要）

完整矩阵见 [`docs/spec/traceability.md`](docs/spec/traceability.md)。

| 需求组 | 已实现 | 部分 | 未实现 | 说明 |
|---|---:|---:|---:|---|
| FR-CP 控制面（17） | 2 | 6 | 9 | CRD/controller/HA/mTLS 未做 |
| FR-DP 数据面（18） | 2 | 6 | 10 | Envoy 运行时整体未验证 |
| FR-POL 策略面（12） | 4 | 3 | 5 | 无 ext_authz/Redis/JWT |
| FR-TR Tool Registry（7） | 3 | 2 | 2 | 无独立服务/drift |
| FR-STD stdio（7） | 1 | 2 | 4 | 单进程桥、基础 HTTP；进程池/SSE/重启未做 |
| FR-REST REST（10） | 2 | 5 | 3 | Go adapter 有；Envoy/SSRF 未做 |
| FR-OBS 可观测（9） | 0 | 0 | 9 | 未开始 |
| FR-SEC 安全（10） | 2 | 3 | 5 | TLS/SSRF/admin auth 未做 |
| FR-OPS 运维（8） | 2 | 2 | 4 | Helm/kind/runbook 未做 |
| FR-TS 工具源（6） | 5 | 1 | 0 | 库级完成 |
| NFR（6） | 0 | 4 | 2 | 性能/可靠性未测 |
| AC-01..30（30） | 14 | 6 | 10 | 无端到端验收 |

> 这不是“失败清单”，而是面试时能讲清楚的边界：**哪些是代码/测试证明，哪些只是设计**。

---

## 6. 测试计划

### 6.1 测试金字塔

| 层 | 当前 | 目标 |
|---|---|---|
| 单元 | `internal/*` Go tests | 核心包 ≥ 75% |
| 组件 | `httptest` legacy/facade/REST/xDS | ext_authz/ratelimit gRPC |
| 集成 | `test/boundary/dual_era_test.go` | kind + Envoy + controlplane |
| 契约 | 自写 modern/legacy 请求 | 官方 MCP Go SDK conformance |
| 性能 | 无 | k6 500 RPS/10min |
| 故障 | 部分超时/5xx | toxiproxy/kill/Redis down/NACK |
| 安全 | API key/allowlist/Origin/body | SSRF/JWT/header/depth/重复 key |

### 6.2 T0–T8 状态

| Stage | 内容 | 测试位置 | 状态 |
|---|---|---|---|
| T0 | codec/`_meta`/错误码 | `internal/mcp` | `[x]` |
| T1 | modern boundary | `internal/facade` | `[x]` |
| T2 | tools aggregation/call | `internal/facade`, `registry` | `[x]` |
| T3 | legacy client/session | `internal/legacy`, `legacyendpoint` | `[x]` |
| T4 | dual-era + mixed sources | `test/boundary` | `[x]` |
| T5 | policy | `internal/policy`, facade policy tests | `[x]` |
| T6 | control plane + ADS CDS/RDS/LDS + ACK/NACK | `internal/controlplane`, `internal/xds`, `extauthz`, `ratelimit` | `[x]` |
| T7 | resilience/concurrency | `internal/facade/resilience_test.go` | `[x]` |
| T8 | Envoy runtime | `scripts/t8.ps1` | `[x]` HTTP+TLS PASS（Envoy 1.40.2） |

### 6.3 命令

```powershell
cd E:\Agent_Project\gateway\dual-era-mcp-gateway
powershell -File scripts/test.ps1
go vet ./...
```

本机限制：

- 无 gcc/clang → 本地不能 `-race`；CI 已配置 Linux race。
- T8 需 Docker named-pipe 全权限；基础拓扑已 PASS，TLS/SSE/ratelimit 仍另行验证。
- 无 Redis/K8s → 分布式限流/CRD/集成未验证。

### 6.4 验收覆盖

- 已覆盖：AC-02/03/04/05/06/07/08/09/10/12/13/14/15/16/18/19（子集）。
- 未覆盖/需 Envoy：AC-01/11/17/20/21/22/23/24/25/26/27/28/29/30。
- 详细映射在 traceability。

---

## 7. 风险登记

| 风险 | 概率 | 影响 | 缓解 | 当前 |
|---|---|---|---|---|
| Envoy MCP filters WIP/不稳定 | 高 | 高 | 锁 digest；Gate 1；fallback ext_proc | 已锁 dev digest |
| LDS 变更导致 SSE 抖动 | 高 | 高 | `mcp_multicluster`/CDS；drain；测试 | 未测 |
| `mcp_router` 不支持 modern | 高 | 高 | B1 facade；modern upstream Phase 2 | 已规避 |
| 单 listener bridge+router 不兼容 | 高 | 中 | 双端点/独立 listener/Go adapter | 已规避 |
| xDS 学习曲线 | 高 | 中 | public CDS/RDS/LDS；internal 仍静态 | public 已测 |
| 环境缺 Envoy binary/gcc | 中 | 高 | Docker T8 + CI race | T8 已跑；本机 race 仍走 CI |
| 范围膨胀 | 高 | 高 | WBS/Gate/Out of Scope | 需持续控制 |
| 只有库没有运行时 | 中 | 高 | T8/集成列为 P0；README 诚实 | 当前主要风险 |
| 观测/安全未做 | 高 | 高 | Gate 7；先做最小指标/日志 | 未开始 |
| GitHub 凭据失败 | 中 | 低 | 用户本机 push | 本地 ahead |
| 27 届时间不足 | 中 | 高 | Gate 裁剪；先保 T0–T7 + 可讲边界 | 需规划 |

---

## 8. Runbook（当前可执行）

### 8.1 本地双端点

```powershell
# terminal 1/2
go run ./cmd/legacy-mock -addr :9001 -name weather -tools get_weather,list_cities
go run ./cmd/legacy-mock -addr :9002 -name crm -tools get_customer

# terminal 3
go run ./cmd/gateway -addr :8080 -legacy "weather=http://127.0.0.1:9001/mcp,crm=http://127.0.0.1:9002/mcp"
```

- `/mcp`：legacy initialize/list/call。
- `/modern/mcp`：需 `_meta` + 三个 header。
- 注意：`cmd/gateway` 当前未接策略；策略只在 facade 单测/库级验证。

### 8.2 控制面 + xDS

```powershell
go run ./cmd/controlplane -addr :18080 -xds-addr :18000
```

- `GET /api/v1/status`：config version + xds version。
- `POST /api/v1/apply`：应用 `controlplane.Config`。
- `POST /api/v1/rollback`：按 version 回滚。
- `GET /api/v1/envoy`：生成 draft 拓扑。
- `:18000`：ADS CDS/RDS/LDS（public listener）。

### 8.3 回滚

1. `GET /api/v1/revisions` 找 version。
2. `POST /api/v1/rollback {"version":"sha256:..."}`。
3. 确认 `/api/v1/status` 的 `version`/`xds_version`。
4. 真实 Envoy 环境还要看 ACK/NACK；当前只有模型，没有 Envoy callback。

### 8.4 T8（有 Envoy/Docker 时）

```powershell
powershell -File scripts/envoy-validate.ps1
```

- 需要 `envoy` 在 PATH；
- 当前只验证 `deploy/envoy/bootstrap.yaml`；
- 完整拓扑（public/internal/mcp_router/REST bridge）仍要在 Docker/kind 中补。

---

## 9. 交付物清单

| 交付物 | 状态 |
|---|---|
| GitHub repo + commit history | `[x]` 本地；远端待 push |
| README（快速开始/架构/限制） | `[x]` |
| B1 设计/需求/协议/工具源/Provider | `[x]` |
| 验收测试 T0–T8 | `[x]` T8 PASS |
| 测试报告/覆盖率 | `[x]` |
| PRD traceability | `[x]` |
| CI（build/vet/test；race on Linux） | `[~]` 无 lint/集成 |
| Envoy bootstrap/生成配置 | `[~]` 草案 |
| Helm/kind/CRD | `[~]` Helm 骨架；无 kind/CRD/controller |
| Grafana/告警/Trace | `[~]` starter dashboard/rules + trace id；未部署 |
| 压测/混沌/安全报告 | `[~]` 微基准 + k6 脚本 + SSRF/安全测试；无真实压测/chaos |
| ADR-001..007 | `[x]` 7 篇 |
| 5 分钟 demo | `[~]` `docs/demo.md`；未录视频 |
| 简历 bullet/面试问答 | `[x]` `docs/resume.md` |

---

## 10. 关键决策与开放问题

### 10.1 已决

| ID | 决策 | 原因 |
|---|---|---|
| D-001 | B1 双端点 | terminal `mcp_router` 与 modern 冲突 |
| D-002 | public Envoy 只做路径分流 | 避免 router 挡住 modern |
| D-003 | internal Envoy 拥有 legacy 聚合 | 复用 `mcp_router` |
| D-004 | modern facade 用 Go | 2026-07-28 聚合/校验可测 |
| D-005 | native + REST 归一化 | 一套 catalog/策略 |
| D-006 | Provider 与 ToolSource 分层 | K8s/Nacos 不污染 facade |
| D-007 | 先 public CDS/RDS/LDS，internal listener 后补 | 降低 xDS 风险 |
| D-008 | TDD 边界测试 | 完成标准可验证 |

### 10.2 未决（Gate 6/7 前）

1. LDS 变更是否真的 drain？SSE 允许一次重连还是必须零中断？
2. internal `mcp_router` listener 是否迁入 xDS，还是长期保持静态拓扑？
3. ext_authz/ratelimit 是独立 Go 服务，还是先 Envoy local policy？
4. Redis 限流是否进入 MVP，还是保持进程内并写明单副本限制？
5. K8s CRD/controller 是否值得在 27 届时间内做，还是用文件配置 + 管理 API？
6. modern MCP upstream 是否 Phase 2 永久不做，还是用 facade 直连？
7. 可观测最小集：Prometheus + JSON log + 一个 Trace 后端，还是只做指标/日志？
8. 是否引入官方 MCP Go SDK 做 conformance oracle？

---

## 11. 下一阶段行动（按优先级）

### P0：让“运行时”可信

1. 把 `internal/policy` 接入 `cmd/gateway`（API key/allowlist/限流）。
2. T8 已跑通 public path + internal `mcp_router`；下一步补 TLS/SSE/ratelimit/Redis 覆盖。
3. 补 TLS/HTTP2、body/header 限制、SSE。
4. 实现 ext_authz/ratelimit gRPC（先最小，再接 Envoy）。
5. 补 internal listener LDS，或明确长期保持静态拓扑。

### P1：平台纵向切片

6. K8s CRD/controller/admission（或只做文件配置 + 管理 API，明确取舍）。
7. 观测：Prometheus 指标 + JSON access log + Trace。
8. Redis 限流、JWT、429 `Retry-After`。
9. Helm/kind/docker-compose + runbook。

### P2：扩展

10. stdio 进程池/自动重启。
11. 复杂 REST adapter（multipart/二进制/长任务；OAuth/HMAC 已做）。
12. modern MCP upstream。
13. canary、审计、多租户物理隔离。

---

## 12. 变更记录

| 日期 | 版本 | 变更 |
|---|---|---|
| 2026-11-24 | v1 | 初始任务清单；B1 双端点；T0–T7；xDS deferred |
| 2026-11-25 | v2 | 按 PRD 重写：架构/职责/端口/WBS/Gate/追踪/测试/风险/Runbook；补充 K8s/Nacos/REST adapter/ADS/CDS 状态；明确 T8/race 环境阻塞与“库级完成≠生产完成” |
| 2026-11-25 | v3 | P2 增量：ext_authz/ratelimit gRPC、gateway 策略接线、public CDS/RDS/LDS、ACK/NACK callback、`/metrics`/trace、T8/Helm harness、REST SSRF allowlist、facade 微基准；T8 运行仍阻塞 |
| 2026-11-25 | v4 | T8 实跑 PASS（Envoy 1.40.2，Docker Desktop）：修 gcr.io 基础镜像、PowerShell JSON body、启动竞态与 teardown 退出码；补 `docs/t8-report.md` |
| 2026-11-25 | v5 | T8 增加 Envoy ratelimit filter：70 次调用 60×200 / 10×429；修 ext_authz 删除 `x-tenant-id` 导致 descriptor 为空；补 catalog fanout 重试 |
| 2026-11-25 | v6 | T8 `-TLS` PASS：TLS1.3 + ALPN h2，HTTP/TLS 两种模式都覆盖 initialize/list/ratelimit |
| 2026-11-25 | v7 | 新增 `cmd/h2check`：T8 TLS 下实际 HTTP/2 initialize 返回 200 HTTP/2.0；C-05 完成 |
| 2026-11-25 | v8 | T8 加 Redis：policy `-redis-addr`，70 次 60×200/10×429，`DBSIZE=1`；镜像改 `Dockerfile.prebuilt` 规避 Docker 构建期 Go 模块网络阻塞；记录 SSE GET 405 |
| 2026-11-25 | v9 | T8 加 `mcp_json_rest_bridge` 内部 listener：`rest__get_customer` 聚合与调用 PASS；REST bridge 不再列为未覆盖 |
| 2026-11-25 | v10 | T8 加真实压测：10 workers/5s → 3,114 RPS，p95 4.80ms，0 error；记录为单机 Docker 证据，不是 500RPS/10min SLA |
| 2026-11-25 | v11 | T8 chaos 实跑：停 crm 后 partial catalog 200、dead call 500、恢复；停 policy 后 403 fail-closed |
| 2026-11-25 | v12 | T8 chaos 增加 Redis 停机：ratelimit fallback 后请求仍 200，验证 fail-open |
| 2026-11-25 | v13 | 增加 `cmd/sse-shim`：public GET `/mcp` + `/mcp/message` 走 shim，T8 `ssecheck` PASS（HTTP/TLS）；raw mcp_router GET 仍 405 |