# Admission webhook skeleton

`internal/admission` implements the validation logic and `cmd/admission` serves
it. It accepts `admission.k8s.io/v1` AdmissionReview and:

- validates `McpSource.spec` with `source.Validate`;
- validates `McpConsumer.spec` tenant/API-key digests.

The manifests here are **not deployed/validated**:

- `deployment.yaml` expects image `/admission` and secret `mcp-admission-tls`;
- `validating-webhook.yaml` has `caBundle: REPLACE_ME`;
- no cert-manager, no TLS bootstrap, no e2e cluster test.

Local smoke test:

```powershell
go run ./cmd/admission -addr :18443
curl.exe -sS -X POST http://127.0.0.1:18443/validate `
  -H "Content-Type: application/json" `
  -d '{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"u","kind":{"group":"gateway.dual-era.io","version":"v1alpha1","kind":"McpSource"},"object":{"spec":{"name":"weather","type":"mcp","endpoint":"http://weather:9001/mcp","prefix":"weather"}}}}'
```

A real webhook must use HTTPS; plain HTTP is only for local testing.