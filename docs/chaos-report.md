# T8 chaos run — 2026-11-25

**Result: PASS** for partial backend failure, recovery, and ext_authz
fail-closed. Docker Desktop + Envoy 1.40.2.

## Command

```powershell
powershell -File scripts/t8-chaos.ps1
```

## Observed

Baseline catalog:

```text
crm__get_customer, weather__get_weather, weather__list_cities,
rest__get_customer, rest__create_order
```

### 1. Stop `crm`

```text
partial status=200
tools=weather__get_weather,weather__list_cities,rest__get_customer,rest__create_order
crm call while stopped: status=500 body=Backend request failed
```

- `mcp_router` returns a partial catalog (healthy backends stay);
- calling the dead backend returns 500, not a false success.

### 2. Restart `crm`

```text
recovered status=200
tools=weather__get_weather,weather__list_cities,rest__get_customer,rest__create_order,crm__get_customer
```

Catalog recovers.

### 3. Stop `policy`

```text
while policy stopped: status=403 body=
```

- public Envoy fails closed when ext_authz is unavailable;
- requests do not pass unauthenticated.

## Not covered

- Redis failure/failover (T8 uses one Redis container);
- xDS NACK / control-plane failure;
- process kill during in-flight SSE;
- multi-replica / leader election.

The plan-only matrix is in [`chaos.md`](chaos.md). This report records only
what was actually executed.