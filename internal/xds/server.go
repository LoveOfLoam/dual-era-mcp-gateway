// Package xds provides a minimal ADS server backed by go-control-plane.
// The MVP serves CDS/RDS/LDS for the public listener; the internal mcp_router
// listener remains statically generated until it is moved into xDS.
package xds

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"

	"dual-era-mcp-gateway/internal/controlplane"
	"dual-era-mcp-gateway/internal/source"
)

// Server is a minimal single-node ADS server.
type Server struct {
	node      string
	cache     cachev3.SnapshotCache
	acks      *controlplane.AckStore
	callbacks *Callbacks

	mu      sync.RWMutex
	version string
	lastCfg controlplane.Config
	grpc    *grpc.Server
}

// New creates an ADS server. Node is the Envoy node ID expected in requests.
func New(node string) *Server {
	if node == "" {
		node = "dual-era"
	}
	acks := controlplane.NewAckStore()
	return &Server{
		node:      node,
		cache:     cachev3.NewSnapshotCache(true, cachev3.IDHash{}, nil),
		acks:      acks,
		callbacks: NewCallbacks(acks),
	}
}

// ACKs exposes the ACK/NACK store for status/API use.
func (s *Server) ACKs() *controlplane.AckStore { return s.acks }

// Apply builds a CDS snapshot from the control-plane config.
func (s *Server) Apply(ctx context.Context, version string, cfg controlplane.Config) error {
	resources, err := buildResources(cfg)
	if err != nil {
		return err
	}
	snap, err := cachev3.NewSnapshot(version, resources)
	if err != nil {
		return fmt.Errorf("xds snapshot: %w", err)
	}
	if err := s.cache.SetSnapshot(ctx, s.node, snap); err != nil {
		return fmt.Errorf("xds set snapshot: %w", err)
	}
	s.mu.Lock()
	s.version = version
	s.lastCfg = cfg
	s.mu.Unlock()
	return nil
}

// InjectInvalid publishes an LDS resource with an unknown network filter. Envoy
// must NACK it and keep the previous good listener. It is used by the T8 NACK
// harness and is not called in normal operation.
func (s *Server) InjectInvalid(ctx context.Context, version string) error {
	s.mu.RLock()
	cfg := s.lastCfg
	s.mu.RUnlock()
	resources, err := buildResources(cfg)
	if err != nil {
		return err
	}
	invalid := &listener.Listener{
		Name: "public",
		Address: &core.Address{Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Address:       "0.0.0.0",
				PortSpecifier: &core.SocketAddress_PortValue{PortValue: publicListenerPort},
			},
		}},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{Name: "envoy.filters.network.definitely_invalid"}},
		}},
	}
	resources[resource.ListenerType] = []types.Resource{invalid}
	snap, err := cachev3.NewSnapshot(version, resources)
	if err != nil {
		return fmt.Errorf("xds invalid snapshot: %w", err)
	}
	if err := s.cache.SetSnapshot(ctx, s.node, snap); err != nil {
		return fmt.Errorf("xds invalid set snapshot: %w", err)
	}
	return nil
}

// CurrentVersion returns the last applied version.
func (s *Server) CurrentVersion() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// Start serves ADS on addr (use 127.0.0.1:0 in tests) and returns the actual
// listen address.
func (s *Server) Start(addr string) (string, error) {
	if addr == "" {
		addr = ":18000"
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	s.grpc = grpc.NewServer()
	discovery.RegisterAggregatedDiscoveryServiceServer(s.grpc, serverv3.NewServer(context.Background(), s.cache, s.callbacks))
	go func() {
		_ = s.grpc.Serve(lis)
	}()
	return lis.Addr().String(), nil
}

// Stop stops the gRPC server.
func (s *Server) Stop() {
	if s.grpc != nil {
		s.grpc.Stop()
	}
}

func clusterFor(src source.SourceSpec) (*cluster.Cluster, error) {
	raw := src.Endpoint
	if src.Type == source.TypeREST {
		raw = src.BaseURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("xds source %q: invalid address %q", src.Name, raw)
	}
	port := 443
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err != nil {
			return nil, fmt.Errorf("xds source %q: invalid port", src.Name)
		}
		port = p
	} else if u.Scheme == "http" {
		port = 80
	}
	name := src.Name
	if src.Type == source.TypeREST {
		name += "_rest"
	}
	return &cluster.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		ConnectTimeout:       durationpb.New(time.Second),
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: &core.Address{
								Address: &core.Address_SocketAddress{
									SocketAddress: &core.SocketAddress{
										Address:       u.Hostname(),
										PortSpecifier: &core.SocketAddress_PortValue{PortValue: uint32(port)},
									},
								},
							},
						},
					},
				}},
			}},
		},
	}, nil
}
