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
	item := File{
		Name: "typed",
		Services: []ServiceOverlay{{
			ID:                 "checkout",
			SharedResourceRefs: []string{" kafka-orders ", "kafka-orders"},
			Placements:         []model.Placement{{Replicas: 2, Labels: map[string]string{" az ": " us-east-1a "}}},
		}},
		Edges: []EdgeOverlay{{
			From:       "frontend",
			To:         "checkout",
			Kind:       "sync",
			Blocking:   boolPtr(true),
			Resilience: &model.ResiliencePolicy{RequestTimeoutMS: &timeout},
			PolicyScope: &model.PolicyScope{
				Method: "get",
			},
		}},
	}

	if err := item.Normalize(filepath.Join("configs", "typed-overlay.yaml")); err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if got, want := item.Edges[0].ID, "frontend|checkout|sync|true"; got != want {
		t.Fatalf("edge id mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Services[0].SharedResourceRefs[0], "kafka-orders"; got != want {
		t.Fatalf("shared resource ref mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Services[0].Placements[0].Labels["az"], "us-east-1a"; got != want {
		t.Fatalf("placement label mismatch: got=%s want=%s", got, want)
	}
	if got, want := item.Edges[0].PolicyScope.Method, "GET"; got != want {
		t.Fatalf("policy scope method mismatch: got=%s want=%s", got, want)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
