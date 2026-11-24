package mcp

import (
	"net/http"
)

// ValidateModern enforces the 2026-07-28 per-request context:
//   - params._meta.protocolVersion == 2026-07-28
//   - params._meta.clientCapabilities is present
//   - MCP-Protocol-Version matches _meta
//   - Mcp-Method matches the body method
//   - Mcp-Name matches params.name for tools/call
func ValidateModern(req Request, h http.Header) *ProtocolError {
	if req.JSONRPC != "2.0" {
		return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeInvalidRequest, Message: "jsonrpc must be 2.0"}
	}
	if req.Method == "" {
		return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeInvalidRequest, Message: "method is required"}
	}

	meta, err := req.Meta()
	if err != nil {
		return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeInvalidParams, Message: "invalid params._meta"}
	}
	if meta.ProtocolVersion == "" {
		return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeInvalidParams, Message: "missing _meta protocolVersion"}
	}
	if meta.ProtocolVersion != Version20260728 {
		return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeUnsupportedVersion, Message: "unsupported protocol version"}
	}
	if meta.ClientCapabilities == nil {
		return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeInvalidParams, Message: "missing _meta clientCapabilities"}
	}

	headerVersion := h.Get(HeaderProtocolVersion)
	if headerVersion == "" || headerVersion != meta.ProtocolVersion {
		return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeHeaderMismatch, Message: "MCP-Protocol-Version does not match _meta protocolVersion"}
	}
	headerMethod := h.Get(HeaderMethod)
	if headerMethod == "" || headerMethod != req.Method {
		return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeHeaderMismatch, Message: "Mcp-Method does not match request body method"}
	}
	if req.Method == "tools/call" {
		name := req.ToolName()
		headerName := h.Get(HeaderName)
		if name == "" || headerName == "" || headerName != name {
			return &ProtocolError{HTTPStatus: http.StatusBadRequest, Code: CodeHeaderMismatch, Message: "Mcp-Name does not match request body params.name"}
		}
	}
	return nil
}
