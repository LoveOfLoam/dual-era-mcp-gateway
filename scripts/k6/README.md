# k6 load test (not executed)

This is a smoke/load script, not a benchmark report.

```powershell
k6 run scripts/k6/tools.js
$env:BASE="http://<host>:8080"; k6 run scripts/k6/tools.js
```

Prerequisites:

- running public Envoy or local gateway;
- API key matching the policy (`test` by default);
- a backend tool `weather__get_weather`.

Do not publish k6 numbers without recording:

- commit, Envoy digest, hardware, concurrency, duration, dataset;
- direct-backend baseline;
- Envoy stats / Go pprof.

The PRD target (500 RPS/10 min, p99 < 10ms, 500 SSE) is **not** validated by
this script alone.