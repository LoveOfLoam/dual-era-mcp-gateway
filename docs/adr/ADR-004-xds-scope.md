# ADR-004: xDS scope — public CDS/RDS/LDS, internal static

- Status: accepted
- Date: 2026-11-25

## Context

A full xDS implementation (all listeners, EDS, canary) is a large workstream.
The highest-value path is to make the public listener and route config
versioned, while keeping the internal `mcp_router` topology stable.

## Decision

- xDS: public CDS clusters, public RDS route config, public LDS listener,
  ACK/NACK callbacks.
- Internal Envoy (`mcp_router` + `mcp_multicluster`) remains static config in
  T8/Helm.
- `mcp_multicluster` is used so backend-list changes can be CDS-only.

## Consequences

- The public listener can be updated without editing a static file.
- Internal listener changes still require a restart/drain; do not claim full
  zero-downtime.
- LDS/RDS for the internal listener remains a future item.