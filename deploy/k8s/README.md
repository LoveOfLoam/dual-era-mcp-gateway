# Kubernetes notes

The control plane is file-based (`controlplane.json`), and the Helm chart is a
skeleton. The CRD path now has a real list/watch controller:

- `cmd/crd-controller` + `internal/kube` watch `McpSource`/`McpConsumer`;
- `deploy/controller/crd-deployment.yaml` runs 2 replicas with Lease leader
  election;
- `deploy/crds/` contains the CRD schemas.

It has **not** been validated against a live cluster in this environment:
`kubectl` exists but there is no context/kubeconfig and `kind` is not installed.

Reproduce with a cluster:

```powershell
kind create cluster --name dual-era
# build/load the image, apply deploy/crds/*.yaml, deploy/k8s/controlplane.yaml,
# deploy/controller/crd-deployment.yaml, then apply deploy/crds/examples/*.yaml
kubectl get pods -A
```

Keep the existing `controlplane.Config` and `Provider` contracts; do not fork the
runtime model.