// Command h2check sends a single HTTP/2 request with prior knowledge over TLS.
// It exists because Windows curl.exe in this environment lacks --http2.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/http2"
)

func main() {
	url := flag.String("url", "https://127.0.0.1:8443/mcp", "target URL")
	bodyFile := flag.String("body-file", "", "request body file")
	token := flag.String("token", "test", "bearer token")
	flag.Parse()

	body := io.Reader(nil)
	if *bodyFile != "" {
		f, err := os.Open(*bodyFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		body = f
	}
	req, err := http.NewRequest(http.MethodPost, *url, body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(*token))

	tr := &http2.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, // local dev certificate only
		NextProtos:         []string{"h2"},
	}}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	fmt.Printf("status=%d proto=%s\n", resp.StatusCode, resp.Proto)
	if len(raw) > 0 {
		fmt.Println(strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode != http.StatusOK || resp.ProtoMajor != 2 {
		os.Exit(2)
	}
}
