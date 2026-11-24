// Command ssecheck drives the legacy MCP SSE handshake and one tools/list.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	base := flag.String("url", "http://127.0.0.1:8080/mcp", "SSE endpoint")
	token := flag.String("token", "test", "bearer token")
	insecure := flag.Bool("insecure", false, "skip TLS verification")
	flag.Parse()

	tr := &http.Transport{}
	if *insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Transport: tr}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, *base, nil)
	if err != nil {
		fail(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+*token)
	resp, err := client.Do(req)
	if err != nil {
		fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fail(fmt.Errorf("SSE status %d", resp.StatusCode))
	}
	br := bufio.NewReader(resp.Body)
	event, endpoint := readEvent(br)
	if event != "endpoint" {
		fail(fmt.Errorf("expected endpoint event, got %q %q", event, endpoint))
	}
	baseURL, _ := url.Parse(*base)
	ref, _ := url.Parse(endpoint)
	target := baseURL.ResolveReference(ref).String()

	post := func(body string) string {
		pr, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
		if err != nil {
			fail(err)
		}
		pr.Header.Set("Content-Type", "application/json")
		pr.Header.Set("Accept", "application/json")
		pr.Header.Set("Authorization", "Bearer "+*token)
		rr, err := client.Do(pr)
		if err != nil {
			fail(err)
		}
		defer rr.Body.Close()
		if rr.StatusCode != http.StatusAccepted {
			fail(fmt.Errorf("POST status %d", rr.StatusCode))
		}
		ev, data := readEvent(br)
		if ev != "message" {
			fail(fmt.Errorf("expected message, got %q %q", ev, data))
		}
		return data
	}
	init := post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"ssecheck","version":"0"}}}`)
	if !strings.Contains(init, "protocolVersion") {
		fail(fmt.Errorf("bad initialize: %s", init))
	}
	list := post(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	var decoded struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(list), &decoded); err != nil || len(decoded.Result.Tools) == 0 {
		fail(fmt.Errorf("bad tools/list: %s", list))
	}
	fmt.Printf("sse_ok=true tools=%d\n", len(decoded.Result.Tools))
}

func readEvent(br *bufio.Reader) (string, string) {
	var event, data string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return event, data
			}
			fail(err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if event != "" || data != "" {
				return event, data
			}
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
