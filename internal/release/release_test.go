package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	testVersion   = "1.2.3"
	testBuildDate = "2026-03-11T12:00:00Z"
	testGitSHA    = "0123456789abcdef0123456789abcdef01234567"
)

func TestGenerateAndValidateRelease(t *testing.T) {
	t.Parallel()

	repoRoot := testRepoRoot(t)
	distDir := filepath.Join(t.TempDir(), "dist")
	if err := ensureDir(distDir); err != nil {
		t.Fatal(err)
	}

	if _, _, err := GenerateContractsPack(ContractsPackOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("generate contracts pack: %v", err)
	}

	writeTestGoreleaserArtifacts(t, distDir)
	writeTestChartMetadata(t, distDir)
	writeTestOCIImageMetadata(t, distDir)

	if _, err := GenerateReleaseManifest(ReleaseManifestOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		GitCommit:  testGitSHA,
		GitTag:     TagForVersion(testVersion),
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("generate release manifest: %v", err)
	}

	if err := ValidateRelease(ValidateOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("validate release: %v", err)
	}
}

func TestBuildContractsManifestIncludesPublishedContractHistory(t *testing.T) {
	t.Parallel()

	repoRoot := testRepoRoot(t)
	manifest, err := BuildContractsManifest(repoRoot, testVersion, testBuildDate)
	if err != nil {
		t.Fatalf("BuildContractsManifest returned error: %v", err)
	}
	if got, want := len(manifest.Contracts), 4; got != want {
		t.Fatalf("contract count mismatch: got=%d want=%d", got, want)
	}
	if got, want := manifest.Contracts[0].File, "schema/model/v1.0.0/model.schema.json"; got != want {
		t.Fatalf("first contract file mismatch: got=%s want=%s", got, want)
	}
	if got, want := manifest.Contracts[3].File, "schema/snapshot/v1.1.0/snapshot.schema.json"; got != want {
		t.Fatalf("last contract file mismatch: got=%s want=%s", got, want)
	}
}

func TestValidateReleaseFailsOnChartVersionDrift(t *testing.T) {
	t.Parallel()

	repoRoot := testRepoRoot(t)
	distDir := filepath.Join(t.TempDir(), "dist")
	if err := ensureDir(distDir); err != nil {
		t.Fatal(err)
	}

	if _, _, err := GenerateContractsPack(ContractsPackOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("generate contracts pack: %v", err)
	}

	writeTestGoreleaserArtifacts(t, distDir)
	writeTestChartMetadata(t, distDir)
	writeTestOCIImageMetadata(t, distDir)

	if _, err := GenerateReleaseManifest(ReleaseManifestOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		GitCommit:  testGitSHA,
		GitTag:     TagForVersion(testVersion),
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("generate release manifest: %v", err)
	}

	var chartMetadata ChartMetadata
	if err := readJSON(filepath.Join(distDir, "chart-metadata.json"), &chartMetadata); err != nil {
		t.Fatalf("read chart metadata: %v", err)
	}
	chartMetadata.Version = "9.9.9"
	if err := writeJSON(filepath.Join(distDir, "chart-metadata.json"), chartMetadata); err != nil {
		t.Fatalf("rewrite chart metadata: %v", err)
	}

	if err := ValidateRelease(ValidateOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		BuildDate:  testBuildDate,
	}); err == nil {
		t.Fatal("expected chart version drift validation failure")
	}
}

func TestValidateReleaseFailsOnChecksumDrift(t *testing.T) {
	t.Parallel()

	repoRoot := testRepoRoot(t)
	distDir := filepath.Join(t.TempDir(), "dist")
	if err := ensureDir(distDir); err != nil {
		t.Fatal(err)
	}

	if _, _, err := GenerateContractsPack(ContractsPackOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("generate contracts pack: %v", err)
	}

	writeTestGoreleaserArtifacts(t, distDir)
	writeTestChartMetadata(t, distDir)
	writeTestOCIImageMetadata(t, distDir)

	if _, err := GenerateReleaseManifest(ReleaseManifestOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		GitCommit:  testGitSHA,
		GitTag:     TagForVersion(testVersion),
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("generate release manifest: %v", err)
	}

	if err := os.WriteFile(filepath.Join(distDir, "chart-metadata.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("corrupt chart metadata: %v", err)
	}

	if err := ValidateRelease(ValidateOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		BuildDate:  testBuildDate,
	}); err == nil {
		t.Fatal("expected checksum validation failure")
	}
}

func TestValidateReleaseNormalizesBuildDate(t *testing.T) {
	t.Parallel()

	repoRoot := testRepoRoot(t)
	distDir := filepath.Join(t.TempDir(), "dist")
	if err := ensureDir(distDir); err != nil {
		t.Fatal(err)
	}

	if _, _, err := GenerateContractsPack(ContractsPackOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("generate contracts pack: %v", err)
	}

	writeTestGoreleaserArtifacts(t, distDir)
	writeTestChartMetadata(t, distDir)
	writeTestOCIImageMetadata(t, distDir)

	if _, err := GenerateReleaseManifest(ReleaseManifestOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		GitCommit:  testGitSHA,
		GitTag:     TagForVersion(testVersion),
		BuildDate:  testBuildDate,
	}); err != nil {
		t.Fatalf("generate release manifest: %v", err)
	}

	if err := ValidateRelease(ValidateOptions{
		RepoRoot:   repoRoot,
		DistDir:    distDir,
		AppVersion: testVersion,
		BuildDate:  "2026-03-11T15:00:00+03:00",
	}); err != nil {
		t.Fatalf("validate release with equivalent timestamp: %v", err)
	}
}

