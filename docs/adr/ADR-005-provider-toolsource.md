# ADR-005: Separate Config Provider from Tool Source

- Status: accepted
- Date: 2026-11-24

## Context

"Import from Nacos/K8s" and "is this a REST or MCP source" are different
questions. Coupling them would make every provider understand every source type.

## Decision

Two interfaces:

- `Provider`: Load/Watch config definitions (static, OpenAPI, K8s manifest, Nacos).
- `ToolSource`: ListTools/Call/Health (native MCP, REST).

The control plane compiles `SourceSpec` into the runtime model; facade/registry
do not branch on where the config came from.

## Consequences

- New providers do not touch the data path.
- K8s/Nacos are currently manifest/Config-Service providers, not live
  informers/discovery.