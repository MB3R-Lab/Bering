package snapshot

import (
	"testing"
	"time"

	"github.com/MB3R-Lab/Bering/internal/model"
)

func TestComputeDiff(t *testing.T) {
	t.Parallel()

	base := sampleEnvelope("2026-03-11T10:00:00Z", []ServiceRecord{{ID: "frontend", Name: "frontend", Replicas: 1, Support: SupportSummary{Observations: 2, TraceCount: 1}}})
	next := sampleEnvelope("2026-03-11T10:01:00Z", []ServiceRecord{{ID: "frontend", Name: "frontend", Replicas: 2, Support: SupportSummary{Observations: 3, TraceCount: 2}}, {ID: "checkout", Name: "checkout", Replicas: 1, Support: SupportSummary{Observations: 1, TraceCount: 1}}})

	diff := ComputeDiff(&base, next)
	if diff.AddedServices != 1 {
		t.Fatalf("added services mismatch: %+v", diff)
	}
	if diff.ChangedServices != 1 {
		t.Fatalf("changed services mismatch: %+v", diff)
	}
}

func TestMarshalAndParseCanonical(t *testing.T) {
	t.Parallel()

	env := sampleEnvelope("2026-03-11T10:00:00Z", []ServiceRecord{{ID: "frontend", Name: "frontend", Replicas: 1, Support: SupportSummary{Observations: 2, TraceCount: 1}}})
	raw, err := MarshalCanonical(env)
	if err != nil {
		t.Fatalf("MarshalCanonical returned error: %v", err)
	}
	parsed, err := ParseJSON(raw)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}
	if parsed.SnapshotID != env.SnapshotID {
		t.Fatalf("snapshot id mismatch: got=%s want=%s", parsed.SnapshotID, env.SnapshotID)
	}
}

func sampleEnvelope(windowEnd string, services []ServiceRecord) Envelope {
	end, _ := time.Parse(time.RFC3339, windowEnd)
	mdl := model.ResilienceModel{
		Services:  []model.Service{{ID: "frontend", Name: "frontend", Replicas: services[0].Replicas}},
		Edges:     []model.Edge{},
		Endpoints: []model.Endpoint{{ID: "frontend:GET /health", EntryService: "frontend", SuccessPredicateRef: "frontend:GET /health"}},
		Metadata:  model.Metadata{SourceType: "bering", SourceRef: "bering://test", DiscoveredAt: windowEnd, Confidence: 0.7, Schema: model.SchemaRef{Name: "io.mb3r.bering.model", Version: "1.0.0", URI: "https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json", Digest: "sha256:272277c093f37580adcd2dded225bd37c86539d642d7910baad7e4228227d1a7"}},
	}
	mdl.SortDeterministic()
	topology, _ := TopologyDigest(mdl)
	return Envelope{
		SnapshotID:      BuildSnapshotID(windowEnd, windowEnd, topology),
		TopologyVersion: topology,
		WindowStart:     windowEnd,
		WindowEnd:       windowEnd,
		Ingest:          IngestSummary{Spans: 2, Traces: 1},
		Counts:          Counts{Services: len(mdl.Services), Edges: len(mdl.Edges), Endpoints: len(mdl.Endpoints)},
		Coverage:        CoverageSummary{Confidence: 0.7, ServiceSupportMin: 1},
		Sources:         []SourceSummary{{Type: "traces", Connector: "otlp_http", Ref: "bering://serve", Observations: 2}},
		Diff:            DiffSummary{AddedServices: len(services)},
		Discovery: DiscoveryDetails{
			Services:  services,
			Edges:     []EdgeRecord{},
			Endpoints: []EndpointRecord{{ID: "frontend:GET /health", EntryService: "frontend", Method: "GET", Path: "/health", Support: SupportSummary{Observations: 1, TraceCount: 1}}},
		},
		Model: mdl,
		Metadata: Metadata{
			SourceType: "bering",
			SourceRef:  "bering://serve",
			EmittedAt:  end.Format(time.RFC3339),
			Confidence: 0.7,
			Schema:     model.SchemaRef{Name: "io.mb3r.bering.snapshot", Version: "1.0.0", URI: "https://mb3r-lab.github.io/Bering/schema/snapshot/v1.0.0/snapshot.schema.json", Digest: "sha256:87e4e887ed4a37b72f6136e268b73552eccb92941c4de2c6f3a514dd066ea972"},
		},
	}
}
