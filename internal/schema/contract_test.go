package schema

import "testing"

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
