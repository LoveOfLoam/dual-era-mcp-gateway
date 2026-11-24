// Command mtlscheck sends one MCP initialize with an optional client cert.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	url := flag.String("url", "https://127.0.0.1:8444/mcp", "target URL")
	certFile := flag.String("cert", "", "client cert PEM")
	keyFile := flag.String("key", "", "client key PEM")
	caFile := flag.String("ca", "", "CA PEM")
	bodyFile := flag.String("body-file", "", "request body")
	token := flag.String("token", "test", "bearer token")
	flag.Parse()

	cfg := &tls.Config{ServerName: "localhost"}
	if *caFile != "" {
		pem, err := os.ReadFile(*caFile)
		if err != nil {
			fail(err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			fail(fmt.Errorf("bad CA file"))
		}
		cfg.RootCAs = pool
	}
	if *certFile != "" || *keyFile != "" {
		cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err != nil {
			fail(err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	var body io.Reader
	if *bodyFile != "" {
		f, err := os.Open(*bodyFile)
		if err != nil {
			fail(err)
		}
		defer f.Close()
		body = f
	}
	req, err := http.NewRequest(http.MethodPost, *url, body)
	if err != nil {
		fail(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(*token))
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	fmt.Printf("status=%d\n%s\n", resp.StatusCode, strings.TrimSpace(string(raw)))
	if resp.StatusCode != http.StatusOK {
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
