package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/topology"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
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
	if !bytes.Equal(normalizeNewlines(actualRaw), normalizeNewlines(expectedRaw)) {
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
	if !bytes.Equal(normalizeNewlines(actualRaw), normalizeNewlines(expectedRaw)) {
		t.Fatalf("discover output mismatch\nactual:\n%s\nexpected:\n%s", actualRaw, expectedRaw)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = runner.Run([]string{"validate", "--input", out})
	if exitCode != ExitOK {
		t.Fatalf("validate failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}
}

func TestDiscoverSnapshotWithOverlay(t *testing.T) {
	t.Parallel()

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
	out := filepath.Join(t.TempDir(), "bering-model.json")
	snapshotOut := filepath.Join(t.TempDir(), "bering-snapshot.json")
	overlayPath := filepath.Join(t.TempDir(), "overlay.yaml")
	overlayRaw := []byte(`name: test-overlay
services:
  - id: checkout
    replicas: 3
endpoints:
  - id: checkout:POST /process
    predicate_ref: catalog.checkout.success
`)
	if err := os.WriteFile(overlayPath, overlayRaw, 0o644); err != nil {
		t.Fatalf("write overlay fixture: %v", err)
	}

	var stdout, stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	exitCode := runner.Run([]string{
		"discover",
		"--input", input,
		"--out", out,
		"--snapshot-out", snapshotOut,
		"--overlay", overlayPath,
		"--discovered-at", "2026-03-03T00:00:00Z",
	})
	if exitCode != ExitOK {
		t.Fatalf("discover failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = runner.Run([]string{"validate", "--input", snapshotOut})
	if exitCode != ExitOK {
		t.Fatalf("validate snapshot failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}
}

func TestDiscoverAndValidate_TopologyFixture(t *testing.T) {
	t.Parallel()

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

	input := filepath.Join("examples", "topology", "topology-api.sample.yaml")
	expected := filepath.Join(root, "examples", "outputs", "bering-model.topology-api.sample.json")
	out := filepath.Join(t.TempDir(), "bering-model.json")
	snapshotOut := filepath.Join(t.TempDir(), "bering-snapshot.json")

	var stdout, stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)

	exitCode := runner.Run([]string{
		"discover",
		"--input", input,
		"--out", out,
		"--snapshot-out", snapshotOut,
		"--discovered-at", "2026-03-11T00:00:00Z",
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
	if !bytes.Equal(normalizeNewlines(actualRaw), normalizeNewlines(expectedRaw)) {
		t.Fatalf("discover output mismatch\nactual:\n%s\nexpected:\n%s", actualRaw, expectedRaw)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = runner.Run([]string{"validate", "--input", out})
	if exitCode != ExitOK {
		t.Fatalf("validate model failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}

	exitCode = runner.Run([]string{"validate", "--input", snapshotOut})
	if exitCode != ExitOK {
		t.Fatalf("validate snapshot failed (exit=%d): stderr=%s", exitCode, stderr.String())
	}

	snapshotRaw, err := os.ReadFile(snapshotOut)
	if err != nil {
		t.Fatalf("read snapshot output: %v", err)
	}
	env, err := snapshot.ParseJSON(snapshotRaw)
	if err != nil {
		t.Fatalf("parse snapshot output: %v", err)
	}
	if got, want := env.Sources[0].Type, topology.SourceTypeTopologyAPI; got != want {
		t.Fatalf("snapshot source type mismatch: got=%s want=%s", got, want)
	}
	if got, want := env.Discovery.Services[0].Provenance[0].Type, topology.SourceTypeTopologyAPI; got != want {
		t.Fatalf("snapshot service provenance mismatch: got=%s want=%s", got, want)
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

func normalizeNewlines(raw []byte) []byte {
	return bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
}
