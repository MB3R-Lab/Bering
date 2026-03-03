package schema

import (
	"testing"
)

func TestValidateJSON_Success(t *testing.T) {
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

	if err := ValidateJSON(raw); err != nil {
		t.Fatalf("ValidateJSON returned error: %v", err)
	}
}

func TestValidateJSON_StrictDigestFail(t *testing.T) {
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
      "digest":"sha256:deadbeef"
    }
  }
}`)

	if err := ValidateJSON(raw); err == nil {
		t.Fatal("expected strict digest validation failure, got nil")
	}
}
