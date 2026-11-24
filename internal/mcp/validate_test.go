package mcp

import (
	"net/http"
	"strings"
	"testing"
)

func modernHeaders(method, name, version string) http.Header {
	h := http.Header{}
	h.Set(HeaderProtocolVersion, version)
	h.Set(HeaderMethod, method)
	if name != "" {
		h.Set(HeaderName, name)
	}
	return h
}

func validRequest(t *testing.T, method string, params string) Request {
	t.Helper()
	raw := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":` + params + `}`
	req, err := ParseRequest(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	return req
}

func TestValidateModernValid(t *testing.T) {
	req := validRequest(t, "tools/call", `{"name":"weather__get_weather","arguments":{"q":"Paris"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	err := ValidateModern(req, modernHeaders("tools/call", "weather__get_weather", Version20260728))
	if err != nil {
		t.Fatalf("ValidateModern: %v", err)
	}
}

func TestValidateModernMissingMeta(t *testing.T) {
	req := validRequest(t, "tools/list", `{"_meta":{}}`)
	err := ValidateModern(req, modernHeaders("tools/list", "", Version20260728))
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != CodeInvalidParams || err.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("got %+v", err)
	}
}

func TestValidateModernMissingCapabilities(t *testing.T) {
	req := validRequest(t, "tools/list", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}`)
	err := ValidateModern(req, modernHeaders("tools/list", "", Version20260728))
	if err == nil || err.Code != CodeInvalidParams {
		t.Fatalf("got %+v", err)
	}
}

func TestValidateModernWrongVersion(t *testing.T) {
	req := validRequest(t, "tools/list", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-03-26","io.modelcontextprotocol/clientCapabilities":{}}}`)
	err := ValidateModern(req, modernHeaders("tools/list", "", Version20260728))
	if err == nil || err.Code != CodeUnsupportedVersion {
		t.Fatalf("got %+v", err)
	}
}

func TestValidateModernMethodHeaderMismatch(t *testing.T) {
	req := validRequest(t, "tools/list", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	err := ValidateModern(req, modernHeaders("tools/call", "", Version20260728))
	if err == nil || err.Code != CodeHeaderMismatch {
		t.Fatalf("got %+v", err)
	}
}

func TestValidateModernNameHeaderMismatch(t *testing.T) {
	req := validRequest(t, "tools/call", `{"name":"weather__get_weather","arguments":{},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	err := ValidateModern(req, modernHeaders("tools/call", "orders__create_order", Version20260728))
	if err == nil || err.Code != CodeHeaderMismatch {
		t.Fatalf("got %+v", err)
	}
}

func TestValidateModernMissingNameHeader(t *testing.T) {
	req := validRequest(t, "tools/call", `{"name":"weather__get_weather","arguments":{},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	err := ValidateModern(req, modernHeaders("tools/call", "", Version20260728))
	if err == nil || err.Code != CodeHeaderMismatch {
		t.Fatalf("got %+v", err)
	}
}

func TestValidateModernMissingProtocolHeader(t *testing.T) {
	req := validRequest(t, "tools/list", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	h := modernHeaders("tools/list", "", "")
	err := ValidateModern(req, h)
	if err == nil || err.Code != CodeHeaderMismatch {
		t.Fatalf("got %+v", err)
	}
}
