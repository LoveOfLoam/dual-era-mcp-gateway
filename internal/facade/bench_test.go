package facade

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

func benchFacade() *Facade {
	tools := make([]registry.Tool, 0, 10)
	for i := 0; i < 10; i++ {
		tools = append(tools, registry.Tool{Name: fmt.Sprintf("backend__tool_%02d", i), OriginalName: fmt.Sprintf("tool_%02d", i)})
	}
	return newTestFacade(&fakeUpstream{
		tools:  tools,
		result: json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`),
	})
}

func BenchmarkToolsList(b *testing.B) {
	f := benchFacade()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{` + meta(mcp.Version20260728) + `}}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/modern/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("MCP-Protocol-Version", mcp.Version20260728)
		req.Header.Set("Mcp-Method", "tools/list")
		rec := httptest.NewRecorder()
		f.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d", rec.Code)
		}
	}
}

func BenchmarkToolsCall(b *testing.B) {
	f := benchFacade()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"backend__tool_00","arguments":{},` + meta(mcp.Version20260728) + `}}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/modern/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("MCP-Protocol-Version", mcp.Version20260728)
		req.Header.Set("Mcp-Method", "tools/call")
		req.Header.Set("Mcp-Name", "backend__tool_00")
		rec := httptest.NewRecorder()
		f.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d", rec.Code)
		}
	}
}
