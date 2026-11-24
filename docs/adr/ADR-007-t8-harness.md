# ADR-007: T8 is a Docker/Envoy harness, not a mocked test

- Status: accepted
- Date: 2026-11-25

## Context

Unit tests can prove Go protocol/aggregation, but not Envoy `mcp_router`,
`mcp_multicluster`, ext_authz, or path routing. The local environment has no
Envoy binary and Docker daemon access is blocked.

## Decision

- Provide `deploy/t8/docker-compose.yml` and `scripts/t8.ps1`.
- Default image is `envoyproxy/envoy:dev`; production must use a digest-locked
  image from the Gate 1 spike.
- The script SKIPs when Docker/Envoy is unavailable; it must never report PASS
  without exercising the topology.

## Consequences

- T8 remains `BLOCKED` in this environment.
- Do not use T0–T7 results as evidence for Envoy behavior.
- The harness is ready for a machine with Docker.