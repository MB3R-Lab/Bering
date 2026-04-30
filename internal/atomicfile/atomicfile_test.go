package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileReplacesExistingContent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "artifact.json")
	if err := WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("initial WriteFile returned error: %v", err)
	}
	if err := WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("replacement WriteFile returned error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if got, want := string(raw), "new\n"; got != want {
		t.Fatalf("content mismatch: got=%q want=%q", got, want)
	}
}

func TestWriteFileCleansTemporaryFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.json")
	if err := WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".artifact.json.tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected temporary files to be cleaned up, got %v", matches)
	}
}
