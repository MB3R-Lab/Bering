package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/topology"
	"github.com/MB3R-Lab/Bering/internal/quality"
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
	if _, err := os.Stat(quality.SidecarPath(out)); err != nil {
		t.Fatalf("model signal quality report was not written: %v", err)
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

func TestServePublicationModeFlagRejectsInvalidValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	if code := runner.Run([]string{"serve", "--publication-mode", "unknown"}); code != ExitError {
		t.Fatalf("invalid mode exit=%d stderr=%s", code, stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("runtime.publication_mode")) {
		t.Fatalf("missing publication mode validation: %s", stderr.String())
	}
}

func TestDiscoverGenericEBPFRequiresAndRetainsCollector(t *testing.T) {
	root := repoRoot(t)
	input := filepath.Join(root, "examples", "traces", "otel.sample.json")
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)

	args := []string{
		"discover", "--input", input, "--input-type", "ebpf-otlp",
		"--source-domain", "deployment", "--out", filepath.Join(dir, "model.json"),
		"--snapshot-out", filepath.Join(dir, "snapshot.json"),
		"--discovered-at", "2026-03-03T00:00:00Z",
	}
	if code := runner.Run(args); code != ExitError || !bytes.Contains(stderr.Bytes(), []byte("--collector is required")) {
		t.Fatalf("missing collector was not rejected: code=%d stderr=%s", code, stderr.String())
	}
	stderr.Reset()
	legacyArgs := append([]string(nil), args...)
	for i := range legacyArgs {
		if legacyArgs[i] == "ebpf-otlp" {
			legacyArgs[i] = "ebpf-obi"
		}
	}
	legacyArgs = append(legacyArgs, "--collector", "collector-b")
	if code := runner.Run(legacyArgs); code != ExitError || !bytes.Contains(stderr.Bytes(), []byte("only supports collector obi")) {
		t.Fatalf("conflicting legacy collector was not rejected: code=%d stderr=%s", code, stderr.String())
	}
	stderr.Reset()
	args = append(args, "--collector", "collector-b")
	if code := runner.Run(args); code != ExitOK {
		t.Fatalf("generic eBPF discovery failed: code=%d stderr=%s", code, stderr.String())
	}
	raw, err := os.ReadFile(filepath.Join(dir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := snapshot.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Sources) != 1 || artifact.Sources[0].Type != "ebpf" || artifact.Sources[0].Connector != "collector-b" {
		t.Fatalf("collector source summary missing: %+v", artifact.Sources)
	}
	for _, service := range artifact.Discovery.Services {
		if len(service.Provenance) == 0 || service.Provenance[0].Connector != "collector-b" {
			t.Fatalf("collector service provenance missing: %+v", service)
		}
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
	if _, err := os.Stat(quality.SidecarPath(snapshotOut)); err != nil {
		t.Fatalf("snapshot signal quality report was not written: %v", err)
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
