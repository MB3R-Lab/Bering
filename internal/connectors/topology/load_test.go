package topology

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileYAMLNormalizesDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "topology.yaml")
	raw := []byte(`services:
  - id: frontend
    reliability:
      live_probability: 0.99
      source: sre-calibration
      confidence: 0.8
  - id: checkout
    shared_resource_refs:
      - kafka-orders
    placements:
      - replicas: 1
        reliability:
          live_probability: 0.98
        labels:
          az: us-east-1a
edges:
  - from: frontend
    to: checkout
    kind: sync
    identity:
      protocol: http
      operation: GET
      route: /health
    reliability:
      live_probability: 0.995
    resilience:
      request_timeout_ms: 2000
    policy_scope:
      method: get
endpoints:
  - entry_service: frontend
    method: get
    path: health
    semantics:
      predicate_mode: immediate_response
      mandatory_targets:
        - checkout
      dependency_modes:
        - sync
      source: service-catalog
      confidence: 0.7
  - entry_service: checkout
    method: post
    path: process
    predicate_ref: catalog.checkout.process.eventual
    semantics:
      predicate_mode: eventual_completion
      mandatory_targets:
        - checkout
      dependency_modes:
        - async
        - sync
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write topology fixture: %v", err)
	}

	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if got, want := doc.Source.Type, SourceTypeTopologyAPI; got != want {
		t.Fatalf("source type mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Services[0].Name, "frontend"; got != want {
		t.Fatalf("service name mismatch: got=%s want=%s", got, want)
	}
	if got, want := *doc.Services[0].Replicas, 1; got != want {
		t.Fatalf("replicas mismatch: got=%d want=%d", got, want)
	}
	if got, want := doc.Services[0].Support.Observations, 1; got != want {
		t.Fatalf("service support mismatch: got=%d want=%d", got, want)
	}
	if got, want := doc.Services[1].SharedResourceRefs[0], "kafka-orders"; got != want {
		t.Fatalf("shared_resource_ref mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Services[1].Placements[0].Labels["az"], "us-east-1a"; got != want {
		t.Fatalf("placement label mismatch: got=%s want=%s", got, want)
	}
	if got, want := *doc.Services[0].Reliability.LiveProbability, 0.99; got != want {
		t.Fatalf("service reliability mismatch: got=%v want=%v", got, want)
	}
	if got, want := doc.Services[0].Reliability.Source, "sre-calibration"; got != want {
		t.Fatalf("service reliability source mismatch: got=%s want=%s", got, want)
	}
	if got, want := *doc.Services[1].Placements[0].Reliability.LiveProbability, 0.98; got != want {
		t.Fatalf("placement reliability mismatch: got=%v want=%v", got, want)
	}
	if got, want := doc.Edges[0].ID, "frontend|checkout|sync|true|protocol=http|operation=GET|route=%2Fhealth"; got != want {
		t.Fatalf("edge id mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Edges[0].Identity.Protocol, "http"; got != want {
		t.Fatalf("edge identity protocol mismatch: got=%s want=%s", got, want)
	}
	if got, want := *doc.Edges[0].Blocking, true; got != want {
		t.Fatalf("edge blocking mismatch: got=%t want=%t", got, want)
	}
	if got, want := *doc.Edges[0].Resilience.RequestTimeoutMS, 2000; got != want {
		t.Fatalf("edge request_timeout_ms mismatch: got=%d want=%d", got, want)
	}
	if got, want := *doc.Edges[0].Reliability.LiveProbability, 0.995; got != want {
		t.Fatalf("edge reliability mismatch: got=%v want=%v", got, want)
	}
	if got, want := doc.Edges[0].PolicyScope.Method, "GET"; got != want {
		t.Fatalf("edge policy_scope.method mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Endpoints[0].ID, "frontend:GET /health"; got != want {
		t.Fatalf("endpoint id mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Endpoints[0].PredicateRef, "frontend:GET /health"; got != want {
		t.Fatalf("endpoint predicate mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Endpoints[0].Support.Evidence[0], SourceTypeTopologyAPI; got != want {
		t.Fatalf("endpoint evidence mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Endpoints[0].Semantics.PredicateMode, "immediate_response"; got != want {
		t.Fatalf("endpoint semantics mode mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Endpoints[0].Semantics.MandatoryTargets[0], "checkout"; got != want {
		t.Fatalf("endpoint mandatory target mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Endpoints[1].Semantics.DependencyModes[0], "async"; got != want {
		t.Fatalf("eventual endpoint dependency mode mismatch: got=%s want=%s", got, want)
	}
}
