package xds

import (
	"testing"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/genproto/googleapis/rpc/status"

	"dual-era-mcp-gateway/internal/controlplane"
)

func TestCallbacksRecordAckNack(t *testing.T) {
	store := controlplane.NewAckStore()
	cb := NewCallbacks(store)

	if err := cb.OnStreamRequest(1, &discovery.DiscoveryRequest{
		Node:        &core.Node{Id: "envoy-1"},
		VersionInfo: "v1",
	}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if !store.Converged("v1") {
		t.Fatal("expected converged after ACK")
	}

	if err := cb.OnStreamRequest(2, &discovery.DiscoveryRequest{
		Node:        &core.Node{Id: "envoy-1"},
		VersionInfo: "v2",
		ErrorDetail: &status.Status{Message: "bad route"},
	}); err != nil {
		t.Fatalf("nack: %v", err)
	}
	if store.Converged("v2") {
		t.Fatal("NACK must not converge")
	}
}
