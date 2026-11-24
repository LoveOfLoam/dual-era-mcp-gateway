# Environment blockers

These are the items that cannot be executed on this Windows/Docker Desktop
host. Each has a harness or documentation path; do not mark them as passing.

## Kubernetes real-cluster validation

- `kubectl.exe` exists but `kubectl config get-contexts` is empty;
  `kubectl cluster-info` tries `http://localhost:8080` and fails;
- `kind`, `minikube`, and `helm` are not installed;
- no kubeconfig/service-account token is available.

Harness: `cmd/crd-controller`, `deploy/controller/crd-deployment.yaml`,
`deploy/crds/`, fake-API tests in `internal/kube`. The list/watch, config
compile, hash dedupe, and Lease leader election are unit-tested; the live
cluster path is not.

## HA zero-loss replica stop

`scripts/t8-ha.ps1` runs two explicit gateways and stops one under load. Current
result: 17,707 requests / 3×503. Active health check, gateway drain, 8s wait,
and explicit endpoints did not remove the stop-boundary 503s. This needs Envoy
data-plane connection draining/EDS; it is not solved.

## Redis HA/failover

T8 runs one Redis container and covers outage fail-open. A Sentinel overlay and
`cmd/policy -redis-sentinel/-redis-master` are implemented, but the Docker
Desktop run did **not** complete promotion: Sentinel entered `+tilt` mode after
the master stop, and `get-master-addr-by-name` still returned `redis-master`
after both 15s and 40s waits. The harness therefore fails by design; do not
claim Redis HA.

## Long-duration SSE

T8 holds one SSE connection for 3s across xDS churn and checks handshakes every
10 applies; it does not run a multi-minute SSE soak.

## Tooling

- Helm lint/template is SKIP (no helm binary).
- Envoy binary validation is SKIP (Envoy only runs in Docker T8).
- `go test -race` needs CGO/gcc; race is CI-only on this host.