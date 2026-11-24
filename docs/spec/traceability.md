# PRD → Implementation Traceability

Source PRD: [`../../envoy-mcp-gateway-plan-a-prd.md`](../../envoy-mcp-gateway-plan-a-prd.md)

Status rules:

- **DONE**：代码 + Go 测试；能在本机复现。
- **PARTIAL**：库/子集/仅设计；未接运行路径或未做运行时验证。
- **TODO**：未开始。
- **BLOCKED**：依赖 Envoy/Docker/K8s/Redis/凭据，当前环境不可执行。
- **N/A**：B1/Phase 1 明确不做。

> 原则：不把“设计文档写了”算 DONE；不把“库里有函数”算运行时完成。

## 0. 摘要

| 组 | DONE | PARTIAL | TODO | BLOCKED | N/A |
|---|---:|---:|---:|---:|---:|
| FR-CP 控制面（17） | 2 | 6 | 9 | 0 | 0 |
| FR-DP 数据面（18） | 2 | 6 | 10 | 0 | 0 |
| FR-POL 策略面（12） | 4 | 3 | 5 | 0 | 0 |
| FR-TR Registry（7） | 3 | 2 | 2 | 0 | 0 |
| FR-STD stdio（7） | 0 | 0 | 7 | 0 | 0 |
| FR-REST（10） | 2 | 5 | 3 | 0 | 0 |
| FR-OBS 可观测（9） | 0 | 0 | 9 | 0 | 0 |
| FR-SEC 安全（10） | 2 | 3 | 5 | 0 | 0 |
| FR-OPS 运维（8） | 2 | 2 | 4 | 0 | 0 |
| FR-TS 工具源（6） | 5 | 1 | 0 | 0 | 0 |
| NFR（6） | 0 | 4 | 2 | 0 | 0 |
| AC-01..30（30） | 14 | 6 | 10 | 0 | 0 |

## 1. FR-CP 控制面

| ID | 需求 | 实现/证据 | 状态 |
|---|---|---|---|
| FR-CP-01 | 6 个 CRD | 无 `deploy/crds`；只有 `api/spec/source-v1alpha1.yaml` | TODO |
| FR-CP-02 | Controller watch | 无 controller-runtime | TODO |
| FR-CP-03 | Admission 静态校验 | `internal/controlplane.Validate` 有语义校验，非 webhook | PARTIAL |
| FR-CP-04 | DesiredSnapshot（CDS/EDS/RDS/LDS） | `internal/xds` 只生成 CDS；`internal/envoy` 生成文本拓扑 | PARTIAL |
| FR-CP-05 | ADS 下发 | `internal/xds` + `cmd/controlplane`；CDS streaming test | PARTIAL |
| FR-CP-06 | ACK/NACK | `internal/controlplane.AckStore` 模型；未接 go-control-plane callback | PARTIAL |
| FR-CP-07 | SHA-256 version | `manager.go` + tests | DONE |
| FR-CP-08 | 历史/回滚 | `manager.go` + tests；API 有 rollback | DONE |
| FR-CP-09 | Canary | 无 | TODO |
| FR-CP-10 | status conditions | 无 CRD status | TODO |
| FR-CP-11 | 管理 API | `cmd/controlplane`：status/config/revisions/apply/rollback/envoy；无 dataplanes/tools/metrics/auth | PARTIAL |
| FR-CP-12 | 2 副本 + leader election | 无 | TODO |
| FR-CP-13 | 配置审计 | 无 | TODO |
| FR-CP-14 | Tool Registry 服务 | `internal/registry` 有 catalog；无 `/api/v1/tools` 服务 | PARTIAL |
| FR-CP-15 | 控制面指标 | 无 Prometheus | TODO |
| FR-CP-16 | dry-run diff | 无 | TODO |
| FR-CP-17 | mTLS/RBAC | 无 | TODO |

## 2. FR-DP 数据面

