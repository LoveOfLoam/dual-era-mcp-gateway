package xds

import (
	"context"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	"dual-era-mcp-gateway/internal/controlplane"
)

// Callbacks records ACK/NACK observations from an ADS stream.
type Callbacks struct {
	store *controlplane.AckStore
}

// NewCallbacks creates callbacks backed by an AckStore.
func NewCallbacks(store *controlplane.AckStore) *Callbacks {
	return &Callbacks{store: store}
}

// OnStreamOpen is a no-op.
func (c *Callbacks) OnStreamOpen(context.Context, int64, string) error { return nil }

// OnStreamClosed is a no-op.
func (c *Callbacks) OnStreamClosed(int64, *core.Node) {}

// OnStreamRequest records ACK/NACK from an xDS request.
func (c *Callbacks) OnStreamRequest(_ int64, req *discovery.DiscoveryRequest) error {
	if req == nil {
		return nil
	}
	node := ""
	if req.GetNode() != nil {
		node = req.GetNode().GetId()
	}
	if node == "" {
		return nil
	}
	if req.GetErrorDetail() != nil {
		c.store.RecordNack(node, req.GetVersionInfo(), req.GetErrorDetail().GetMessage())
		return nil
	}
	if req.GetVersionInfo() != "" {
		c.store.RecordAck(node, req.GetVersionInfo())
	}
	return nil
}

// OnStreamResponse is a no-op.
func (c *Callbacks) OnStreamResponse(context.Context, int64, *discovery.DiscoveryRequest, *discovery.DiscoveryResponse) {
}

// OnFetchRequest is a no-op.
func (c *Callbacks) OnFetchRequest(context.Context, *discovery.DiscoveryRequest) error { return nil }

// OnFetchResponse is a no-op.
func (c *Callbacks) OnFetchResponse(*discovery.DiscoveryRequest, *discovery.DiscoveryResponse) {}

// OnDeltaStreamOpen is a no-op.
func (c *Callbacks) OnDeltaStreamOpen(context.Context, int64, string) error { return nil }

// OnDeltaStreamClosed is a no-op.
func (c *Callbacks) OnDeltaStreamClosed(int64, *core.Node) {}

// OnStreamDeltaRequest is a no-op for the MVP.
func (c *Callbacks) OnStreamDeltaRequest(int64, *discovery.DeltaDiscoveryRequest) error {
	return nil
}

// OnStreamDeltaResponse is a no-op for the MVP.
func (c *Callbacks) OnStreamDeltaResponse(int64, *discovery.DeltaDiscoveryRequest, *discovery.DeltaDiscoveryResponse) {
}
