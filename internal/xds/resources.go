package xds

import (
	"fmt"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	router "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"dual-era-mcp-gateway/internal/controlplane"
)

const (
	publicListenerPort = 8443
	modernFacadeHost   = "modern-facade"
	modernFacadePort   = 8080
	legacyInternalHost = "127.0.0.1"
	legacyInternalPort = 10001
)

// buildResources builds CDS + RDS + LDS for the public B1 listener. The
// internal mcp_router listener remains static in the MVP.
func buildResources(cfg controlplane.Config) (map[resource.Type][]types.Resource, error) {
	clusters := make([]types.Resource, 0, len(cfg.Sources)+2)
	for _, src := range cfg.Sources {
		c, err := clusterFor(src)
		if err != nil {
			return nil, err
		}
		clusters = append(clusters, c)
	}
	clusters = append(clusters,
		staticCluster("modern_facade", modernFacadeHost, modernFacadePort),
		staticCluster("legacy_internal", legacyInternalHost, legacyInternalPort),
	)

	routes, err := publicRouteConfig()
	if err != nil {
		return nil, err
	}
	listeners, err := publicListener()
	if err != nil {
		return nil, err
	}
	return map[resource.Type][]types.Resource{
		resource.ClusterType:  clusters,
		resource.RouteType:    {routes},
		resource.ListenerType: {listeners},
	}, nil
}

func publicRouteConfig() (*route.RouteConfiguration, error) {
	routeFor := func(prefix, clusterName string) *route.Route {
		return &route.Route{
			Match: &route.RouteMatch{PathSpecifier: &route.RouteMatch_Prefix{Prefix: prefix}},
			Action: &route.Route_Route{Route: &route.RouteAction{
				ClusterSpecifier: &route.RouteAction_Cluster{Cluster: clusterName},
			}},
		}
	}
	return &route.RouteConfiguration{
		Name: "public",
		VirtualHosts: []*route.VirtualHost{{
			Name:    "public",
			Domains: []string{"*"},
			Routes: []*route.Route{
				routeFor("/modern/mcp", "modern_facade"),
				routeFor("/mcp", "legacy_internal"),
			},
		}},
	}, nil
}

func publicListener() (*listener.Listener, error) {
	hcmAny, err := anypb.New(&hcm.HttpConnectionManager{
		StatPrefix: "public",
		CodecType:  hcm.HttpConnectionManager_AUTO,
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{Rds: &hcm.Rds{
			ConfigSource: &core.ConfigSource{
				ResourceApiVersion:    core.ApiVersion_V3,
				ConfigSourceSpecifier: &core.ConfigSource_Ads{Ads: &core.AggregatedConfigSource{}},
			},
		}},
		HttpFilters: []*hcm.HttpFilter{{
			Name: "envoy.filters.http.router",
			ConfigType: &hcm.HttpFilter_TypedConfig{
				TypedConfig: mustAny(&router.Router{}),
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	return &listener.Listener{
		Name: "public",
		Address: &core.Address{Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{Address: "0.0.0.0", PortSpecifier: &core.SocketAddress_PortValue{PortValue: publicListenerPort}},
		}},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: "envoy.filters.network.http_connection_manager",
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: hcmAny,
				},
			}},
		}},
	}, nil
}

func staticCluster(name, host string, port uint32) *cluster.Cluster {
	return &cluster.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		ConnectTimeout:       durationpb.New(time.Second),
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{Address: &core.Address{Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Address:       host,
								PortSpecifier: &core.SocketAddress_PortValue{PortValue: port},
							},
						}}},
					},
				}},
			}},
		},
	}
}

func mustAny(m proto.Message) *anypb.Any {
	a, err := anypb.New(m)
	if err != nil {
		panic(fmt.Sprintf("xds any: %v", err))
	}
	return a
}