| ID | 需求 | 实现/证据 | 状态 |
|---|---|---|---|
| FR-DP-01 | TLS listener 8443/HTTP2 | 无真实 Envoy listener | TODO |
| FR-DP-02 | Filter chain | `internal/envoy` 生成 mcp + router 草案；无 ext_authz/ratelimit | PARTIAL |
| FR-DP-03 | MCP filter 解析/VERIFY | Go facade 做 header/body 校验；非 Envoy filter | PARTIAL |
| FR-DP-04 | `mcp_router` 聚合 2 后端 | Go aggregator 测试通过；Envoy 未跑 | PARTIAL |
| FR-DP-05 | `tools/call` 前缀路由 | `aggregator` + tests | DONE |
| FR-DP-06 | REST bridge | Go `restadapter` + tests；Envoy `mcp_json_rest_bridge` 未跑 | PARTIAL |
| FR-DP-07 | EDS/health | 无 | TODO |
| FR-DP-08 | outlier/circuit | 无 | TODO |
| FR-DP-09 | retry 安全方法 | 无 | TODO |
| FR-DP-10 | SSE 不 buffer | 无 | TODO |
| FR-DP-11 | Access log JSON | 无 | TODO |
| FR-DP-12 | OTel Trace | 无 | TODO |
| FR-DP-13 | Admin 仅内网 | 无 | TODO |
| FR-DP-14 | 优雅 drain | 无 | TODO |
| FR-DP-15 | `mcp_multicluster` CDS | `internal/envoy` 生成 `mcp_multicluster`；未运行 | PARTIAL |
| FR-DP-16 | body/header 限制 | facade `MaxBodyBytes` + 413 test；header 限制未做 | PARTIAL |
| FR-DP-17 | Origin 校验 | facade `originAllowed` + test | DONE |
| FR-DP-18 | local rate limit | 无 | TODO |

## 3. FR-POL 策略面

| ID | 需求 | 实现/证据 | 状态 |
|---|---|---|---|
| FR-POL-01 | API Key SHA-256 | `internal/policy` + tests | DONE |
| FR-POL-02 | JWT | HS256/RS256 + JWKS 缓存，iss/aud/exp/nbf 测试；无密钥轮换 | DONE（Go 层） |
| FR-POL-03 | tenant 只从凭证 | `Authenticate` 忽略 `x-tenant-id` + test | DONE |
| FR-POL-04 | allowlist | `Authorize` + tests | DONE |
| FR-POL-05 | RPM + burst/Redis | 进程内 + Redis Lua（miniredis 测试）；未做真实 Redis 压测 | PARTIAL |
| FR-POL-06 | 429 body/layer/Retry-After | layer 有；无 Retry-After/Envoy local reply | PARTIAL |
| FR-POL-07 | 审计 | 无 | TODO |
| FR-POL-08 | 失败模式 | auth 库内 fail-closed；无 Redis 降级 | PARTIAL |
| FR-POL-09 | 策略指标 | 无 | TODO |
| FR-POL-10 | 并发限制 | 无 | TODO |
| FR-POL-11 | 无状态/水平扩展 | 库无状态；无 gRPC 服务 | TODO |
| FR-POL-12 | 不记录 token | 代码无 token 日志 + 测试无泄漏断言 | DONE |

## 4. FR-TR Tool Registry

| ID | 需求 | 实现/证据 | 状态 |
|---|---|---|---|
| FR-TR-01 | 拉取 tools/list | `legacy.HTTPClient` + `modernclient`（discover/list/call） | DONE（Go 层） |
| FR-TR-02 | `backend__tool`/冲突 | `registry.Merge` + tests | DONE |
| FR-TR-03 | 确定性排序 | `Catalog.List` sort + tests | DONE |
| FR-TR-04 | schema/description/协议/健康 | Tool 有 schema/description/source；无健康状态 | PARTIAL |
| FR-TR-05 | deprecated/replacedBy | 无 | TODO |
| FR-TR-06 | 后端失败不阻塞 | `aggregator` partial failure test | DONE |
| FR-TR-07 | drift 检测 | 无 | TODO |

## 5. FR-STD stdio bridge

| ID | 需求 | 状态 |
|---|---|---|
| FR-STD-01 | stdio 子进程管理 | TODO |
| FR-STD-02 | `/mcp`/`/sse` | TODO |
| FR-STD-03 | 进程池/id 映射 | TODO |
| FR-STD-04 | 崩溃重启 | TODO |
| FR-STD-05 | allowlist/非 root/资源限制 | TODO |
| FR-STD-06 | 优雅 drain | TODO |
| FR-STD-07 | 不记录完整参数 | TODO |

## 6. FR-REST

