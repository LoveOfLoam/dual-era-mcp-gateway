# Requirements

## 1. Scope

B1 dual-era MCP gateway with versioned endpoints:

- `/mcp` legacy (2025-xx).
- `/modern/mcp` modern (2026-07-28).
- Native MCP and REST tool sources.
- Static/OpenAPI providers in MVP; K8s/Nacos interfaces only.

## 2. Functional requirements

### 2.1 Modern endpoint

| ID | Requirement |
|---|---|
| FR-M-001 | Accept only POST on `/modern/mcp`; reject GET/DELETE with 405. |
| FR-M-002 | Implement `server/discover` with supportedVersions `["2026-07-28"]`, tools capability, serverInfo, `ttlMs`, `cacheScope`. |
| FR-M-003 | Require `params._meta.io.modelcontextprotocol/protocolVersion == "2026-07-28"`. |
| FR-M-004 | Require `params._meta.io.modelcontextprotocol/clientCapabilities` object; missing → JSON-RPC `-32602`. |
| FR-M-005 | Require headers `MCP-Protocol-Version`, `Mcp-Method`, `Mcp-Name` where applicable. |
| FR-M-006 | Header/body inconsistency → HTTP 400 + `-32020`. |
| FR-M-007 | Unsupported version → HTTP 400 + `-32022`. |
| FR-M-008 | `tools/list` returns aggregated tools, deterministic sort, `ttlMs`, `cacheScope`. |
| FR-M-009 | `tools/call` routes by tool name and returns `resultType: "complete"`. |
| FR-M-010 | Never emit `Mcp-Session-Id`; never accept `initialize` as a modern method. |
| FR-M-011 | Reject non-JSON / invalid JSON with `-32700` / `-32600`. |
| FR-M-012 | Unknown tool → `-32601`. |

### 2.2 Legacy endpoint

| ID | Requirement |
|---|---|
| FR-L-001 | `/mcp` supports `initialize` + `notifications/initialized`. |
| FR-L-002 | Reuse `Mcp-Session-Id` for subsequent calls. |
| FR-L-003 | `tools/list` aggregates native MCP + REST sources. |
| FR-L-004 | `tools/call` routes by prefix. |
| FR-L-005 | Upstream 404 on session → re-initialize once. |
| FR-L-006 | Legacy behavior is Envoy/internal-router compatible. |

### 2.3 Tool sources

| ID | Requirement |
|---|---|
| FR-S-001 | `ToolSource` interface: ListTools, Call, Health. |
| FR-S-002 | Native MCP source: legacy HTTP MCP; modern direct client Phase 2. |
| FR-S-003 | REST source: simple JSON REST via bridge; complex via adapter interface Phase 2. |
| FR-S-004 | Normalize all sources to `backend__tool`; reject backend names containing `__`. |
| FR-S-005 | Deterministic tool order: sort by name. |
| FR-S-006 | Tool name collision → config validation error. |
| FR-S-007 | Partial source failure must not hide healthy sources; report stale. |

### 2.4 Providers

| ID | Requirement |
|---|---|
| FR-P-001 | `Provider` interface: Load, Watch. |
| FR-P-002 | Static YAML provider in MVP. |
| FR-P-003 | OpenAPI provider in MVP: operationId → tool, parameters/requestBody → inputSchema. |
| FR-P-004 | K8s/Nacos providers are interfaces only; no implementation in MVP. |
| FR-P-005 | Provider reload updates catalog without restart. |

### 2.5 Policy

| ID | Requirement |
|---|---|
| FR-POL-001 | API key via `Authorization: Bearer` or `X-API-Key`; SHA-256 only. |
| FR-POL-002 | Tenant derived from credential; ignore client `x-tenant-id`. |
| FR-POL-003 | Tool allowlist per tenant; denied → 403. |
| FR-POL-004 | Rate limit per tenant/tool; over → 429 with `error.layer`. |
| FR-POL-005 | No secrets in logs. |

### 2.6 Control plane

| ID | Requirement |
|---|---|
| FR-C-001 | Load + validate config; reject unknown refs/collisions. |
| FR-C-002 | Version = SHA-256 of normalized config. |
| FR-C-003 | Snapshot with sources, tools, policies, routes. |
| FR-C-004 | Rollback to previous version. |
| FR-C-005 | Generate Envoy public/internal listener + clusters. |
| FR-C-006 | Optional xDS server; generated config fallback documented. |

### 2.7 Observability

| ID | Requirement |
|---|---|
| FR-O-001 | Structured logs with trace_id, tenant, tool, source, status. |
| FR-O-002 | Prometheus metrics: requests, errors, rate-limited, source health. |
| FR-O-003 | Health endpoints `/healthz`, `/readyz`. |

## 3. Non-functional

- Go 1.26+, Windows/Linux.
- `go test ./... -race` green.
- No external service required for T0–T7.
- Deterministic behavior; no map-order output.
- Timeouts on all upstream calls.
- Graceful shutdown.

## 4. Out of scope

- B2 single endpoint.
- Full OAuth/OIDC.
- Redis.
- K8s/Nacos provider implementation.
- Complex REST adapter implementation.
- MCP resources/prompts/subscriptions/tasks/MRTR.