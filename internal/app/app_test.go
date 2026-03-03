package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiscoverAndValidate_NormalizedFixture(t *testing.T) {
	root := repoRoot(t)
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()

	input := filepath.Join("examples", "traces", "normalized.sample.json")
	expected := filepath.Join(root, "examples", "outputs", "bering-model.normalized.sample.json")
	out := filepath.Join(t.TempDir(), "bering-model.json")

	var stdout, stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)

	exitCode := runner.Run([]string{
		"discover",
		"--input", input,
		"--out", out,
		"--discovered-at", "2026-03-03T00:00:00Z",
	})
	if exitCode != ExitOK {
		t.Fatalf("discover failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}

	actualRaw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read discovered output: %v", err)
	}
	expectedRaw, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("read expected output: %v", err)
	}
	if !bytes.Equal(actualRaw, expectedRaw) {
		t.Fatalf("discover output mismatch\nactual:\n%s\nexpected:\n%s", actualRaw, expectedRaw)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = runner.Run([]string{"validate", "--input", out})
	if exitCode != ExitOK {
		t.Fatalf("validate failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}
}

func TestDiscoverAndValidate_OTLPFixture(t *testing.T) {
	root := repoRoot(t)
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()

	input := filepath.Join("examples", "traces", "otel.sample.json")
	expected := filepath.Join(root, "examples", "outputs", "bering-model.otel.sample.json")
	out := filepath.Join(t.TempDir(), "bering-model.json")

	var stdout, stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)

	exitCode := runner.Run([]string{
		"discover",
		"--input", input,
		"--out", out,
		"--discovered-at", "2026-03-03T00:00:00Z",
	})
	if exitCode != ExitOK {
		t.Fatalf("discover failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}

	actualRaw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read discovered output: %v", err)
	}
	expectedRaw, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("read expected output: %v", err)
	}
	if !bytes.Equal(actualRaw, expectedRaw) {
		t.Fatalf("discover output mismatch\nactual:\n%s\nexpected:\n%s", actualRaw, expectedRaw)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = runner.Run([]string{"validate", "--input", out})
	if exitCode != ExitOK {
		t.Fatalf("validate failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}
