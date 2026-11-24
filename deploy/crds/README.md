# CRD schemas (controller not implemented)

These manifests define **schemas only**:

- `McpSource` — native MCP / REST source, mirroring `internal/source.SourceSpec`;
- `McpConsumer` — tenant, API-key digests, tool allowlist.

What is **not** implemented:

- a controller that watches these CRs and applies a `controlplane.Config`;
- a deployed admission webhook (the handler/manifests exist in
  `internal/admission` and `deploy/admission`, but no cluster/TLS validation);
- status/conditions, finalizers, leader election, mTLS.

`kubectl apply -f deploy/crds/` creates the CRDs, but nothing reconciles them.
Do not describe this as Kubernetes operator support.

The current working K8s-adjacent path is the manifest/ConfigMap provider
(`internal/source/k8s.go`) and the Helm skeleton in `deploy/helm/dual-era`.