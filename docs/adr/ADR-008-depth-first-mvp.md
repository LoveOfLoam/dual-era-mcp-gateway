# ADR-008: Depth-first on three differentiators

- Status: accepted
- Date: 2026-11-25

## Context

The gateway already has a broad surface: two endpoints, native MCP + REST,
policy, xDS, T8, Helm, K8s extras. Adding more features would increase breadth
but weaken the project story.

For a resume/demo project, the defensible differentiation is:

1. Envoy data plane + Go control plane with xDS;
2. legacy/modern dual endpoints during the MCP transition;
3. native MCP + REST → MCP behind one catalog.

Production-only work (real cluster, zero-loss HA, Redis HA, OIDC, canary) is
valuable but currently unverified on this host.

## Decision

1. **Do not add new protocol/tool-source features** until the three
   differentiators have depth.
2. Treat unverified production work as “implemented/harnessed, not claimed”.
3. Depth means:
   - xDS: persistence, auth, leader election, internal listener, EDS, NACK
     rollback;
   - dual endpoints: protocol matrix, SSE limits, error compatibility, drain;
   - REST/native: retries/circuit/health, idempotency, multipart/async.
4. Keep T8 as separate evidence; CI remains Go-only unless a Linux harness is
   added intentionally.

## Consequences

- Resume claims stay narrow and defensible.
- More value per remaining hour than adding a fourth protocol.
- Production gaps remain explicit in `docs/project-status.md` and
  `docs/blockers.md`.
- If the goal changes to production trial, this ADR must be revisited first.