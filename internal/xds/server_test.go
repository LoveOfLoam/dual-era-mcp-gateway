package xds

import (
	"context"
	"testing"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"dual-era-mcp-gateway/internal/controlplane"
	"dual-era-mcp-gateway/internal/source"
)

func testConfig() controlplane.Config {
	return controlplane.Config{
		Sources: []source.SourceSpec{
			{Name: "weather", Type: source.TypeMCP, Endpoint: "http://weather:8080/mcp", Prefix: "weather"},
			{Name: "crm", Type: source.TypeREST, BaseURL: "https://crm.example.com", Prefix: "crm"},
		},
	}
}

func TestApplyBuildsClusters(t *testing.T) {
	s := New("test-node")
	if err := s.Apply(context.Background(), "v1", testConfig()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := s.CurrentVersion(); got != "v1" {
		t.Fatalf("version = %q", got)
	}
}

func TestApplyRejectsBadEndpoint(t *testing.T) {
	s := New("test-node")
	cfg := testConfig()
	cfg.Sources[0].Endpoint = "://bad"
	if err := s.Apply(context.Background(), "v1", cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestADSServesCluster(t *testing.T) {
	s := New("test-node")
	if err := s.Apply(context.Background(), "v1", testConfig()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	addr, err := s.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	client := discovery.NewAggregatedDiscoveryServiceClient(conn)
	stream, err := client.StreamAggregatedResources(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if err := stream.Send(&discovery.DiscoveryRequest{
		Node:    &core.Node{Id: "test-node"},
		TypeUrl: resource.ClusterType,
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if len(resp.Resources) == 0 {
		t.Fatal("empty cluster response")
	}
	var c cluster.Cluster
	if err := resp.Resources[0].UnmarshalTo(&c); err != nil {
		t.Fatalf("unmarshal cluster: %v", err)
	}
	if c.Name != "weather" && c.Name != "crm_rest" {
		t.Fatalf("cluster = %q", c.Name)
	}
}
