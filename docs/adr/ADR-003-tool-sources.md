# ADR-003: Native MCP and REST are normalized tool sources

- Status: accepted
- Date: 2026-11-24

## Context

Tool sources differ: native MCP servers speak JSON-RPC; REST APIs need mapping.
If each source type leaked into the facade/policy layer, every new source would
fork the data path.

## Decision

Normalize both to one catalog:

- prefix `backend__tool`;
- registry sorts and rejects collisions;
- native MCP via `legacy.HTTPClient` / `modernclient`;
- REST via `restadapter` (or Envoy REST bridge in deployment);
- facade only sees `ListTools` / `Call`.

## Consequences

- One policy/rate-limit model.
- REST limitations must be explicit (OAuth/HMAC/multipart/long-running are not
  handled by the simple adapter).
- SSRF/allowlist and credential isolation must be enforced at this boundary.