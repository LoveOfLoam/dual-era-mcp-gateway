# ADR-002: Envoy legacy data plane + Go modern facade

- Status: accepted
- Date: 2026-11-24

## Context

Envoy's MCP filters are WIP. `mcp_router` is useful for legacy aggregation but
does not support modern upstreams; a Go implementation is easier to test for
`_meta`/header validation and result shaping.

## Decision

- Public Envoy: TLS/L7/auth/path split (TLS still TODO).
- Internal Envoy: legacy `mcp_router` + `mcp_multicluster`.
- Go facade: 2026-07-28 `server/discover`, `tools/list`, `tools/call`, `resultType`.

## Consequences

- Modern path does not depend on WIP Envoy filter behavior.
- Policy boundary is explicit: public Envoy can run ext_authz; the facade can
  run its own policy for local mode. Do not double-authenticate accidentally.
- The architecture is a hybrid, not "all traffic through Envoy".