| ID | 需求 | 实现/证据 | 状态 |
|---|---|---|---|
| FR-REST-01 | SourceSpec → tool 配置 | `source.ToolSpec`/OpenAPI + tests | PARTIAL |
| FR-REST-02 | call → HTTP 映射 | `restadapter` GET/POST + tests | DONE |
| FR-REST-03 | 2026-07-28 bridge | facade 支持；Envoy bridge 未跑 | PARTIAL |
| FR-REST-04 | Secret 注入/不透传 | `Headers` 支持；无 Secret；无下游 credential strip 测试 | PARTIAL |
| FR-REST-05 | URL allowlist/SSRF | 无 | TODO |
| FR-REST-06 | `Mcp-Param-*` | 无 Envoy bridge | TODO |
| FR-REST-07 | 流式转码 | 无 | TODO |
| FR-REST-08 | OpenAPI importer | `source.OpenAPIProvider` 最小版；无 $ref/security | PARTIAL |
| FR-REST-09 | Go adapter 可聚合 | `restadapter` + tests | DONE |
| FR-REST-10 | 明确不支持子集 | `tool-sources.md` 记录；无运行时拒绝 | PARTIAL |

## 7. FR-OBS

| ID | 需求 | 状态 |
|---|---|---|
| FR-OBS-01..09 | 控制面/Envoy/策略指标、日志、Trace、看板、告警、审计 | TODO（全部未做） |

## 8. FR-SEC

| ID | 需求 | 实现/证据 | 状态 |
|---|---|---|---|
| FR-SEC-01 | TLS/xDS mTLS | 无 | TODO |
| FR-SEC-02 | Origin 403 | facade + test | DONE |
| FR-SEC-03 | body/header/深度/重复 key | facade body；其余无 | PARTIAL |
| FR-SEC-04 | REST SSRF | 无 | TODO |
| FR-SEC-05 | Secret/hash/脱敏 | API key hash；无 Secret 集成 | PARTIAL |
| FR-SEC-06 | 管理 API 鉴权 | 无 | TODO |
| FR-SEC-07 | Envoy admin 内网 | 无 | TODO |
| FR-SEC-08 | 默认拒绝 | `Authorize` 无规则即 403 + test | DONE |
| FR-SEC-09 | 下游凭证隔离 | REST adapter 只用 source headers；无 strip 测试 | PARTIAL |
| FR-SEC-10 | 审计防篡改 | 无 | TODO |

## 9. FR-OPS

| ID | 需求 | 实现/证据 | 状态 |
|---|---|---|---|
| FR-OPS-01 | kind `make up` | 无 | TODO |
| FR-OPS-02 | Helm | 无 | TODO |
| FR-OPS-03 | CI | `.github/workflows/test.yml`：build/vet/test/race；无 lint/integration | PARTIAL |
| FR-OPS-04 | Runbook | `task_plan.md` §8 + README；无完整故障手册 | PARTIAL |
| FR-OPS-05 | 升级兼容 | 无 | TODO |
| FR-OPS-06 | 历史/回滚 | `controlplane.Manager` + API + tests | DONE |
| FR-OPS-07 | HPA/PDB | 无 | TODO |
| FR-OPS-08 | 不提交二进制/密钥 | `.gitignore` + repo 检查 | DONE |

## 10. FR-TS

| ID | 需求 | 实现/证据 | 状态 |
|---|---|---|---|
| FR-TS-01 | Provider Load/Watch | static/OpenAPI/K8s/Nacos + tests | DONE |
| FR-TS-02 | ToolSource native/REST | legacy client + restadapter + tests | DONE |
| FR-TS-03 | 归一化/冲突 | registry + tests | DONE |
| FR-TS-04 | 单源也带前缀 | aggregator single-source test | DONE |
| FR-TS-05 | OpenAPI 3.0/3.1 | 最小版；无 $ref/security | PARTIAL |
| FR-TS-06 | K8s/Nacos 同接口 | manifest/Config Service + tests | DONE |

## 11. NFR

| ID | 目标 | 状态 |
|---|---|---|
| NFR-PERF-01..06 | 500 RPS、p99<10ms、SSE、收敛<5s、CPU | TODO（无压测） |
| NFR-REL-01..06 | 2 副本、断流、outlier、坏配置、Redis、滚动 | PARTIAL（坏配置保持上一版/回滚有） |
| NFR-SEC | TLS/mTLS/最小权限/扫描 | PARTIAL（Origin/hash/default deny） |
| NFR-OBS | trace_id/指标基数/看板/告警 | TODO |
| NFR-COMPAT | Envoy/MCP/Go/K8s 版本 | PARTIAL（Go/MCP；Envoy/Gate 1） |
| NFR-MAINT | main<100、依赖注入、覆盖率 60%+ | PARTIAL（覆盖率有报告；无 lint） |

## 12. AC-01..AC-30

