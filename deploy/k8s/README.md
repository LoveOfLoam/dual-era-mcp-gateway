# Kubernetes notes

The project currently uses a file-based control plane (`controlplane.json`) plus
the Helm skeleton. It does **not** yet implement the six CRDs or a
controller-runtime reconciler described in the PRD.

Why:

- CRD/controller/admission is a multi-week workstream and was not required to
  prove the B1 protocol/aggregation slice.
- The `Provider` abstraction already supports K8s ConfigMap/McpSource manifests
  and Nacos Config Service; a live controller can reuse it later.

If you add a controller, keep the existing `controlplane.Config` and
`Provider` contracts; do not fork the runtime model.