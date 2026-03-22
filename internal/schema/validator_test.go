package schema

import "testing"

func TestValidateJSON_SuccessLatest(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "services": [{"id":"frontend","name":"frontend","replicas":1}],
  "edges": [],
  "endpoints": [{"id":"frontend:GET /health","entry_service":"frontend","success_predicate_ref":"frontend:GET /health","method":"GET","path":"/health"}],
  "metadata": {
    "source_type":"bering",
    "source_ref":"bering://discover?input=examples%2Ftraces",
    "discovered_at":"2026-03-03T00:00:00Z",
    "confidence":0.72,
    "schema":{
      "name":"io.mb3r.bering.model",
      "version":"1.1.0",
      "uri":"https://mb3r-lab.github.io/Bering/schema/model/v1.1.0/model.schema.json",
      "digest":"sha256:bc9a60736c9e6bda9599243fd68f293b88f42ade65321d8267369a5c3214779a"
    }
  }
}`)

	if err := ValidateJSON(raw); err != nil {
		t.Fatalf("ValidateJSON returned error: %v", err)
	}
}

func TestValidateArtifactJSON_PreservesPublishedV1_0_0(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "services": [{"id":"frontend","name":"frontend","replicas":1}],
  "edges": [],
  "endpoints": [{"id":"frontend:GET /health","entry_service":"frontend","success_predicate_ref":"frontend:GET /health"}],
  "metadata": {
    "source_type":"bering",
    "source_ref":"bering://discover?input=examples%2Ftraces",
    "discovered_at":"2026-03-03T00:00:00Z",
    "confidence":0.72,
    "schema":{
      "name":"io.mb3r.bering.model",
      "version":"1.0.0",
      "uri":"https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json",
      "digest":"sha256:272277c093f37580adcd2dded225bd37c86539d642d7910baad7e4228227d1a7"
    }
  }
}`)

	if err := ValidateArtifactJSON(raw); err != nil {
		t.Fatalf("ValidateArtifactJSON returned error for v1.0.0 model: %v", err)
	}
}

func TestValidateJSON_StrictDigestFail(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "services": [{"id":"frontend","name":"frontend","replicas":1}],
  "edges": [],
  "endpoints": [{"id":"frontend:GET /health","entry_service":"frontend","success_predicate_ref":"frontend:GET /health","method":"GET","path":"/health"}],
  "metadata": {
    "source_type":"bering",
    "source_ref":"bering://discover?input=examples%2Ftraces",
    "discovered_at":"2026-03-03T00:00:00Z",
    "confidence":0.72,
    "schema":{
      "name":"io.mb3r.bering.model",
      "version":"1.1.0",
      "uri":"https://mb3r-lab.github.io/Bering/schema/model/v1.1.0/model.schema.json",
      "digest":"sha256:deadbeef"
    }
  }
}`)

	if err := ValidateJSON(raw); err == nil {
		t.Fatal("expected strict digest validation failure, got nil")
	}
}

func TestValidateSnapshotJSON_SuccessLatest(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "snapshot_id":"snap-123",
  "topology_version":"sha256:abc",
  "window_start":"2026-03-03T00:00:00Z",
  "window_end":"2026-03-03T00:00:00Z",
  "ingest":{"spans":1,"traces":1,"dropped_spans":0,"late_spans":0},
  "counts":{"services":1,"edges":0,"endpoints":1},
  "coverage":{"confidence":0.72,"service_support_min":1,"edge_support_min":0,"endpoint_support_min":1},
  "sources":[{"type":"traces","connector":"trace_file","ref":"bering://discover","observations":1}],
  "diff":{"added_services":1,"removed_services":0,"changed_services":0,"added_edges":0,"removed_edges":0,"changed_edges":0,"added_endpoints":1,"removed_endpoints":0,"changed_endpoints":0},
  "discovery":{
    "services":[{"id":"frontend","name":"frontend","replicas":1,"support":{"observations":1,"trace_count":1}}],
    "edges":[],
    "endpoints":[{"id":"frontend:GET /health","entry_service":"frontend","method":"GET","path":"/health","support":{"observations":1,"trace_count":1}}]
  },
  "model": {
    "services": [{"id":"frontend","name":"frontend","replicas":1}],
    "edges": [],
    "endpoints": [{"id":"frontend:GET /health","entry_service":"frontend","success_predicate_ref":"frontend:GET /health","method":"GET","path":"/health"}],
    "metadata": {
      "source_type":"bering",
      "source_ref":"bering://discover?input=examples%2Ftraces",
      "discovered_at":"2026-03-03T00:00:00Z",
      "confidence":0.72,
      "schema":{
        "name":"io.mb3r.bering.model",
        "version":"1.1.0",
        "uri":"https://mb3r-lab.github.io/Bering/schema/model/v1.1.0/model.schema.json",
        "digest":"sha256:bc9a60736c9e6bda9599243fd68f293b88f42ade65321d8267369a5c3214779a"
      }
    }
  },
  "metadata": {
    "source_type":"bering",
    "source_ref":"bering://serve?listen=:4318",
    "emitted_at":"2026-03-03T00:00:00Z",
    "confidence":0.72,
    "schema":{
      "name":"io.mb3r.bering.snapshot",
      "version":"1.1.0",
      "uri":"https://mb3r-lab.github.io/Bering/schema/snapshot/v1.1.0/snapshot.schema.json",
      "digest":"sha256:53b127608b2aaa4fabb352b998cd6b2c5ed558764729a09abea56f4f9b40fa01"
    }
  }
}`)

	if err := ValidateSnapshotJSON(raw); err != nil {
		t.Fatalf("ValidateSnapshotJSON returned error: %v", err)
	}
	if err := ValidateArtifactJSON(raw); err != nil {
		t.Fatalf("ValidateArtifactJSON returned error: %v", err)
	}
}

