package schema

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestEmbeddedSchemaDigestMatchesPinned(t *testing.T) {
	t.Parallel()

	if got, want := EmbeddedSchemaDigest(), ExpectedSchemaDigest; got != want {
		t.Fatalf("embedded schema digest mismatch: got=%s want=%s", got, want)
	}
}

func TestEmbeddedSnapshotSchemaDigestMatchesPinned(t *testing.T) {
	t.Parallel()

	if got, want := EmbeddedSnapshotSchemaDigest(), ExpectedSnapshotSchemaDigest; got != want {
		t.Fatalf("embedded snapshot schema digest mismatch: got=%s want=%s", got, want)
	}
}

func TestValidateStrict(t *testing.T) {
	t.Parallel()

	if err := ValidateStrict(ExpectedRef()); err != nil {
		t.Fatalf("expected strict validation to pass, got error: %v", err)
	}
	if err := ValidateSnapshotStrict(ExpectedSnapshotRef()); err != nil {
		t.Fatalf("expected snapshot strict validation to pass, got error: %v", err)
	}
}

func TestPublishedContractsHavePinnedDigestAndID(t *testing.T) {
	t.Parallel()

	for _, contract := range PublishedContracts() {
		raw, err := EmbeddedBytes(contract.Ref)
		if err != nil {
			t.Fatalf("embedded bytes for %s@%s: %v", contract.Ref.Name, contract.Ref.Version, err)
		}
		if got, want := mustEmbeddedDigest(contract.Ref), contract.Ref.Digest; got != want {
			t.Fatalf("embedded digest mismatch for %s@%s: got=%s want=%s", contract.Ref.Name, contract.Ref.Version, got, want)
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode embedded schema %s@%s: %v", contract.Ref.Name, contract.Ref.Version, err)
		}
		id, _ := payload["$id"].(string)
		if id != contract.Ref.URI {
			t.Fatalf("schema $id mismatch for %s@%s: got=%q want=%q", contract.Ref.Name, contract.Ref.Version, id, contract.Ref.URI)
		}

		parsed, err := url.Parse(contract.Ref.URI)
		if err != nil {
			t.Fatalf("parse schema uri for %s@%s: %v", contract.Ref.Name, contract.Ref.Version, err)
		}
		wantSegment := fmt.Sprintf("/v%s/", contract.Ref.Version)
		if !strings.Contains(parsed.Path, wantSegment) {
			t.Fatalf("schema uri path %q must contain %q", parsed.Path, wantSegment)
		}
	}
}
