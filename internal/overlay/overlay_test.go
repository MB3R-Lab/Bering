package overlay

import (
	"path/filepath"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/model"
)

func TestFileNormalizeCanonicalizesRef(t *testing.T) {
	t.Parallel()

	item := File{Name: "test"}
	if err := item.Normalize(filepath.Join("configs", "nested", "overlay.yaml")); err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if item.Ref != "configs/nested/overlay.yaml" {
		t.Fatalf("ref mismatch: got=%s", item.Ref)
	}
}

func TestFileNormalizeCanonicalizesTypedMetadata(t *testing.T) {
	t.Parallel()

	timeout := 2000
	serviceReliability := 0.98
	placementReliability := 0.97
	edgeReliability := 0.995
	item := File{
		Name: "typed",
		Services: []ServiceOverlay{{
			ID:                 "checkout",
			SharedResourceRefs: []string{" kafka-orders ", "kafka-orders"},
			Reliability:        &model.ReliabilityEvidence{LiveProbability: &serviceReliability, Source: " sre "},
			Placements: []model.Placement{{
				Replicas:    2,
				Labels:      map[string]string{" az ": " us-east-1a "},
				Reliability: &model.ReliabilityEvidence{LiveProbability: &placementReliability},
			}},
		}},
		Edges: []EdgeOverlay{{
			From:        "frontend",
			To:          "checkout",
			Kind:        "sync",
			Blocking:    boolPtr(true),
			Identity:    &model.EdgeIdentity{Protocol: "HTTP", Operation: "GET", Route: "/checkout"},
			Reliability: &model.ReliabilityEvidence{LiveProbability: &edgeReliability},
			Resilience:  &model.ResiliencePolicy{RequestTimeoutMS: &timeout},
			PolicyScope: &model.PolicyScope{
				Method: "get",
			},
		}},
		Endpoints: []EndpointOverlay{{
			EntryService: "frontend",
			Method:       "post",
			Path:         "checkout",
			Semantics: &model.EndpointSemantics{
				PredicateMode:    "EVENTUAL_COMPLETION",
				MandatoryTargets: []string{" checkout ", "checkout"},
				DependencyModes:  []string{"ASYNC", "sync", "async"},
			},
		}},
	}

	if err := item.Normalize(filepath.Join("configs", "typed-overlay.yaml")); err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got, want := item.Edges[0].ID, "frontend|checkout|sync|true|protocol=http|operation=GET|route=%2Fcheckout"; got != want {
		t.Fatalf("edge id mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Services[0].SharedResourceRefs[0], "kafka-orders"; got != want {
		t.Fatalf("shared resource ref mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Services[0].Placements[0].Labels["az"], "us-east-1a"; got != want {
		t.Fatalf("placement label mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Services[0].Reliability.Source, "sre"; got != want {
		t.Fatalf("service reliability source mismatch: got=%s want=%s", got, want)
	}
	if got, want := *item.Services[0].Placements[0].Reliability.LiveProbability, 0.97; got != want {
		t.Fatalf("placement reliability mismatch: got=%v want=%v", got, want)
	}
	if got, want := *item.Edges[0].Reliability.LiveProbability, 0.995; got != want {
		t.Fatalf("edge reliability mismatch: got=%v want=%v", got, want)
	}
	if got, want := item.Edges[0].PolicyScope.Method, "GET"; got != want {
		t.Fatalf("policy scope method mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Endpoints[0].ID, "frontend:POST /checkout"; got != want {
		t.Fatalf("endpoint id mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Endpoints[0].Semantics.PredicateMode, model.EndpointPredicateModeEventual; got != want {
		t.Fatalf("endpoint semantics mode mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Endpoints[0].Semantics.DependencyModes[0], "async"; got != want {
		t.Fatalf("endpoint semantics dependency mode mismatch: got=%s want=%s", got, want)
	}
	if got, want := len(item.Endpoints[0].Semantics.MandatoryTargets), 1; got != want {
		t.Fatalf("endpoint mandatory target count mismatch: got=%d want=%d", got, want)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
