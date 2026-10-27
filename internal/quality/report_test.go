package quality

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/evidence"
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

func TestBuildExtractsObservationEvidenceAndWarnsOnMalformedPayload(t *testing.T) {
	t.Parallel()

	encoded, err := evidence.Encode(evidence.Record{
		Instances: []evidence.Instance{{
			ID: "worker-b", IdentityStatus: "observed",
			Sources: []evidence.Source{{Ref: "collector-b", ScopeName: "generic"}},
		}},
		Warnings: []string{"identity is observation-scoped"},
	})
	if err != nil {
		t.Fatalf("encode fixture evidence: %v", err)
	}
	details := snapshot.DiscoveryDetails{
		Services: []snapshot.ServiceRecord{{
			ID: "worker",
			Metadata: &snapshot.ServiceMetadata{Attributes: map[string]string{
				evidence.AttributeKey: encoded,
			}},
		}},
		Edges: []snapshot.EdgeRecord{{
			ID: "gateway|worker|sync|true",
			Metadata: &snapshot.EdgeMetadata{Attributes: map[string]string{
				evidence.AttributeKey: `{"schema_version":`,
			}},
		}},
		Endpoints: []snapshot.EndpointRecord{{
			ID: "gateway:GET /items",
			Metadata: &snapshot.EndpointMetadata{Attributes: map[string]string{
				evidence.AttributeKey: encoded,
			}},
		}},
	}

	report := Build(model.ResilienceModel{}, details, nil, snapshot.CoverageSummary{}, snapshot.Counts{}, ArtifactRef{}, "2026-01-01T00:00:00Z")
	if len(report.ObservationEvidence) != 2 {
		t.Fatalf("expected two decoded evidence items, got %+v", report.ObservationEvidence)
	}
	if got := report.ObservationEvidence[0]; got.EntityType != "endpoint" || got.EntityID != "gateway:GET /items" {
		t.Fatalf("evidence items are not canonically ordered: %+v", report.ObservationEvidence)
	}
	item := report.ObservationEvidence[1]
	if item.EntityType != "service" || item.EntityID != "worker" || len(item.Evidence.Instances) != 1 || item.Evidence.Instances[0].ID != "worker-b" {
		t.Fatalf("unexpected evidence item: %+v", item)
	}
	foundWarning := false
	for _, warning := range report.Warnings {
		if strings.Contains(warning, `invalid observation evidence for edge "gateway|worker|sync|true"`) &&
			strings.Contains(warning, evidence.AttributeKey) {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("malformed evidence warning missing: %+v", report.Warnings)
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
