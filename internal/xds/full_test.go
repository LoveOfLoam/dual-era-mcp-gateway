package xds

import (
	"context"
	"testing"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestADSServesListenerAndRoute(t *testing.T) {
	s := New("test-node")
	if err := s.Apply(context.Background(), "v1", testConfig()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	addr, err := s.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	client := discovery.NewAggregatedDiscoveryServiceClient(conn)

	t.Run("listener", func(t *testing.T) {
		stream, err := client.StreamAggregatedResources(ctx)
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		if err := stream.Send(&discovery.DiscoveryRequest{Node: &core.Node{Id: "test-node"}, TypeUrl: resource.ListenerType}); err != nil {
			t.Fatalf("Send: %v", err)
		}
		resp, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if len(resp.Resources) == 0 {
			t.Fatal("empty listener response")
		}
		var l listener.Listener
		if err := resp.Resources[0].UnmarshalTo(&l); err != nil {
			t.Fatalf("unmarshal listener: %v", err)
		}
		if l.Name != "public" {
			t.Fatalf("listener = %q", l.Name)
		}
	})

	t.Run("route", func(t *testing.T) {
		stream, err := client.StreamAggregatedResources(ctx)
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		if err := stream.Send(&discovery.DiscoveryRequest{Node: &core.Node{Id: "test-node"}, TypeUrl: resource.RouteType}); err != nil {
			t.Fatalf("Send: %v", err)
		}
		resp, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if len(resp.Resources) == 0 {
			t.Fatal("empty route response")
		}
		var r route.RouteConfiguration
		if err := resp.Resources[0].UnmarshalTo(&r); err != nil {
			t.Fatalf("unmarshal route: %v", err)
		}
		if r.Name != "public" {
			t.Fatalf("route = %q", r.Name)
		}
	})
}
