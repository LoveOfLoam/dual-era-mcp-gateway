# T8 mTLS run — 2026-11-25

**Result: PASS** — a client certificate is required at the public listener.

## Command

```powershell
powershell -File scripts/t8-mtls.ps1
```

The script generates a dev CA, server cert, client cert, and a PKCS#12 client
identity under `deploy/t8/certs/mtls/` (gitignored), starts the T8 stack with
`docker-compose.mtls.yml`, and uses `cmd/mtlscheck` (Go TLS client).

## Observed

```text
with client cert: status=200
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18",...}}

without client cert: exit=3
Post "https://127.0.0.1:8444/mcp": remote error: tls: certificate required

T8 MTLS PASS: client-cert request 200; no-cert request rejected
```

## What this proves

- Envoy `DownstreamTlsContext` with `require_client_certificate: true`;
- trusted CA validation (`validation_context.trusted_ca`);
- a request with the client identity succeeds;
- a request without a client certificate is rejected at the TLS layer.

## Not covered

- production PKI / rotation / revocation;
- mTLS for control-plane ADS;
- HA / leader election / multi-replica (B-12 remains partial);
- client-certificate identity mapping into policy.

Dev certs are 2-day self-signed material; do not use them in production.