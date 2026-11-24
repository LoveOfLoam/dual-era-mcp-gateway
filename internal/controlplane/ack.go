package controlplane

import (
	"sort"
	"sync"
	"time"
)

// DataPlaneState is the last ACK/NACK observation for one Envoy.
type DataPlaneState struct {
	Node      string    `json:"node"`
	Version   string    `json:"version"`
	Ack       bool      `json:"ack"`
	Nack      bool      `json:"nack"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AckStore tracks per-data-plane configuration acknowledgements. It is the
// testable model behind a future xDS StreamState handler.
type AckStore struct {
	mu     sync.RWMutex
	states map[string]DataPlaneState
	acks   uint64
	nacks  uint64
}

// NewAckStore creates an empty store.
func NewAckStore() *AckStore {
	return &AckStore{states: make(map[string]DataPlaneState)}
}

// RecordAck records a successful apply.
func (s *AckStore) RecordAck(node, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[node] = DataPlaneState{Node: node, Version: version, Ack: true, UpdatedAt: time.Now().UTC()}
	s.acks++
}

// RecordNack records a rejected configuration.
func (s *AckStore) RecordNack(node, version, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[node] = DataPlaneState{Node: node, Version: version, Nack: true, Error: errMsg, UpdatedAt: time.Now().UTC()}
	s.nacks++
}

// Counts returns cumulative ACK/NACK events.
func (s *AckStore) Counts() (acks, nacks uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.acks, s.nacks
}

// Snapshot returns states sorted by node.
func (s *AckStore) Snapshot() []DataPlaneState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DataPlaneState, 0, len(s.states))
	for _, st := range s.states {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// Converged reports whether at least one data plane applied version and none
// rejected it.
func (s *AckStore) Converged(version string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acked := false
	for _, st := range s.states {
		if st.Version != version {
			continue
		}
		if st.Nack {
			return false
		}
		if st.Ack {
			acked = true
		}
	}
	return acked
}
