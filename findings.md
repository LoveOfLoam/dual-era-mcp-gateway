# Findings

## From Gate 1 spike

- `mcp_router` + `mcp_multicluster` works for legacy aggregation; `servers: []` keeps the backend list in CDS.
- `mcp_router` does **not** support 2026-07-28 modern upstream: `server/discover` 400, headers not forwarded.
- `mcp_json_rest_bridge` supports 2026-07-28 on 1.41-dev: `_meta`, header validation, `server/discover`, `ttlMs/cacheScope`, `Mcp-Param-*`.
- Single listener `mcp_json_rest_bridge` + terminal `mcp_router` does not work in global mode.
- Path C: REST bridge on an internal listener, aggregated by `mcp_router`, gives a unified legacy catalog.
- `mcp_router.tools/list` order is non-deterministic; facade/registry must sort.
- `mcp_router` single backend does not prefix tools; multi-backend does.
- REST bridge forwards downstream `Authorization`/`x-tenant-id` by default; must remove/allowlist.
- 1.40.2 lacks `session_signing_key`, `mcp_param_headers`, cache fields, clientCapabilities validation.
- 1.41.0-dev is the only tested version with the full P0 set; it is WIP and must be pinned by digest.

## Product decision

- B1: versioned endpoints, not single endpoint.
- `/mcp` legacy; `/modern/mcp` modern.
- Native MCP and REST are both first-class tool sources.
- Providers are separate from tool sources: static/OpenAPI now, K8s/Nacos later.

## Environment constraints

- No `envoy` binary installed.
- Docker daemon is not accessible from the current sandbox (`permission denied`).
- Go 1.26.4, Git 2.48.1, Python 3.12 available.
- MVP tests must run with Go `httptest` and mock servers; Envoy validation is optional T8.

## Design implications

- The modern facade must not depend on `mcp_router` supporting modern; it speaks 2026-07-28 itself.
- The internal legacy layer normalizes native MCP and REST into one MCP catalog.
- Tool names are `backend__tool`; single-backend behavior must be normalized by the registry, not left to `mcp_router`.
- `server/discover` must advertise only supported versions; do not blindly copy upstream.
- Header/body consistency must be enforced before forwarding.