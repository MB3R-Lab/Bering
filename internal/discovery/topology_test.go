package discovery

import (
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/topology"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
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
	serviceReliability := 0.98
	placementReliability := 0.97
	edgeReliability := 0.995
	overlayServiceReliability := 0.965
	overlayEdgeReliability := 0.990
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
				Reliability:     &model.ReliabilityEvidence{LiveProbability: &serviceReliability, Source: "topology-api", Confidence: floatPtr(0.75)},
				FailureEligible: boolPtr(true),
				Placements: []model.Placement{
					{Replicas: 1, Labels: map[string]string{"region": "us-east-1", "az": "us-east-1a"}, Reliability: &model.ReliabilityEvidence{LiveProbability: &placementReliability}},
				},
				SharedResourceRefs: []string{"kafka-orders"},
			},
		},
		Edges: []topology.Edge{
			{
				ID:          "frontend|checkout|sync|true",
				From:        "frontend",
				To:          "checkout",
				Kind:        "sync",
				Blocking:    boolPtr(true),
				Reliability: &model.ReliabilityEvidence{LiveProbability: &edgeReliability, Source: "topology-api"},
				Support:     topology.Support{Observations: 1, Evidence: []string{"topology_api"}},
				Resilience: &model.ResiliencePolicy{
					RequestTimeoutMS: &timeout,
				},
				PolicyScope: &model.PolicyScope{SourceEndpointID: "frontend:GET /checkout", Method: "get"},
			},
		},
		Endpoints: []topology.Endpoint{
			{
				ID:           "frontend:GET /checkout",
				EntryService: "frontend",
				Method:       "GET",
				Path:         "/checkout",
				PredicateRef: "catalog.frontend.checkout.success",
				Semantics: &model.EndpointSemantics{
					PredicateMode:    model.EndpointPredicateModeImmediate,
					MandatoryTargets: []string{"checkout"},
					DependencyModes:  []string{"sync"},
					Source:           "topology-api",
				},
				Support: topology.Support{Observations: 1, Evidence: []string{"topology_api"}},
			},
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
				Reliability:        &model.ReliabilityEvidence{LiveProbability: &overlayServiceReliability, Source: "overlay"},
				Placements:         []model.Placement{{Replicas: 3, Labels: map[string]string{"region": "us-east-1", "az": "us-east-1b", "node_pool": "green"}}},
				SharedResourceRefs: []string{"kafka-orders", "redis-cart"},
			}},
			Edges: []overlay.EdgeOverlay{{
				ID:          "frontend|checkout|sync|true",
				Reliability: &model.ReliabilityEvidence{LiveProbability: &overlayEdgeReliability, Source: "overlay"},
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
			Endpoints: []overlay.EndpointOverlay{{
				ID:     "frontend:GET /checkout",
				Weight: &weight,
				Semantics: &model.EndpointSemantics{
					PredicateMode:    model.EndpointPredicateModeEventual,
					MandatoryTargets: []string{"checkout"},
					DependencyModes:  []string{"sync", "async"},
					Source:           "overlay",
				},
				CommonMetadata: overlay.CommonMetadata{Attributes: map[string]string{"owner": "edge-team"}},
			}},
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
	checkoutRecord := serviceRecordByID(t, result.Discovery.Services, "checkout")
	if got, want := result.Model.Services[0].Replicas, 5; got != want {
		t.Fatalf("replicas overlay mismatch: got=%d want=%d", got, want)
	}
	if got, want := result.Model.Services[0].Metadata.Placements[0].Labels["node_pool"], "green"; got != want {
		t.Fatalf("service placement mismatch: got=%s want=%s", got, want)
	}
	if got, want := *result.Model.Services[0].Metadata.Reliability.LiveProbability, 0.965; got != want {
		t.Fatalf("service reliability overlay mismatch: got=%v want=%v", got, want)
	}
	if got, want := checkoutRecord.Metadata.Reliability.Source, "overlay"; got != want {
		t.Fatalf("service reliability source mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Model.Services[0].Metadata.SharedResourceRefs[1], "redis-cart"; got != want {
		t.Fatalf("shared resource ref mismatch: got=%s want=%s", got, want)
	}
	if got, want := checkoutRecord.Provenance[0].Type, topology.SourceTypeTopologyAPI; got != want {
		t.Fatalf("service provenance type mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Discovery.Endpoints[0].Metadata.Attributes["owner"], "edge-team"; got != want {
		t.Fatalf("endpoint metadata attribute mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Model.Endpoints[0].Metadata.Semantics.PredicateMode, model.EndpointPredicateModeEventual; got != want {
		t.Fatalf("endpoint semantics overlay mode mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Discovery.Endpoints[0].Metadata.Semantics.Source, "overlay"; got != want {
		t.Fatalf("endpoint semantics source mismatch: got=%s want=%s", got, want)
	}
	if got, want := result.Model.Endpoints[0].Metadata.Semantics.DependencyModes[0], "async"; got != want {
		t.Fatalf("endpoint semantics dependency mode mismatch: got=%s want=%s", got, want)
	}
	if got, want := *result.Model.Edges[0].Resilience.Retry.MaxAttempts, 3; got != want {
		t.Fatalf("edge retry max_attempts mismatch: got=%d want=%d", got, want)
	}
	if got, want := *result.Model.Edges[0].Observed.ErrorRate, 0.02; got != want {
		t.Fatalf("edge observed error_rate mismatch: got=%v want=%v", got, want)
	}
	if got, want := *result.Model.Edges[0].Metadata.Reliability.LiveProbability, 0.99; got != want {
		t.Fatalf("edge reliability overlay mismatch: got=%v want=%v", got, want)
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

func floatPtr(v float64) *float64 {
	return &v
}

func serviceRecordByID(t *testing.T, records []snapshot.ServiceRecord, id string) snapshot.ServiceRecord {
	t.Helper()
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	t.Fatalf("service record %q not found", id)
	return snapshot.ServiceRecord{}
}