func TestExtractChangelogSection(t *testing.T) {
	t.Parallel()

	markdown := strings.Join([]string{
		"# Changelog",
		"",
		"## v0.1.0",
		"",
		"First public release.",
		"",
		"## v0.0.1",
		"",
		"Bootstrap.",
		"",
	}, "\n")

	section, ok := extractChangelogSection(markdown, "0.1.0")
	if !ok {
		t.Fatal("expected changelog section to be found")
	}

	want := strings.Join([]string{
		"## v0.1.0",
		"",
		"First public release.",
		"",
	}, "\n")
	if section != want {
		t.Fatalf("unexpected changelog section:\nwant:\n%s\ngot:\n%s", want, section)
	}
}

func TestExtractChangelogSectionMissing(t *testing.T) {
	t.Parallel()

	if section, ok := extractChangelogSection("# Changelog\n", "9.9.9"); ok || section != "" {
		t.Fatalf("expected missing section, got ok=%t section=%q", ok, section)
	}
}

func writeTestGoreleaserArtifacts(t *testing.T, distDir string) {
	t.Helper()

	archiveName := "bering_1.2.3_linux_amd64.tar.gz"
	sbomName := archiveName + ".sbom.json"
	checksumName := "bering_1.2.3_checksums.txt"
	binaryPath := filepath.Join(distDir, "bering")
	archivePath := filepath.Join(distDir, archiveName)
	sbomPath := filepath.Join(distDir, sbomName)
	checksumPath := filepath.Join(distDir, checksumName)

	if err := os.WriteFile(binaryPath, []byte("binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, []byte("archive\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sbomPath, []byte("{\"sbom\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checksumPath, []byte("deadbeef  "+archiveName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	archiveDigest, err := DigestFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	artifacts := []goreleaserArtifact{
		{
			Type:   "Binary",
			Path:   binaryPath,
			Name:   "bering",
			Goos:   "linux",
			Goarch: "amd64",
			Extra: map[string]any{
				"Binary": "bering",
				"ID":     "bering_linux_amd64",
			},
		},
		{
			Type:   "Archive",
			Path:   archivePath,
			Name:   archiveName,
			Goos:   "linux",
			Goarch: "amd64",
			Extra: map[string]any{
				"Checksum": archiveDigest,
				"Format":   "tar.gz",
				"ID":       "release",
			},
		},
		{
			Type:   "SBOM",
			Path:   sbomPath,
			Name:   sbomName,
			Goos:   "linux",
			Goarch: "amd64",
		},
		{
			Type: "Checksum",
			Path: checksumPath,
			Name: checksumName,
		},
	}
	raw, err := json.Marshal(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "artifacts.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestChartMetadata(t *testing.T, distDir string) {
	t.Helper()

	chartsDir := filepath.Join(distDir, "charts")
	if err := ensureDir(chartsDir); err != nil {
		t.Fatal(err)
	}

	chartSourceDir := filepath.Join(distDir, "chart-src")
	if err := ensureDir(chartSourceDir); err != nil {
		t.Fatal(err)
	}

	chartYAMLPath := filepath.Join(chartSourceDir, "Chart.yaml")
	if err := os.WriteFile(chartYAMLPath, []byte("apiVersion: v2\nname: bering\nversion: 1.2.3\nappVersion: 1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	chartPackagePath := filepath.Join(chartsDir, "bering-1.2.3.tgz")
	if err := writeTarGz(chartPackagePath, mustBuildTime(t), map[string]string{
		"bering/Chart.yaml": chartYAMLPath,
	}); err != nil {
		t.Fatal(err)
	}

	packageDigest, err := DigestFile(chartPackagePath)
	if err != nil {
		t.Fatal(err)
	}

	metadata := ChartMetadata{
		Name:           "bering",
		Version:        testVersion,
		AppVersion:     testVersion,
		Package:        "charts/bering-1.2.3.tgz",
		PackageDigest:  packageDigest,
		Published:      false,
		SourceChartDir: "charts/bering",
	}
	if err := writeJSON(filepath.Join(distDir, "chart-metadata.json"), metadata); err != nil {
		t.Fatal(err)
	}
}

func writeTestOCIImageMetadata(t *testing.T, distDir string) {
	t.Helper()

	metadata := OCIImageManifest{
		Repository: "registry.example.com/bering",
		References: []string{
			"registry.example.com/bering:v1.2.3",
			"registry.example.com/bering:v1.2",
			"registry.example.com/bering:sha-" + testGitSHA,
		},
		Tags:      []string{"v1.2.3", "v1.2", "sha-" + testGitSHA},
		Platforms: []string{"linux/amd64", "linux/arm64"},
		Published: false,
		BuildDate: testBuildDate,
		GitCommit: testGitSHA,
	}
	if err := writeJSON(filepath.Join(distDir, "oci-image-manifest.json"), metadata); err != nil {
		t.Fatal(err)
	}
}

func testRepoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func mustBuildTime(t *testing.T) (ts time.Time) {
	t.Helper()
	var err error
	ts, err = parseBuildDate(testBuildDate)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
