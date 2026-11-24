package load

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	res, err := Run(context.Background(), Config{URL: srv.URL, Concurrency: 4, Duration: 80 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Requests == 0 || res.Errors != 0 {
		t.Fatalf("res = %+v", res)
	}
	if res.P95 <= 0 || res.RPS <= 0 {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunSendsBodyAndHeaders(t *testing.T) {
	var gotBody, gotMethod, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		gotMethod = r.Method
		gotHeader = r.Header.Get("Mcp-Method")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, err := Run(context.Background(), Config{
		URL: srv.URL, Method: http.MethodPost, Body: []byte(`{"jsonrpc":"2.0"}`),
		Headers: map[string]string{"Mcp-Method": "tools/list"}, Concurrency: 1, Duration: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotMethod != http.MethodPost || gotBody != `{"jsonrpc":"2.0"}` || gotHeader != "tools/list" {
		t.Fatalf("method=%q body=%q header=%q", gotMethod, gotBody, gotHeader)
	}
}

func TestRun5xxCountsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	res, err := Run(context.Background(), Config{URL: srv.URL, Concurrency: 2, Duration: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Errors == 0 || res.Status[http.StatusInternalServerError] == 0 {
		t.Fatalf("res = %+v", res)
	}
}

func TestPercentile(t *testing.T) {
	in := []time.Duration{1, 2, 3, 4, 5}
	if got := percentile(in, 50); got != 3 {
		t.Fatalf("p50 = %v", got)
	}
	if got := percentile(in, 99); got != 5 {
		t.Fatalf("p99 = %v", got)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Fatalf("empty = %v", got)
	}
}

func TestMissingURL(t *testing.T) {
	if _, err := Run(context.Background(), Config{}); err == nil {
		t.Fatal("expected error")
	}
}
