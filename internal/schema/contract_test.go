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

func TestValidateStrict(t *testing.T) {
	t.Parallel()

	if err := ValidateStrict(ExpectedRef()); err != nil {
		t.Fatalf("expected strict validation to pass, got error: %v", err)
	}
}

func TestEmbeddedSchemaIDMatchesExpectedURI(t *testing.T) {
	t.Parallel()

	var payload map[string]any
	if err := json.Unmarshal(EmbeddedSchema(), &payload); err != nil {
		t.Fatalf("decode embedded schema: %v", err)
	}

	id, _ := payload["$id"].(string)
	if id != ExpectedSchemaURI {
		t.Fatalf("schema $id mismatch: got=%q want=%q", id, ExpectedSchemaURI)
	}
}

func TestExpectedSchemaURIVersionPathMatchesConstant(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(ExpectedSchemaURI)
	if err != nil {
		t.Fatalf("parse ExpectedSchemaURI: %v", err)
	}

	wantSegment := fmt.Sprintf("/v%s/", ExpectedSchemaVersion)
	if !strings.Contains(parsed.Path, wantSegment) {
		t.Fatalf("ExpectedSchemaURI path %q must contain %q", parsed.Path, wantSegment)
	}
}
