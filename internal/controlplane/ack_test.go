package controlplane

import "testing"

func TestAckStore(t *testing.T) {
	s := NewAckStore()
	if s.Converged("v1") {
		t.Fatal("empty store should not converge")
	}
	s.RecordAck("envoy-1", "v1")
	if !s.Converged("v1") {
		t.Fatal("expected convergence")
	}
	s.RecordNack("envoy-2", "v1", "bad route")
	if s.Converged("v1") {
		t.Fatal("nack must break convergence")
	}
	states := s.Snapshot()
	if len(states) != 2 {
		t.Fatalf("states = %+v", states)
	}
}
