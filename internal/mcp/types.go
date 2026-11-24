// Package mcp contains the JSON-RPC and MCP protocol primitives shared by the
// legacy and modern sides of the gateway.
package mcp

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	Version20260728 = "2026-07-28"
	Version20250326 = "2025-03-26"
	Version20250618 = "2025-06-18"
	Version20251125 = "2025-11-25"

	HeaderProtocolVersion = "MCP-Protocol-Version"
	HeaderMethod          = "Mcp-Method"
	HeaderName            = "Mcp-Name"

	MetaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	MetaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	MetaClientInfo         = "io.modelcontextprotocol/clientInfo"
)

const (
	CodeParse              = -32700
	CodeInvalidRequest     = -32600
	CodeMethodNotFound     = -32601
	CodeInvalidParams      = -32602
	CodeInternal           = -32603
	CodeUnauthorized       = -32001
	CodeForbidden          = -32003
	CodeHeaderMismatch     = -32020
	CodeUnsupportedVersion = -32022
)

// Request is an inbound JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is an outbound JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC error object. Layer is a gateway extension used by the
// rate limiter so clients can tell which policy rejected the request.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Layer   string `json:"layer,omitempty"`
}

// Meta is the per-request protocol context introduced by MCP 2026-07-28.
type Meta struct {
	ProtocolVersion    string         `json:"io.modelcontextprotocol/protocolVersion"`
	ClientCapabilities map[string]any `json:"io.modelcontextprotocol/clientCapabilities"`
	ClientInfo         map[string]any `json:"io.modelcontextprotocol/clientInfo,omitempty"`
}

// ProtocolError is an error that already knows its HTTP status and JSON-RPC code.
type ProtocolError struct {
	HTTPStatus int
	Code       int
	Message    string
	Layer      string
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("mcp: http=%d code=%d %s", e.HTTPStatus, e.Code, e.Message)
}

// ParseRequest decodes one JSON-RPC request. Callers should limit the reader
// size before calling it.
func ParseRequest(r io.Reader) (Request, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Request{}, fmt.Errorf("read request: %w", err)
	}
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return Request{}, fmt.Errorf("parse request: %w", err)
	}
	return req, nil
}

// IsNotification reports whether the request is a JSON-RPC notification.
func (r Request) IsNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

// Meta extracts params._meta. A missing params or _meta object returns the zero
// Meta, which ValidateModern turns into a missing-context error.
func (r Request) Meta() (Meta, error) {
	if len(r.Params) == 0 || string(r.Params) == "null" {
		return Meta{}, nil
	}
	var p struct {
		Meta Meta `json:"_meta"`
	}
	if err := json.Unmarshal(r.Params, &p); err != nil {
		return Meta{}, fmt.Errorf("parse params._meta: %w", err)
	}
	return p.Meta, nil
}

// ToolName extracts params.name for tools/call.
func (r Request) ToolName() string {
	if len(r.Params) == 0 {
		return ""
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(r.Params, &p); err != nil {
		return ""
	}
	return p.Name
}

// ResultResponse builds a success response.
func ResultResponse(id json.RawMessage, result any) Response {
	raw, _ := json.Marshal(result)
	return Response{JSONRPC: "2.0", ID: id, Result: raw}
}

// ErrorResponse builds a JSON-RPC error response.
func ErrorResponse(id json.RawMessage, code int, message string) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: code, Message: message}}
}

// RateLimitedResponse builds a rate-limit error with the layer extension.
func RateLimitedResponse(id json.RawMessage, layer string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: CodeUnauthorized, Message: "rate limited", Layer: layer},
	}
}
