// Package stdio bridges a newline-delimited JSON-RPC stdio MCP server to the
// in-process ListTools/Call contract. It intentionally does not use a shell.
package stdio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"

	"dual-era-mcp-gateway/internal/mcp"
	"dual-era-mcp-gateway/internal/registry"
)

// Bridge owns one stdio child process.
type Bridge struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan mcp.Response
	nextID  atomic.Int64

	closeOnce sync.Once
	closed    chan struct{}
	err       error
}

// Start starts an executable with args.
func Start(command string, args ...string) (*Bridge, error) {
	return StartCommand(exec.Command(command, args...))
}

// StartCommand starts a prepared command. The caller must not reuse cmd.
func StartCommand(cmd *exec.Cmd) (*Bridge, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	b := &Bridge{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		pending: make(map[string]chan mcp.Response),
		closed:  make(chan struct{}),
	}
	go b.readLoop()
	go b.waitLoop()
	return b, nil
}

func (b *Bridge) readLoop() {
	dec := json.NewDecoder(bufio.NewReader(b.stdout))
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			b.shutdown(err)
			return
		}
		var probe struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		if len(probe.ID) == 0 || string(probe.ID) == "null" {
			continue // server notification/log
		}
		var resp mcp.Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue
		}
		key := string(probe.ID)
		b.mu.Lock()
		ch := b.pending[key]
		delete(b.pending, key)
		b.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

func (b *Bridge) waitLoop() {
	err := b.cmd.Wait()
	b.shutdown(err)
}

func (b *Bridge) shutdown(err error) {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.err = err
		close(b.closed)
		for key, ch := range b.pending {
			close(ch)
			delete(b.pending, key)
		}
		b.mu.Unlock()
		_ = b.stdin.Close()
	})
}

// Close terminates the child. It is safe to call more than once.
func (b *Bridge) Close() error {
	b.shutdown(errors.New("stdio bridge closed"))
	if b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
	}
	return nil
}

// Initialize performs the legacy MCP handshake.
func (b *Bridge) Initialize(ctx context.Context) (json.RawMessage, error) {
	resp, err := b.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "dual-era-mcp-gateway", "version": "0.1.0"},
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("initialize: %s", resp.Error.Message)
	}
	if err := b.notify(ctx, "notifications/initialized", nil); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

// ListTools returns tools from the child, unprefixed.
func (b *Bridge) ListTools(ctx context.Context) ([]registry.Tool, error) {
	resp, err := b.rpc(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list: %s", resp.Error.Message)
	}
	var result struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}
	out := make([]registry.Tool, 0, len(result.Tools))
	for _, t := range result.Tools {
		out = append(out, registry.Tool{Name: t.Name, OriginalName: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return out, nil
}

// Call implements the legacy.Client contract.
func (b *Bridge) Call(ctx context.Context, name string, args json.RawMessage) (mcp.Response, error) {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	}
	return b.rpc(ctx, "tools/call", params)
}

// Notify sends a JSON-RPC notification (no id).
func (b *Bridge) notify(ctx context.Context, method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	return b.write(ctx, msg)
}

func (b *Bridge) rpc(ctx context.Context, method string, params any) (mcp.Response, error) {
	id := b.nextID.Add(1)
	key := strconv.FormatInt(id, 10)
	ch := make(chan mcp.Response, 1)
	b.mu.Lock()
	b.pending[key] = ch
	b.mu.Unlock()

	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := b.write(ctx, msg); err != nil {
		b.mu.Lock()
		delete(b.pending, key)
		b.mu.Unlock()
		return mcp.Response{}, err
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return mcp.Response{}, errors.New("stdio child exited")
		}
		return resp, nil
	case <-ctx.Done():
		b.mu.Lock()
		delete(b.pending, key)
		b.mu.Unlock()
		return mcp.Response{}, ctx.Err()
	case <-b.closed:
		return mcp.Response{}, errors.New("stdio child exited")
	}
}

func (b *Bridge) write(ctx context.Context, msg any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.closed:
		return errors.New("stdio child exited")
	default:
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	if _, err := b.stdin.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}
