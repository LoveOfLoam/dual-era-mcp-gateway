// Package audit writes JSON Lines authorization/rate-limit/tool events without
// blocking the request path on a slow audit sink.
package audit

import (
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Event is one audit record. Payload values are intentionally absent.
type Event struct {
	TS            string  `json:"ts"`
	TraceID       string  `json:"trace_id,omitempty"`
	Tenant        string  `json:"tenant,omitempty"`
	Consumer      string  `json:"consumer,omitempty"`
	Tool          string  `json:"tool,omitempty"`
	Method        string  `json:"method,omitempty"`
	Decision      string  `json:"decision"`
	Layer         string  `json:"layer,omitempty"`
	Status        string  `json:"status"`
	LatencyMS     float64 `json:"latency_ms,omitempty"`
	ConfigVersion string  `json:"config_version,omitempty"`
	RemoteIP      string  `json:"remote_ip,omitempty"`
}

// Logger serializes events to a writer.
type Logger struct {
	mu      sync.Mutex
	enc     *json.Encoder
	dropped atomic.Int64
}

// New creates a logger. A nil writer discards events.
func New(w io.Writer) *Logger {
	if w == nil {
		w = io.Discard
	}
	return &Logger{enc: json.NewEncoder(w)}
}

// Emit writes one event. Write failures increment dropped and do not panic.
func (l *Logger) Emit(e Event) {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	l.mu.Lock()
	err := l.enc.Encode(e)
	l.mu.Unlock()
	if err != nil {
		l.dropped.Add(1)
	}
}

// Dropped returns the number of failed writes.
func (l *Logger) Dropped() int64 { return l.dropped.Load() }
