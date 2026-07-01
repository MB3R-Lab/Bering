package model

import "testing"

func TestEdgeIDWithIdentityPreservesLegacyIDWhenIdentityEmpty(t *testing.T) {
	t.Parallel()

	got := EdgeIDWithIdentity("frontend", "checkout", EdgeKindSync, true, nil)
	want := "frontend|checkout|sync|true"
	if got != want {
		t.Fatalf("edge id mismatch: got=%s want=%s", got, want)
	}
}

func TestEdgeIDWithIdentityAddsStableDiscriminators(t *testing.T) {
	t.Parallel()

	identity := &EdgeIdentity{Protocol: "HTTP", Operation: "POST", Route: "/process", SpanKind: "SERVER"}
	identity.Normalize()
	got := EdgeIDWithIdentity("frontend", "checkout", EdgeKindSync, true, identity)
	want := "frontend|checkout|sync|true|protocol=http|operation=POST|route=%2Fprocess|span_kind=server"
	if got != want {
		t.Fatalf("edge id mismatch: got=%s want=%s", got, want)
	}
}

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

func TestValidateSemantic_ReliabilityEvidenceMustBeProbability(t *testing.T) {
	t.Parallel()

	serviceReliability := 1.01
	edgeReliability := -0.1
	mdl := ResilienceModel{
		Services: []Service{{
			ID:       "frontend",
			Name:     "frontend",
			Replicas: 1,
			Metadata: &ServiceMetadata{
				Reliability: &ReliabilityEvidence{LiveProbability: &serviceReliability},
			},
		}},
		Edges: []Edge{{
			ID:       "frontend|frontend|sync|true",
			From:     "frontend",
			To:       "frontend",
			Kind:     EdgeKindSync,
			Blocking: true,
			Metadata: &EdgeMetadata{
				Reliability: &ReliabilityEvidence{LiveProbability: &edgeReliability},
			},
		}},
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

	if err := mdl.ValidateSemantic(); err == nil {
		t.Fatal("expected reliability probability validation error, got nil")
	}
}

func TestValidateSemantic_EndpointSemanticsValidateMandatoryTargets(t *testing.T) {
	t.Parallel()

	confidence := 0.9
	mdl := ResilienceModel{
		Services: []Service{
			{ID: "frontend", Name: "frontend", Replicas: 1},
			{ID: "checkout", Name: "checkout", Replicas: 1},
			{ID: "email", Name: "email", Replicas: 1},
		},
		Edges: []Edge{},
		Endpoints: []Endpoint{{
			ID:                  "frontend:POST /checkout",
			EntryService:        "frontend",
			SuccessPredicateRef: "catalog.checkout.eventual",
			Metadata: &EndpointMetadata{
				Semantics: &EndpointSemantics{
					PredicateMode:    EndpointPredicateModeEventual,
					MandatoryTargets: []string{"checkout", "email"},
					DependencyModes:  []string{string(EdgeKindSync), string(EdgeKindAsync)},
					Source:           "service-catalog",
					Confidence:       &confidence,
				},
			},
		}},
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

	mdl.Endpoints[0].Metadata.Semantics.MandatoryTargets = []string{"missing"}
	if err := mdl.ValidateSemantic(); err == nil {
		t.Fatal("expected unknown mandatory target validation error, got nil")
	}
}
