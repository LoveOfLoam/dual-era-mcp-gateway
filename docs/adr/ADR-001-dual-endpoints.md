# ADR-001: Versioned dual endpoints (B1)

- Status: accepted
- Date: 2026-11-24

## Context

Gate 1 showed that a single public listener with terminal `mcp_router` cannot
serve 2026-07-28 clients: `server/discover` is not handled, and modern headers
are not forwarded. Treating the legacy path as "modern support" would be a false
claim.

## Decision

Use two endpoints:

- `/mcp` for legacy 2025-xx clients;
- `/modern/mcp` for modern 2026-07-28 clients.

Public Envoy only routes by path. The Go facade owns the modern protocol.

## Consequences

- Honest compatibility matrix; no single-endpoint claim.
- Two entry points to document/test.
- B2 (single endpoint) remains a future design, not the MVP.