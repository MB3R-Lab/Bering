package model

import "testing"

func TestValidateSemantic_StrictSchemaFieldsLeftToContract(t *testing.T) {
	t.Parallel()

	mdl := ResilienceModel{
		Services: []Service{{ID: "frontend", Name: "frontend", Replicas: 1}},
		Edges:    []Edge{},
		Endpoints: []Endpoint{
			{ID: "frontend:GET /health", EntryService: "frontend", SuccessPredicateRef: "frontend:GET /health"},
		},
		Metadata: Metadata{
			SourceType:   "bering",
			SourceRef:    "bering://discover?input=test",
			DiscoveredAt: "2026-03-03T00:00:00Z",
			Confidence:   0.7,
			Schema:       SchemaRef{},
		},
	}

	if err := mdl.ValidateSemantic(); err != nil {
		t.Fatalf("expected semantic validation pass, got: %v", err)
	}
}
