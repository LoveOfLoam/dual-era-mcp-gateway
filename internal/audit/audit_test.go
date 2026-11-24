package audit

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestEmitJSON(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf)
	l.Emit(Event{Tenant: "acme", Tool: "weather__get_weather", Decision: "allow", Status: "ok"})
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if out["tenant"] != "acme" || out["decision"] != "allow" || out["ts"] == "" {
		t.Fatalf("event = %+v", out)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func TestDroppedOnWriteError(t *testing.T) {
	l := New(errWriter{})
	l.Emit(Event{Decision: "allow"})
	if l.Dropped() != 1 {
		t.Fatalf("dropped = %d", l.Dropped())
	}
}
