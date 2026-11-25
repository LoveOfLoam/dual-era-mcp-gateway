# Environment blockers

These are the items that cannot be executed on this Windows/Docker Desktop
host. Each has a harness or documentation path; do not mark them as passing.

## CI status

- GitHub Actions `test` is green at `49e4c54`;
- it runs only `gofmt`, `go test`, `go test -race`, `go vet`, `go build`;
- it does not run T8, Helm, load, chaos, or Kubernetes;
- historical red runs (#10–#17) were mostly the flaky `internal/load` race test,
  fixed by `6de166f`; do not rewrite history for those.

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
- `go test -race` needs CGO/gcc; the Windows host has no gcc, but a Linux Go
  container (`golang:1.26`) is used for local race runs and CI also runs race.

## WSL as the Linux environment

WSL2 is the recommended way to remove most of the tooling blockers:

- install `build-essential` for local `go test -race`;
- install `helm`, `kind`, `kubectl`, `govulncheck`, `trivy`, `k6`;
- run the T8 Docker/Envoy harness from Linux (Docker Desktop WSL integration or
  Docker Engine inside WSL);
- validate the CRD controller/leader election against a kind cluster.

Caveats: keep the repo on the WSL ext4 filesystem (`~/...`, not `/mnt/c`);
PowerShell T8 scripts need PowerShell 7 or a bash conversion; Docker Desktop
networking/clock quirks (including Sentinel tilt) may still reproduce.