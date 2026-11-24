# Resume bullets and interview notes

## 简历 bullet（可直接改写）

- 设计并实现 **dual-era MCP 网关**：按版本拆成 legacy `/mcp`（Envoy `mcp_router`）与 modern
  `/modern/mcp`（Go 2026-07-28 facade），避免把旧入口包装成“现代协议支持”。
- 用 **Envoy + Go 控制面** 分离数据面/控制面：public CDS/RDS/LDS + ACK/NACK、
  配置 SHA-256 版本、history/rollback、dry-run diff。
- 抽象 **Provider / ToolSource** 两层：static、OpenAPI、K8s manifest、Nacos Config，
  native MCP、REST、stdio 统一成 `ListTools`/`Call`，工具名 `source__tool` 前缀、
  排序、冲突校验。
- 实现 **策略运行时**：ext_authz gRPC（API key/HS256/RS256 JWKS JWT、tenant、tool allowlist）、
  ratelimit gRPC（内存 + Redis Lua fixed window、429 JSON-RPC body/Retry-After）、
  JSONL 审计；Redis 故障走 fail-open + 内存回退。
- 实现 **REST → MCP adapter**：path/query/JSON body、OAuth2 client credentials、
  HMAC 签名、metadata host SSRF 拒绝、allowedHosts。
- 建立 **TDD/spec-driven** 证据链：T0–T8 边界、race CI、覆盖率、WBS/Gate/FR/AC
  traceability；T8 用真实 Docker/Envoy harness：path split、ext_authz、Redis ratelimit、
  REST bridge、legacy SSE shim、TLS/HTTP2、mTLS、100 次 xDS CDS/RDS churn + ACK、
  load 与 chaos 都有运行证据；环境不可用时明确 SKIP 而非伪造 PASS。
- 可观测：`/metrics`、结构化日志、`X-Trace-Id`/`traceparent` 传播、Grafana/Prometheus
  starter 资产。

## 面试问答（简短版）

**Q1：为什么不做单端点，而做 `/mcp` + `/modern/mcp`？**
Envoy `mcp_router` 是 terminal filter，不能在同一 listener 上处理 2026-07-28 的
`server/discover`/`_meta`/无 session 语义；单端点要么把现代请求塞进 legacy，要么让
modern 依赖 WIP filter。B1 是诚实且可测试的折中；B2 单端点是后续设计。

**Q2：控制面和数据面怎么分？**
Envoy 负责 TLS/L7/auth/路由/legacy 聚合；Go 负责 modern 协议、Provider 编译、
版本/回滚、xDS 资源生成。控制面不转发业务请求；数据面不读配置文件。

**Q3：为什么 `mcp_multicluster`？**
`mcp_router` 的 `servers` 是静态列表；用 `mcp_multicluster` 后，后端列表变化可以只改
CDS，避免每次加 MCP server 都改 listener。

**Q4：xDS 做到什么程度？**
public listener 的 CDS/RDS/LDS + ACK/NACK；internal `mcp_router` 仍静态。没有冒充
完整 LDS/EDS/canary。

**Q5：Redis 限流为什么 fail-open？**
限流服务不可用时，鉴权仍 fail-closed；限流选择 fail-open + 内存回退，避免 Redis
故障放大为全站不可用。这个取舍写进了 ADR/测试。

**Q6：REST 桥的 SSRF 怎么防？**
metadata host 拒绝；`allowedHosts` 白名单；不允许用户随意指定 URL；下游凭据
不默认透传（T8 route `request_headers_to_remove` + ext_authz 移除）。

**Q7：T8 跑了吗？**
跑了。Docker Desktop + Envoy 1.40.2 上 `scripts/t8.ps1` / `-TLS` / `t8-mtls.ps1` /
`t8-xds.ps1` / `t8-load.ps1` / `t8-chaos.ps1` 都 PASS；raw `mcp_router` GET/SSE
仍是 405，公开 SSE 由 `cmd/sse-shim` 承担。T8 是运行证据，不是 SLA。

**Q8：stdio bridge 怎么做的？**
子进程 stdin/stdout newline-delimited JSON-RPC；ID→pending channel 路由；HTTP
`initialize`/`tools/list`/`tools/call` 映射；单进程，无进程池/自动重启（Phase 2）。

**Q9：JWT 做了什么？**
验证 issuer/audience/exp/nbf/tenant claim；HS256 用共享密钥，RS256 支持 JWKS
（5 分钟缓存）。没有做生产级 OIDC 的密钥轮换、吊销和全量 claim 策略，避免把
“有 JWT 代码”说成“生产级 OIDC”。

**Q10：这个项目最大的限制？**
SSE 跨 churn 未做；K8s CRD informer/controller、
HA/leader election、canary、Redis HA 未做。10 分钟单机 T8 已跑（~835 RPS、p99 3.74ms、
0 error），但仍是单 Docker 主机、单副本、`tools/list` 流量，不是生产 SLA。
T8 的 ext_authz/Redis ratelimit/SSE/mTLS/xDS LDS-CDS-RDS churn + NACK 是真运行证据。

## 不要写进简历的表述

- “支撑 500 RPS / p99 < 10ms”——没有真实压测数据。
- “生产级 K8s controller/CRD”——只有 Helm 骨架和文件 Provider。
- “完整 MCP 2026-07-28 全特性”——只覆盖 discover/list/call/_meta/headers 的核心路径。
- “T8 等于生产可用”——T8 是单机 Docker harness，不是 HA/SLA；raw `mcp_router` SSE 仍 405。