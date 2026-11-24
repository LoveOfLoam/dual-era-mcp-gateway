package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRequestPreservesIDAndParams(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"weather__get_weather","arguments":{"q":"Paris"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
	req, err := ParseRequest(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	if req.JSONRPC != "2.0" || req.Method != "tools/call" {
		t.Fatalf("unexpected request: %+v", req)
	}
	if string(req.ID) != "7" {
		t.Fatalf("id = %s, want 7", req.ID)
	}
	meta, err := req.Meta()
	if err != nil {
		t.Fatalf("Meta: %v", err)
	}
	if meta.ProtocolVersion != Version20260728 {
		t.Fatalf("protocolVersion = %q", meta.ProtocolVersion)
	}
	if meta.ClientCapabilities == nil {
		t.Fatal("clientCapabilities should be present")
	}
}

func TestParseRequestStringID(t *testing.T) {
	req, err := ParseRequest(strings.NewReader(`{"jsonrpc":"2.0","id":"abc","method":"server/discover","params":{}}`))
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	if string(req.ID) != `"abc"` {
		t.Fatalf("id = %s", req.ID)
	}
}

func TestNotificationHasNoID(t *testing.T) {
	req, err := ParseRequest(strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	if !req.IsNotification() {
		t.Fatal("expected notification")
	}
}

func TestMetaMissingCapabilities(t *testing.T) {
	req, err := ParseRequest(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`))
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	meta, err := req.Meta()
	if err != nil {
		t.Fatalf("Meta: %v", err)
	}
	if meta.ClientCapabilities != nil {
		t.Fatal("capabilities should be nil when absent")
	}
}

func TestErrorResponseMarshal(t *testing.T) {
	resp := ErrorResponse(json.RawMessage(`1`), CodeHeaderMismatch, "bad header")
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"code":-32020`) {
		t.Fatalf("missing code: %s", s)
	}
	if !strings.Contains(s, `"jsonrpc":"2.0"`) {
		t.Fatalf("missing jsonrpc: %s", s)
	}
}
