# Protocol Matrix

## 1. Endpoints

| Endpoint | Client era | Transport | Session |
|---|---|---|---|
| `/mcp` | legacy 2025-xx | Streamable HTTP / SSE-compatible | `Mcp-Session-Id` optional |
| `/modern/mcp` | modern 2026-07-28 | Streamable HTTP POST | none |

## 2. Modern request requirements

### Headers

| Header | Required | Rule |
|---|---|---|
| `MCP-Protocol-Version` | yes | `2026-07-28` |
| `Mcp-Method` | yes | equals body `method` |
| `Mcp-Name` | for `tools/call` | equals body `params.name` |
| `Content-Type` | yes | `application/json` |
| `Accept` | yes | includes `application/json` (and optionally `text/event-stream`) |

### `_meta`

```json
{
  "io.modelcontextprotocol/protocolVersion": "2026-07-28",
  "io.modelcontextprotocol/clientCapabilities": {},
  "io.modelcontextprotocol/clientInfo": {"name": "test", "version": "1.0.0"}
}
```

- `protocolVersion` must match header.
- `clientCapabilities` must be an object; missing → `-32602`.
- `clientInfo` optional.

## 3. Methods

### `server/discover`

Request:

```json
{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}
```

Response:

```json
{
  "jsonrpc":"2.0",
  "id":1,
  "result":{
    "supportedVersions":["2026-07-28"],
    "capabilities":{"tools":{}},
    "serverInfo":{"name":"dual-era-mcp-gateway","version":"0.1.0"},
    "ttlMs":300000,
    "cacheScope":"public"
  }
}
```

### `tools/list`

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}
```

Response:

```json
{
  "jsonrpc":"2.0",
  "id":2,
  "result":{
    "tools":[
      {"name":"weather__get_weather","description":"...","inputSchema":{"type":"object"}},
      {"name":"crm__get_customer","description":"...","inputSchema":{"type":"object"}}
    ],
    "ttlMs":60000,
    "cacheScope":"public"
  }
}
```

### `tools/call`

```json
{
  "jsonrpc":"2.0",
  "id":3,
  "method":"tools/call",
  "params":{
    "name":"weather__get_weather",
    "arguments":{"q":"Paris"},
    "_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}
  }
}
```

Response:

```json
{
  "jsonrpc":"2.0",
  "id":3,
  "result":{
    "content":[{"type":"text","text":"..."}],
    "resultType":"complete"
  }
}
```

## 4. Errors

| Scenario | HTTP | JSON-RPC |
|---|---|---|
| Parse error | 400 | -32700 |
| Invalid request | 400 | -32600 |
| Unknown method | 200 | -32601 |
| Unknown tool | 200 | -32601 |
| Invalid params | 200/400 | -32602 |
| Missing clientCapabilities | 400 | -32602 |
| Header/body mismatch | 400 | -32020 |
| Unsupported version | 400 | -32022 |
| Auth missing | 401 | -32001 |
| Forbidden | 403 | -32003 |
| Rate limited | 429 | -32001 + `error.layer` |
| Upstream error | 502 | -32003 |

## 5. Legacy behavior

- `initialize` returns protocolVersion (2025-03-26 or configured legacy version).
- `notifications/initialized` returns 202.
- `tools/list`/`tools/call` use `Mcp-Session-Id`.
- GET/DELETE are legacy-only and handled by Envoy/internal layer; modern endpoint rejects them.

## 6. Compatibility matrix

| Client | `/mcp` | `/modern/mcp` |
|---|---|---|
| legacy 2025-xx | yes | no (should use `/mcp`) |
| modern 2026-07-28 | no (should use `/modern/mcp`) | yes |
| native MCP upstream | via `mcp_router` | via facade → internal `mcp_router` |
| REST upstream | via REST bridge | via facade → internal REST bridge |