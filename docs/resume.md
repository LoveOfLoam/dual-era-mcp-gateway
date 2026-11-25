# Resume bullets and interview notes

## 定位

> B1 dual-era MCP gateway vertical slice：Envoy 数据面 + Go 控制面，xDS 下发，
> legacy/modern 双端点，native MCP + REST 统一 catalog。
>
> 不是生产级网关；三个设计亮点有 T8 运行证据，生产化缺口单独列出。

## 简历 bullet（可直接改写）

- 设计并实现 **dual-era MCP 网关**：legacy `/mcp` 走 Envoy `mcp_router`，
  modern `/modern/mcp` 由 Go 2026-07-28 facade 处理；用双端点承接新旧客户端，
  不把旧入口包装成“现代协议支持”。
- 实现 **Envoy 数据面 + Go 控制面**：ADS 下发 public LDS/CDS/RDS，
  配置 SHA-256 版本、history/rollback/dry-run/audit；T8 实测 100 次
  LDS/CDS/RDS 变更、ACK/NACK、长连接 SSE 跨 churn。
- 抽象 **Provider / ToolSource**：native MCP、REST、stdio 统一为
  `ListTools`/`Call`；`source__tool` 前缀、排序、冲突校验；native MCP 与
  REST 共享一个 catalog。
- 实现 **REST → MCP**：path/query/JSON body、OAuth2 client credentials、
  HMAC、metadata host SSRF 拒绝、allowedHosts；Envoy
  `mcp_json_rest_bridge` 作为内部 REST backend 接入 `rest__*`。
- 实现 **策略运行时**：ext_authz（API key/tenant/allowlist、HS256/RS256 JWKS）、
  ratelimit（内存 + Redis Lua fixed window）；T8 实测 60×200 / 10×429，
  Redis 故障 fail-open 回退。
- 建立 **TDD/spec-driven 证据链**：T0–T7 边界测试、Go race CI、T8
  Docker/Envoy harness；TLS1.3/HTTP2、mTLS、SSE、Redis ratelimit、xDS
  churn、chaos、10 分钟压测都有运行证据；环境不足处明确标注 PARTIAL/BLOCKED。
- 可观测：`/metrics`、Envoy `/stats/prometheus`（T8 断言）、结构化日志、
  `X-Trace-Id`/`traceparent` 传播、Grafana/Prometheus starter 资产。

## 面试问答（简短版）

**Q1：为什么不做单端点，而做 `/mcp` + `/modern/mcp`？**
Envoy `mcp_router` 是 terminal filter，不能在同一 listener 上处理 2026-07-28 的
`server/discover`/`_meta`/无 session 语义；单端点要么把现代请求塞进 legacy，
要么让 modern 依赖 WIP filter。B1 是诚实且可测试的折中；B2 单端点是后续设计。

**Q2：控制面和数据面怎么分？**
Envoy 负责 TLS/L7/auth/路由/legacy 聚合；Go 负责 modern 协议、Provider 编译、
版本/回滚、xDS 资源生成。控制面不转发业务请求；数据面不读配置文件。

**Q3：为什么 `mcp_multicluster`？**
`mcp_router` 的 `servers` 是静态列表；用 `mcp_multicluster` 后，后端列表变化可以只改
CDS，避免每次加 MCP server 都改 listener。

**Q4：xDS 做到什么程度？**
public listener 的 LDS/CDS/RDS + ACK/NACK；T8 实测 100 次变更、live load
0 error、ACK 175、NACK 3143 + recovery、长连接 SSE 跨 churn。
internal `mcp_router` 仍静态；没有 EDS、多节点 xDS、控制面 leader election。

**Q5：Redis 限流为什么 fail-open？**
限流服务不可用时，鉴权仍 fail-closed；限流选择 fail-open + 内存回退，避免 Redis
故障放大为全站不可用。这个取舍写进了 ADR/测试。

**Q6：REST 桥的 SSRF 怎么防？**
metadata host 拒绝；`allowedHosts` 白名单；不允许用户随意指定 URL；下游凭据
不默认透传（T8 route `request_headers_to_remove` + ext_authz 移除）。

**Q7：T8 跑了吗？**
跑了。Docker Desktop + Envoy 1.40.2 上 `scripts/t8.ps1` / `-TLS` / `t8-mtls.ps1` /
`t8-xds.ps1` / `t8-load.ps1` / `t8-chaos.ps1` 都有结果；10 分钟单机 835 RPS、
p99 3.74ms、0 error。raw `mcp_router` GET/SSE 仍是 405，公开 SSE 由
`cmd/sse-shim` 承担。T8 是运行证据，不是 SLA。

**Q8：stdio bridge 怎么做的？**
子进程 stdin/stdout newline-delimited JSON-RPC；ID→pending channel 路由；HTTP
`initialize`/`tools/list`/`tools/call` 映射；单进程，无进程池/自动重启（Phase 2）。

**Q9：JWT 做了什么？**
验证 issuer/audience/exp/nbf/tenant claim；HS256 用共享密钥，RS256 支持 JWKS
（5 分钟缓存）。没有做生产级 OIDC 的密钥轮换、吊销和全量 claim 策略，避免把
“有 JWT 代码”说成“生产级 OIDC”。

**Q10：这个项目最大的限制？**
真实 K8s 集群验证、零丢失 HA、Redis Sentinel failover、控制面持久化/鉴权都未完成。
T8 停 1 个 gateway 仍有 3×503；Redis Sentinel 在 Docker Desktop 进入 `+tilt`；
CRD controller/leader election 只有 fake API 测试。10 分钟压测是单机 Docker，
不是生产 SLA。

## 三个亮点的边界

| 亮点 | 能讲什么 | 不能讲什么 |
|---|---|---|
| xDS 控制面 | public LDS/CDS/RDS、ACK/NACK、100 次 churn、长 SSE | “完整多节点 xDS / internal mcp_router LDS / EDS” |
| 双端点 | legacy + modern 同栈、TLS/HTTP2、SSE shim | “单端点 B2 / 完整 2026 conformance” |
| REST + native | `rest__*` 实际调用、统一 catalog、SSRF 防护 | “复杂 multipart/async REST / 生产级 REST bridge” |

## 不要写进简历的表述

- “支撑 500 RPS / p99 < 10ms 的生产 SLA”——只有单机 Docker 10 分钟 835 RPS 的证据。
- “生产级 K8s controller/CRD”——只有 fake API 测试，没有真实集群。
- “完整 MCP 2026-07-28 全特性”——只覆盖 discover/list/call/_meta/headers 核心路径。
- “零停机 HA”——当前停 1 个 gateway 有 3×503。
- “Redis HA / Sentinel failover”——当前被 Docker Desktop tilt 阻塞。
- “T8 等于生产可用”——T8 是单机 Docker harness，不是 HA/SLA。