func TestValidateSnapshotArtifactJSON_PreservesPublishedV1_0_0(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "snapshot_id":"snap-123",
  "topology_version":"sha256:abc",
  "window_start":"2026-03-03T00:00:00Z",
  "window_end":"2026-03-03T00:00:00Z",
  "ingest":{"spans":1,"traces":1,"dropped_spans":0,"late_spans":0},
  "counts":{"services":1,"edges":0,"endpoints":1},
  "coverage":{"confidence":0.72,"service_support_min":1,"edge_support_min":0,"endpoint_support_min":1},
  "sources":[{"type":"traces","connector":"trace_file","ref":"bering://discover","observations":1}],
  "diff":{"added_services":1,"removed_services":0,"changed_services":0,"added_edges":0,"removed_edges":0,"changed_edges":0,"added_endpoints":1,"removed_endpoints":0,"changed_endpoints":0},
  "discovery":{
    "services":[{"id":"frontend","name":"frontend","replicas":1,"support":{"observations":1,"trace_count":1}}],
    "edges":[],
    "endpoints":[{"id":"frontend:GET /health","entry_service":"frontend","method":"GET","path":"/health","support":{"observations":1,"trace_count":1}}]
  },
  "model": {
    "services": [{"id":"frontend","name":"frontend","replicas":1}],
    "edges": [],
    "endpoints": [{"id":"frontend:GET /health","entry_service":"frontend","success_predicate_ref":"frontend:GET /health"}],
    "metadata": {
      "source_type":"bering",
      "source_ref":"bering://discover?input=examples%2Ftraces",
      "discovered_at":"2026-03-03T00:00:00Z",
      "confidence":0.72,
      "schema":{
        "name":"io.mb3r.bering.model",
        "version":"1.0.0",
        "uri":"https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json",
        "digest":"sha256:272277c093f37580adcd2dded225bd37c86539d642d7910baad7e4228227d1a7"
      }
    }
  },
  "metadata": {
    "source_type":"bering",
    "source_ref":"bering://serve?listen=:4318",
    "emitted_at":"2026-03-03T00:00:00Z",
    "confidence":0.72,
    "schema":{
      "name":"io.mb3r.bering.snapshot",
      "version":"1.0.0",
      "uri":"https://mb3r-lab.github.io/Bering/schema/snapshot/v1.0.0/snapshot.schema.json",
      "digest":"sha256:87e4e887ed4a37b72f6136e268b73552eccb92941c4de2c6f3a514dd066ea972"
    }
  }
}`)

	if err := ValidateArtifactJSON(raw); err != nil {
		t.Fatalf("ValidateArtifactJSON returned error for v1.0.0 snapshot: %v", err)
	}
}
