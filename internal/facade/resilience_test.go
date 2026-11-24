package facade

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"dual-era-mcp-gateway/internal/mcp"
)

func TestUpstreamListErrorReturns502(t *testing.T) {
	f := newTestFacade(&fakeUpstream{err: errors.New("boom")})
	rec := post(t, f, "tools/list", `{`+meta(mcp.Version20260728)+`}`, nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestConcurrentToolsCall(t *testing.T) {
	f := newTestFacade(&fakeUpstream{result: []byte(`{"content":[]}`)})
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := post(t, f, "tools/call", `{"name":"weather__get_weather","arguments":{},`+meta(mcp.Version20260728)+`}`, map[string]string{"Mcp-Name": "weather__get_weather"})
			if rec.Code != http.StatusOK {
				errs <- rec.Body.String()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for body := range errs {
		t.Fatalf("concurrent call failed: %s", body)
	}
}

func TestBodyTooLarge(t *testing.T) {
	f := newTestFacade(&fakeUpstream{})
	body := strings.Repeat("a", 2<<20)
	req := httptest.NewRequest(http.MethodPost, "/modern/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", rec.Code)
	}
}