| AC | 场景 | 当前证据 | 状态 |
|---|---|---|---|
| AC-01 | `make up` | 无 kind/Helm | TODO |
| AC-02 | tools/list 2 后端 | boundary test | PARTIAL（Go 非 Envoy） |
| AC-03 | tools/call 路由 | boundary/aggregator | PARTIAL |
| AC-04 | 未知工具 -32601 | facade/aggregator tests | DONE |
| AC-05 | 无 key 401 | facade policy test | DONE |
| AC-06 | 错 key 401 | policy test | DONE |
| AC-07 | 无权限 403 | facade policy test | DONE |
| AC-08 | tenant 伪造 | policy test | DONE |
| AC-09 | 租户 RPM 429 | policy test | PARTIAL（无 Redis/Retry-After） |
| AC-10 | 工具 RPM 隔离 | policy test | DONE |
| AC-11 | 429 body/layer | facade test layer；无 Retry-After | PARTIAL |
| AC-12 | header/body 400/-32020 | facade tests | DONE |
| AC-13 | 版本 400/-32022 | facade tests | DONE |
| AC-14 | server/discover | facade test | DONE |
| AC-15 | ttlMs/cacheScope | facade test | DONE |
| AC-16 | resultType complete | facade test | DONE |
| AC-17 | 配置变更 ACK <5s | 无真实 Envoy | TODO |
| AC-18 | 回滚 | controlplane test/API | PARTIAL（无 Envoy） |
| AC-19 | 坏配置拒绝 | controlplane test | DONE（无 admission） |
| AC-20 | 100 变更 + 500 RPS | 无 | TODO |
| AC-21 | 后端 5xx outlier | 无 | TODO |
| AC-22 | Redis down | 无 | TODO |
| AC-23 | ext_authz down | 无 | TODO |
| AC-24 | Origin 403 | facade test | DONE |
| AC-25 | body 413 | facade test | DONE |
| AC-26 | SSRF | 无 | TODO |
| AC-27 | 滚动更新 | 无 | TODO |
| AC-28 | 控制面挂 | 无 | TODO |
| AC-29 | NACK 注入 | AckStore model only | PARTIAL |
| AC-30 | 审计 | 无 | TODO |

## 13. Test evidence

| 命令 | 结果 |
|---|---|
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| `internal/xds` ADS CDS/RDS/LDS + ACK/NACK | PASS |
| T8 Envoy 1.40.2 Docker topology | PASS (`docs/t8-report.md`) |
| `internal/extauthz` / `internal/ratelimit` | PASS |
| `scripts/test.ps1` | PASS；Envoy validate SKIP |
| `go test -race ./...` | BLOCKED locally（no CGO/gcc）；CI configured |
| T8 Envoy runtime | BLOCKED（no Envoy/Docker） |

## 14. 2026-11-25 P2 delta

The per-ID tables above are the v2 baseline. This delta records the P2 additions
without pretending the remaining gaps are closed:

| 需求 | 新增证据 | 状态变化 |
|---|---|---|
| FR-CP-05 ADS | `internal/xds` public CDS/RDS/LDS + ADS tests | PARTIAL → PARTIAL（public only） |
| FR-CP-06 ACK/NACK | `internal/xds/callbacks.go` + test | PARTIAL → PARTIAL（no live Envoy） |
| FR-DP-02 filter chain | T8 public Envoy `ext_authz` | PARTIAL（no TLS/ratelimit) |
| FR-POL-06 429 | `internal/ratelimit` body/layer/Retry-After | PARTIAL（no Envoy local reply） |
| FR-POL-06/08 | `cmd/gateway -policy` + ext_authz/ratelimit services | PARTIAL |
| FR-REST-05 SSRF | metadata host reject + allowedHosts | PARTIAL → DONE（adapter level） |
| FR-OBS-01/04/05 | `/metrics`, JSON logger, trace id | TODO → PARTIAL |
| FR-OPS-02 Helm | `deploy/helm/dual-era` skeleton | TODO → PARTIAL |
| FR-OPS-03 CI | race green; no lint/integration | PARTIAL |
| NFR-PERF | facade microbenchmark | TODO → PARTIAL（not 500 RPS） |

Still TODO/BLOCKED: T8 runtime, TLS/mTLS, SSE, Redis, CRD/controller,
internal `mcp_router` LDS, k6/chaos, demo, ADR files.

## 15. 结论

- B1 的**协议/聚合/策略/Provider/控制面/ADS/CDS/RDS/LDS** 在 Go 层有测试证据。
- Envoy/K8s/Redis/可观测/T8 仍是设计或配置草案；不能写成生产完成。
- 面试讲法：先讲“为什么双端点”和“哪些边界测试证明”，再主动讲“哪些没跑、为什么”。