// Package xds provides a minimal ADS server backed by go-control-plane.
// The MVP serves CDS clusters derived from SourceSpecs; listeners/routes are
// still rendered by internal/envoy until the xDS resource builder is extended.
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
	node  string
	cache cachev3.SnapshotCache

	mu      sync.RWMutex
	version string
	grpc    *grpc.Server
}

// New creates an ADS server. Node is the Envoy node ID expected in requests.
func New(node string) *Server {
	if node == "" {
		node = "dual-era"
	}
	return &Server{
		node:  node,
		cache: cachev3.NewSnapshotCache(true, cachev3.IDHash{}, nil),
	}
}

// Apply builds a CDS snapshot from the control-plane config.
func (s *Server) Apply(ctx context.Context, version string, cfg controlplane.Config) error {
	resources := make([]types.Resource, 0, len(cfg.Sources))
	for _, src := range cfg.Sources {
		c, err := clusterFor(src)
		if err != nil {
			return err
		}
		resources = append(resources, c)
	}
	snap, err := cachev3.NewSnapshot(version, map[resource.Type][]types.Resource{
		resource.ClusterType: resources,
	})
	if err != nil {
		return fmt.Errorf("xds snapshot: %w", err)
	}
	if err := s.cache.SetSnapshot(ctx, s.node, snap); err != nil {
		return fmt.Errorf("xds set snapshot: %w", err)
	}
	s.mu.Lock()
	s.version = version
	s.mu.Unlock()
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
	discovery.RegisterAggregatedDiscoveryServiceServer(s.grpc, serverv3.NewServer(context.Background(), s.cache, nil))
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
