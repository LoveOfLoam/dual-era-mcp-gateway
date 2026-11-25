# CRD schemas and list/watch controller

These manifests define:

- `McpSource` — native MCP / REST source, mirroring `internal/source.SourceSpec`;
- `McpConsumer` — tenant, API-key digests, tool allowlist.

A controller exists:

- `cmd/crd-controller` + `internal/kube` list/watch both CRDs and POST a
  compiled `controlplane.Config` to `/api/v1/apply`;
- `deploy/controller/crd-deployment.yaml` runs 2 replicas with Lease leader
  election;
- fake-Kubernetes API tests cover list/compile/hash dedupe and lease
  acquire/renew/expiry.

Not verified:

- real-cluster install/rollout;
- status/conditions and finalizers;
- admission webhook against a real API server;
- mTLS/HA for the controller.

Do not describe this as production Kubernetes operator support. The
ConfigMap-directory controller (`internal/controller`) remains available as a
non-CRD mode.