package edge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func marker(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Handler", name)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(name))
	})
}

func TestDispatch(t *testing.T) {
	s := New(marker("legacy"), marker("modern"))
	for path, want := range map[string]string{"/mcp": "legacy", "/modern/mcp": "modern"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("X-Handler") != want {
			t.Fatalf("%s: status=%d handler=%s body=%s", path, rec.Code, rec.Header().Get("X-Handler"), rec.Body.String())
		}
	}
}

func TestUnknownPath(t *testing.T) {
	s := New(marker("legacy"), marker("modern"))
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	s := New(marker("legacy"), marker("modern"))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("health json: %v", err)
	}
	if out["status"] != "ok" {
		t.Fatalf("health = %+v", out)
	}
}
