// Command loadgen is a local HTTP load generator. It is not an Envoy/SSE/k6
// benchmark; record the topology and commit when publishing numbers.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"dual-era-mcp-gateway/internal/load"
)

type headers []string

func (h *headers) String() string { return strings.Join(*h, ",") }
func (h *headers) Set(v string) error {
	*h = append(*h, v)
	return nil
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/modern/mcp", "target URL")
	method := flag.String("method", http.MethodPost, "HTTP method")
	body := flag.String("body", "", "request body (inline)")
	bodyFile := flag.String("body-file", "", "read request body from file (avoids shell quoting)")
	concurrency := flag.Int("concurrency", 20, "workers")
	duration := flag.Duration("duration", 5*time.Second, "run duration")
	timeout := flag.Duration("timeout", 10*time.Second, "per-request timeout")
	insecure := flag.Bool("insecure", false, "skip TLS verification")
	out := flag.String("out", "", "optional JSON output file")
	var hdrs headers
	flag.Var(&hdrs, "header", "header 'Key: Value' (repeatable)")
	flag.Parse()

	headersMap := map[string]string{}
	for _, h := range hdrs {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			fmt.Fprintf(os.Stderr, "skip invalid header %q\n", h)
			continue
		}
		headersMap[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	payload := []byte(*body)
	if *bodyFile != "" {
		raw, err := os.ReadFile(*bodyFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		payload = raw
	}
	payload = bytes.TrimPrefix(payload, []byte{0xEF, 0xBB, 0xBF})
	client := &http.Client{
		Timeout: *timeout,
		Transport: &http.Transport{
			MaxIdleConns:        *concurrency * 4,
			MaxIdleConnsPerHost: *concurrency * 4,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: *insecure}, // local dev only
		},
	}
	res, err := load.Run(context.Background(), load.Config{
		URL:         *url,
		Method:      *method,
		Body:        payload,
		Headers:     headersMap,
		Concurrency: *concurrency,
		Duration:    *duration,
		Client:      client,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raw, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(raw))
	fmt.Fprintln(os.Stderr, res.Summary())
	if *out != "" {
		if err := os.WriteFile(*out, append(raw, '\n'), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
