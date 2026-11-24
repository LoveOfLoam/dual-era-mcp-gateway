// Command legacy-mock is a tiny legacy MCP server for local dual-era testing.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"

	"dual-era-mcp-gateway/internal/mcp"
)

type tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

func main() {
	addr := flag.String("addr", ":9001", "listen address")
	name := flag.String("name", "mock", "server name")
	toolNames := flag.String("tools", "get_weather,list_cities", "comma-separated tool names")
	flag.Parse()

	session := "sess-" + *name
	var tools []tool
	for _, n := range strings.Split(*toolNames, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		tools = append(tools, tool{Name: n, Description: n, InputSchema: map[string]any{"type": "object"}})
	}

	http.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req mcp.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.IsNotification() {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", session)
			write(w, mcp.ResultResponse(req.ID, map[string]any{
				"protocolVersion": mcp.Version20250618,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": *name, "version": "1"},
			}))
		case "tools/list":
			write(w, mcp.ResultResponse(req.ID, map[string]any{"tools": tools}))
		case "tools/call":
			var p struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(req.Params, &p)
			write(w, mcp.ResultResponse(req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": *name + ":" + p.Name}},
			}))
		default:
			write(w, mcp.ErrorResponse(req.ID, mcp.CodeMethodNotFound, "method not found"))
		}
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	log.Printf("legacy-mock %s listening on %s", *name, *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func write(w http.ResponseWriter, resp mcp.Response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
