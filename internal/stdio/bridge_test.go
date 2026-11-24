package stdio

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestHelperProcess is a stdio MCP server used by TestBridge. It exits before
// the Go test framework writes anything to stdout.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_STDIO_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		if len(req.ID) == 0 {
			continue // notification
		}
		var result any
		var rpcErr any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]any{"name": "helper"}, "capabilities": map[string]any{}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "echo", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}}
		default:
			rpcErr = map[string]any{"code": -32601, "message": "method not found"}
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			resp["error"] = rpcErr
		} else {
			resp["result"] = result
		}
		_ = enc.Encode(resp)
	}
	os.Exit(0)
}

func helperCommand() *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--")
	cmd.Env = append(os.Environ(), "GO_WANT_STDIO_HELPER=1")
	return cmd
}

func TestBridge(t *testing.T) {
	b, err := StartCommand(helperCommand())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := b.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	tools, err := b.ListTools(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", tools)
	}
	resp, err := b.Call(ctx, "echo", json.RawMessage(`{"x":1}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.Error != nil || len(resp.Result) == 0 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestBridgeUnknownMethod(t *testing.T) {
	b, err := StartCommand(helperCommand())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := b.rpc(ctx, "unknown/method", nil)
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestStartInvalidCommand(t *testing.T) {
	if _, err := Start("definitely-not-a-real-binary-xyz"); err == nil {
		t.Fatal("expected error")
	}
}
