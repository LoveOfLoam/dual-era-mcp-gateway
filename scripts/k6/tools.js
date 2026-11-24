import http from 'k6/http';
import { check } from 'k6';

// Not executed in this repo's local environment (k6 is not installed).
// Requires a running public Envoy/gateway with the demo API key.
const BASE = __ENV.BASE || 'http://127.0.0.1:8080';
const KEY = __ENV.API_KEY || 'test';

export const options = {
  scenarios: {
    calls: { executor: 'constant-vus', vus: 50, duration: '30s' },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<100'],
  },
};

const meta = '"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}';

export default function () {
  const body = '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"weather__get_weather","arguments":{"q":"Paris"},' + meta + '}}';
  const res = http.post(`${BASE}/modern/mcp`, body, {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${KEY}`,
      'MCP-Protocol-Version': '2026-07-28',
      'Mcp-Method': 'tools/call',
      'Mcp-Name': 'weather__get_weather',
    },
  });
  check(res, { 'status is 200': (r) => r.status === 200 });
}