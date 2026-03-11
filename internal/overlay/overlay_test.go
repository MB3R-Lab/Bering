package overlay

import (
	"path/filepath"
	"testing"
)

func TestFileNormalizeCanonicalizesRef(t *testing.T) {
	t.Parallel()

	item := File{Name: "test"}
	if err := item.Normalize(filepath.Join("configs", "nested", "overlay.yaml")); err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if item.Ref != "configs/nested/overlay.yaml" {
		t.Fatalf("ref mismatch: got=%s", item.Ref)
	}
}
