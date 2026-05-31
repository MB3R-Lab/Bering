package quality

import (
	"path/filepath"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func TestFromModelReportsTypedFieldCoverage(t *testing.T) {
	t.Parallel()

	failureEligible := true
	timeoutMS := 200
	maxAttempts := 2
	edgeWeight := 1.0
	endpointWeight := 1.0
	mdl := model.ResilienceModel{
		Services: []model.Service{
			{
				ID:       "checkout",
				Name:     "checkout",
				Replicas: 1,
			},
			{
				ID:       "frontend",
				Name:     "frontend",
				Replicas: 2,
				Metadata: &model.ServiceMetadata{
					FailureEligible: &failureEligible,
					Placements:      []model.Placement{{Replicas: 2, Labels: map[string]string{"az": "a"}}},
				},
			},
		},
		Edges: []model.Edge{{
			ID:       "frontend|checkout|sync|true",
			From:     "frontend",
			To:       "checkout",
			Kind:     model.EdgeKindSync,
			Blocking: true,
			Metadata: &model.EdgeMetadata{Weight: &edgeWeight},
			Resilience: &model.ResiliencePolicy{
				RequestTimeoutMS: &timeoutMS,
				Retry:            &model.RetryPolicy{MaxAttempts: &maxAttempts},
			},
		}},
		Endpoints: []model.Endpoint{{
			ID:                  "frontend:GET /checkout",
			EntryService:        "frontend",
			SuccessPredicateRef: "catalog.frontend.checkout.success",
			Method:              "GET",
			Path:                "/checkout",
			Metadata:            &model.EndpointMetadata{Weight: &endpointWeight},
		}},
		Metadata: model.Metadata{
			Schema: model.SchemaRef{Name: "io.mb3r.bering.model", Version: "1.1.0"},
		},
	}
	details := snapshot.DiscoveryDetails{
		Services: []snapshot.ServiceRecord{
			{ID: "checkout", Provenance: []snapshot.Provenance{{Type: "traces"}}},
			{ID: "frontend", Provenance: []snapshot.Provenance{{Type: "traces"}, {Type: "overlay"}}},
		},
		Edges: []snapshot.EdgeRecord{
			{ID: "frontend|checkout|sync|true", Provenance: []snapshot.Provenance{{Type: "overlay"}}},
		},
		Endpoints: []snapshot.EndpointRecord{
			{ID: "frontend:GET /checkout", Provenance: []snapshot.Provenance{{Type: "overlay"}}},
		},
	}

	report := FromModel(mdl, details, nil, snapshot.CoverageSummary{Confidence: 0.8}, "2026-03-03T00:00:00Z", "model.json")

	if report.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version mismatch: got=%s want=%s", report.SchemaVersion, SchemaVersion)
	}
	if got := dimensionByName(t, report, "services.failure_eligible"); got.Ratio != 0.5 || got.Status != "partial" {
		t.Fatalf("unexpected failure eligible dimension: %+v", got)
	}
	if got := dimensionByName(t, report, "edges.request_timeout_ms"); got.Ratio != 1 || got.Status != "complete" {
		t.Fatalf("unexpected timeout dimension: %+v", got)
	}
	if got := dimensionByName(t, report, "provenance.overlay"); got.Present != 3 || got.Total != 4 {
		t.Fatalf("unexpected overlay provenance dimension: %+v", got)
	}
}

func TestSidecarPath(t *testing.T) {
	t.Parallel()

	got := SidecarPath(filepath.Join("out", "bering-model.json"))
	want := filepath.Join("out", "bering-model.signal-quality.json")
	if got != want {
		t.Fatalf("sidecar path mismatch: got=%s want=%s", got, want)
	}
}

func dimensionByName(t *testing.T, report Report, name string) Dimension {
	t.Helper()
	for _, item := range report.Dimensions {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("dimension %s not found", name)
	return Dimension{}
}
