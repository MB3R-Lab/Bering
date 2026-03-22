package discovery

import (
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/topology"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
)

func TestDiscoverTopology_AppliesProvenanceAndOverlays(t *testing.T) {
	t.Parallel()

	replicas := 5
	weight := 0.4
	timeout := 2000
	maxAttempts := 3
	initialBackoff := 50
	maxBackoff := 500
	multiplier := 2.0
	overlayErrorRate := 0.02
	doc := topology.Document{
		Source: topology.Source{Type: topology.SourceTypeTopologyAPI, Ref: "https://topology.internal.example/api/v1/topology"},
		Services: []topology.Service{
			{
				ID:       "frontend",
				Name:     "frontend",
				Replicas: intPtr(2),
			},
			{
				ID:              "checkout",
				Name:            "checkout",
				Replicas:        intPtr(1),
				FailureEligible: boolPtr(true),
				Placements: []model.Placement{
					{Replicas: 1, Labels: map[string]string{"region": "us-east-1", "az": "us-east-1a"}},
				},
				SharedResourceRefs: []string{"kafka-orders"},
			},
		},
		Edges: []topology.Edge{
			{
				ID:       "frontend|checkout|sync|true",
				From:     "frontend",
				To:       "checkout",
				Kind:     "sync",
				Blocking: boolPtr(true),
				Support:  topology.Support{Observations: 1, Evidence: []string{"topology_api"}},
				Resilience: &model.ResiliencePolicy{
					RequestTimeoutMS: &timeout,
				},
				PolicyScope: &model.PolicyScope{SourceEndpointID: "frontend:GET /checkout", Method: "get"},
			},
		},
		Endpoints: []topology.Endpoint{
			{ID: "frontend:GET /checkout", EntryService: "frontend", Method: "GET", Path: "/checkout", PredicateRef: "catalog.frontend.checkout.success", Support: topology.Support{Observations: 1, Evidence: []string{"topology_api"}}},
		},
	}
	if err := doc.Normalize(); err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}

	result, err := DiscoverTopology(doc, Options{
		SourceRef:    BuildSourceRef("examples/topology/topology-api.sample.yaml"),
		DiscoveredAt: "2026-03-11T00:00:00Z",
		Overlays: []overlay.File{{
			Name: "topology-overlay",
			Services: []overlay.ServiceOverlay{{
				ID:                 "checkout",
				Replicas:           &replicas,
				Placements:         []model.Placement{{Replicas: 3, Labels: map[string]string{"region": "us-east-1", "az": "us-east-1b", "node_pool": "green"}}},
				SharedResourceRefs: []string{"kafka-orders", "redis-cart"},
			}},
			Edges: []overlay.EdgeOverlay{{
				ID: "frontend|checkout|sync|true",
				Resilience: &model.ResiliencePolicy{
					Retry: &model.RetryPolicy{
						MaxAttempts: &maxAttempts,
						Backoff: &model.BackoffPolicy{
							InitialMS:  &initialBackoff,
							MaxMS:      &maxBackoff,
							Multiplier: &multiplier,
						},
					},
				},
				Observed: &model.ObservedEdge{ErrorRate: &overlayErrorRate},
			}},
			Endpoints: []overlay.EndpointOverlay{{ID: "frontend:GET /checkout", Weight: &weight, CommonMetadata: overlay.CommonMetadata{Attributes: map[string]string{"owner": "edge-team"}}}},
		}},
	})
	if err != nil {
		t.Fatalf("DiscoverTopology returned error: %v", err)
	}

	if got, want := result.Sources[0].Type, topology.SourceTypeTopologyAPI; got != want {
		t.Fatalf("source type mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Sources[0].Connector, topology.ConnectorName; got != want {
		t.Fatalf("source connector mismatch: got=%s want=%s", got, want)
	}
	if got := result.Model.Metadata.Confidence; got != 1.0 {
		t.Fatalf("confidence mismatch: got=%.2f", got)
	}
	if got, want := result.Model.Services[0].ID, "checkout"; got != want {
		t.Fatalf("service ordering mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Model.Services[0].Replicas, 5; got != want {
		t.Fatalf("replicas overlay mismatch: got=%d want=%d", got, want)
	}
	if got, want := result.Model.Services[0].Metadata.Placements[0].Labels["node_pool"], "green"; got != want {
		t.Fatalf("service placement mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Model.Services[0].Metadata.SharedResourceRefs[1], "redis-cart"; got != want {
		t.Fatalf("shared resource ref mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Discovery.Services[0].Provenance[0].Type, topology.SourceTypeTopologyAPI; got != want {
		t.Fatalf("service provenance type mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Discovery.Endpoints[0].Metadata.Attributes["owner"], "edge-team"; got != want {
		t.Fatalf("endpoint metadata attribute mismatch: got=%s want=%s", got, want)
	}
	if got, want := *result.Model.Edges[0].Resilience.Retry.MaxAttempts, 3; got != want {
		t.Fatalf("edge retry max_attempts mismatch: got=%d want=%d", got, want)
	}
	if got, want := *result.Model.Edges[0].Observed.ErrorRate, 0.02; got != want {
		t.Fatalf("edge observed error_rate mismatch: got=%v want=%v", got, want)
	}
	if got, want := result.Model.Edges[0].PolicyScope.Method, "GET"; got != want {
		t.Fatalf("policy scope method mismatch: got=%s want=%s", got, want)
	}
	if got, want := len(result.Discovery.Overlays), 1; got != want {
		t.Fatalf("overlay application count mismatch: got=%d want=%d", got, want)
	}
}

func intPtr(v int) *int {
	return &v
}
