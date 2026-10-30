package discovery

import (
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
)

func TestRPCSystemSemanticConventionsReachGraphAndEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs map[string]any
	}{
		{"legacy", map[string]any{"rpc.system": "grpc"}},
		{"current", map[string]any{"rpc.system.name": "grpc"}},
		{"both_agree", map[string]any{"rpc.system": "gRPC", "rpc.system.name": "grpc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := ebpfClient("client-trace", "client", "reader")
			client.Name = "sample.Lookup/Find"
			client.Attributes = map[string]any{
				"peer.service": "lookup",
				"rpc.method":   "sample.Lookup/Find",
			}
			server := ebpfClient("server-trace", "server", "lookup")
			server.Kind = "server"
			server.Name = "sample.Lookup/Find"
			server.Attributes = map[string]any{"rpc.method": "sample.Lookup/Find"}
			for key, value := range tc.attrs {
				client.Attributes[key] = value
				server.Attributes[key] = value
			}
			result := discoverObservations(t, []traces.Span{client, server}, Options{})
			rpcEdges := 0
			for _, edge := range result.Model.Edges {
				if edge.Identity != nil && edge.Identity.Protocol == "grpc" {
					rpcEdges++
					if edge.From != "reader" || edge.Identity.Operation != "sample.Lookup/Find" {
						t.Fatalf("RPC operation was changed: %+v", edge)
					}
					record := edgeEvidence(t, result, edge.ID)
					if record.Relation == nil || record.Relation.Kind != "client_dependency" {
						t.Fatalf("independent roots became causal ancestry: %+v", record)
					}
				}
			}
			if rpcEdges != 1 {
				t.Fatalf("expected one observed RPC dependency: %+v", result.Model.Edges)
			}
			if len(result.Model.Endpoints) != 1 || result.Model.Endpoints[0].Method != "GRPC" ||
				result.Model.Endpoints[0].Path != "/sample.Lookup/Find" {
				t.Fatalf("observed server endpoint missing: %+v", result.Model.Endpoints)
			}
		})
	}
}

func TestConflictingRPCSystemAliasesDoNotCreateTypedRPCObservation(t *testing.T) {
	client := ebpfClient("client-trace", "client", "reader")
	client.Name = "sample.Lookup/Find"
	client.Attributes = map[string]any{
		"peer.service": "lookup", "rpc.method": "sample.Lookup/Find",
		"rpc.system": "grpc", "rpc.system.name": "other",
	}
	server := ebpfClient("server-trace", "server", "lookup")
	server.Kind = "server"
	server.Name = "sample.Lookup/Find"
	server.Attributes = map[string]any{
		"rpc.method": "sample.Lookup/Find",
		"rpc.system": "grpc", "rpc.system.name": "other",
	}
	result := discoverObservations(t, []traces.Span{client, server}, Options{})
	if len(result.Model.Edges) != 0 || len(result.Model.Endpoints) != 0 {
		t.Fatalf("conflicting protocol aliases became typed observations: %+v", result.Model)
	}
	if client.Attributes["rpc.system"] != "grpc" || client.Attributes["rpc.system.name"] != "other" {
		t.Fatal("original conflicting attributes were modified")
	}
}

func TestCurrentRPCSystemNamesTypeGenuineParentDependency(t *testing.T) {
	parent := ebpfClient("same-trace", "parent", "reader")
	parent.Name = "sample.Lookup/Find"
	parent.Attributes = map[string]any{"rpc.system.name": "grpc", "rpc.method": "sample.Lookup/Find"}
	child := ebpfClient("same-trace", "child", "lookup")
	child.Kind = "server"
	child.ParentSpanID = "parent"
	child.Name = "sample.Lookup/Find"
	child.Attributes = map[string]any{"rpc.system.name": "grpc", "rpc.method": "sample.Lookup/Find"}
	result := discoverObservations(t, []traces.Span{parent, child}, Options{})
	found := false
	for _, edge := range result.Model.Edges {
		if edge.From == "reader" && edge.To == "lookup" && edge.Identity != nil &&
			edge.Identity.Protocol == "grpc" && edge.Identity.Operation == "sample.Lookup/Find" {
			found = true
		}
	}
	if !found {
		t.Fatalf("genuine parent dependency lost its observed protocol: %+v", result.Model.Edges)
	}
}
