package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type compatibilityManifest struct {
	Producer struct {
		Product    string `json:"product"`
		AppVersion string `json:"app_version"`
		SchemaLine string `json:"schema_line"`
	} `json:"producer"`
	Contracts []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Digest  string `json:"digest"`
	} `json:"contracts"`
	Artifacts []struct {
		Kind   string `json:"kind"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"artifacts"`
	Coverage map[string]bool `json:"coverage"`
}

func TestSheaftV1CompatibilityFixtures(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	fixtureDir := filepath.Join(root, "examples", "compatibility", "sheaft-v1")
	rawManifest := readFile(t, filepath.Join(fixtureDir, "manifest.json"))
	var manifest compatibilityManifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Producer.Product != "bering" || manifest.Producer.AppVersion != "v1.0.0" || manifest.Producer.SchemaLine != "1.2.0" {
		t.Fatalf("unexpected producer checkpoint: %+v", manifest.Producer)
	}
	if len(manifest.Contracts) != 2 {
		t.Fatalf("expected model and snapshot contract pins, got %d", len(manifest.Contracts))
	}
	for _, artifact := range manifest.Artifacts {
		path := filepath.Join(fixtureDir, filepath.FromSlash(artifact.Path))
		if got := fileSHA256(t, path); got != artifact.SHA256 {
			t.Fatalf("%s hash mismatch: got=%s want=%s", artifact.Path, got, artifact.SHA256)
		}
	}
	for key, ok := range manifest.Coverage {
		if !ok {
			t.Fatalf("coverage %s must stay true", key)
		}
	}

	modelRaw := readFile(t, filepath.Join(fixtureDir, "bering-model.v1.sample.json"))
	if err := ValidateArtifactJSON(modelRaw); err != nil {
		t.Fatalf("model fixture schema validation failed: %v", err)
	}
	var mdl model.ResilienceModel
	if err := json.Unmarshal(modelRaw, &mdl); err != nil {
		t.Fatalf("decode model fixture: %v", err)
	}
	if err := mdl.ValidateSemantic(); err != nil {
		t.Fatalf("model fixture semantic validation failed: %v", err)
	}
	assertSheaftV1Coverage(t, mdl)

	snapshotRaw := readFile(t, filepath.Join(fixtureDir, "bering-snapshot.v1.sample.json"))
	if err := ValidateArtifactJSON(snapshotRaw); err != nil {
		t.Fatalf("snapshot fixture schema validation failed: %v", err)
	}
	var envelope snapshot.Envelope
	if err := json.Unmarshal(snapshotRaw, &envelope); err != nil {
		t.Fatalf("decode snapshot fixture: %v", err)
	}
	if err := envelope.ValidateSemantic(); err != nil {
		t.Fatalf("snapshot fixture semantic validation failed: %v", err)
	}
	assertSheaftV1Coverage(t, envelope.Model)
	assertSnapshotPredicateRefs(t, envelope)
}

func assertSheaftV1Coverage(t *testing.T, mdl model.ResilienceModel) {
	t.Helper()

	var (
		replicas             bool
		placements           bool
		serviceReliability   bool
		placementReliability bool
		typedEdgeIDs         bool
		edgeReliability      bool
		observedEdgeMetadata bool
		resiliencePolicy     bool
		predicateRefs        bool
		immediateSemantics   bool
		eventualSemantics    bool
		syncEdge             bool
		asyncEdge            bool
	)
	for _, service := range mdl.Services {
		if service.Replicas > 1 {
			replicas = true
		}
		if service.Metadata == nil {
			continue
		}
		if len(service.Metadata.Placements) > 0 {
			placements = true
		}
		if service.Metadata.Reliability != nil && service.Metadata.Reliability.LiveProbability != nil {
			serviceReliability = true
		}
		for _, placement := range service.Metadata.Placements {
			if placement.Reliability != nil && placement.Reliability.LiveProbability != nil {
				placementReliability = true
			}
		}
	}
	for _, edge := range mdl.Edges {
		switch edge.Kind {
		case model.EdgeKindSync:
			syncEdge = true
		case model.EdgeKindAsync:
			asyncEdge = true
		}
		if edge.ID != "" && edge.Identity != nil {
			typedEdgeIDs = true
		}
		if edge.Metadata != nil && edge.Metadata.Reliability != nil && edge.Metadata.Reliability.LiveProbability != nil {
			edgeReliability = true
		}
		if edge.Observed != nil && edge.Observed.LatencyMS != nil && edge.Observed.ErrorRate != nil {
			observedEdgeMetadata = true
		}
		if edge.Resilience != nil {
			resiliencePolicy = true
		}
	}
	for _, endpoint := range mdl.Endpoints {
		if endpoint.SuccessPredicateRef != "" {
			predicateRefs = true
		}
		if endpoint.Metadata == nil || endpoint.Metadata.Semantics == nil {
			continue
		}
		switch endpoint.Metadata.Semantics.PredicateMode {
		case model.EndpointPredicateModeImmediate:
			immediateSemantics = true
		case model.EndpointPredicateModeEventual:
			eventualSemantics = true
		}
	}
	checks := map[string]bool{
		"replicas":                      replicas,
		"placements":                    placements,
		"service_reliability":           serviceReliability,
		"placement_reliability":         placementReliability,
		"typed_edge_ids":                typedEdgeIDs,
		"edge_reliability":              edgeReliability,
		"observed_edge_metadata":        observedEdgeMetadata,
		"resilience_policy":             resiliencePolicy,
		"endpoint_predicate_refs":       predicateRefs,
		"immediate_response_semantics":  immediateSemantics,
		"eventual_completion_semantics": eventualSemantics,
		"sync_edge":                     syncEdge,
		"async_edge":                    asyncEdge,
	}
	for name, ok := range checks {
		if !ok {
			t.Fatalf("fixture is missing %s coverage", name)
		}
	}
}

func assertSnapshotPredicateRefs(t *testing.T, envelope snapshot.Envelope) {
	t.Helper()
	found := false
	for _, endpoint := range envelope.Discovery.Endpoints {
		if endpoint.Metadata != nil && endpoint.Metadata.PredicateRef != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("snapshot discovery endpoints must preserve predicate_ref metadata")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	sum := sha256.Sum256(readFile(t, path))
	return hex.EncodeToString(sum[:])
}
