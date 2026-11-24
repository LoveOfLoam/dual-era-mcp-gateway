# CRD schemas (controller not implemented)

These manifests define **schemas only**:

- `McpSource` — native MCP / REST source, mirroring `internal/source.SourceSpec`;
- `McpConsumer` — tenant, API-key digests, tool allowlist.

What is **not** implemented:

- a controller that watches these CRs and applies a `controlplane.Config`;
- a CRD controller/informer (there is a ConfigMap-directory controller in
  `internal/controller`, but it polls a volume, not the Kubernetes API);
- status/conditions, finalizers, leader election, mTLS.

`kubectl apply -f deploy/crds/` creates the CRDs, but nothing reconciles them.
Do not describe this as Kubernetes operator support.

The current working K8s-adjacent path is the manifest/ConfigMap provider
(`internal/source/k8s.go`) and the Helm skeleton in `deploy/helm/dual-era`.