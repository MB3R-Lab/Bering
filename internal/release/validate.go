package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

type ValidateOptions struct {
	RepoRoot                  string
	DistDir                   string
	AppVersion                string
	BuildDate                 string
	RequirePublishedOCI       bool
	AllowChartVersionMismatch bool
}

func ValidateRelease(opts ValidateOptions) error {
	if err := ValidateSemVer(opts.AppVersion); err != nil {
		return err
	}

	contractsManifest, err := readContractsManifest(filepath.Join(opts.DistDir, ContractsManifestName))
	if err != nil {
		return fmt.Errorf("read contracts manifest: %w", err)
	}
	if err := validateJSONFileAgainstSchema(
		filepath.Join(opts.RepoRoot, "api", "release", "contracts-manifest.schema.json"),
		ContractsManifestSchemaURI,
		filepath.Join(opts.DistDir, ContractsManifestName),
	); err != nil {
		return fmt.Errorf("contracts manifest schema validation failed: %w", err)
	}

	expectedContracts, err := BuildContractsManifest(opts.RepoRoot, opts.AppVersion, opts.BuildDate)
	if err != nil {
		return err
	}
	if !contractsManifestEqual(contractsManifest, expectedContracts) {
		return fmt.Errorf("contracts manifest does not match the schema sources in api/schema")
	}

	if err := validateJSONFileAgainstSchema(
		filepath.Join(opts.RepoRoot, "api", "release", "release-manifest.schema.json"),
		ReleaseManifestSchemaURI,
		filepath.Join(opts.DistDir, ReleaseManifestName),
	); err != nil {
		return fmt.Errorf("release manifest schema validation failed: %w", err)
	}

	var releaseManifest ReleaseManifest
	if err := readJSON(filepath.Join(opts.DistDir, ReleaseManifestName), &releaseManifest); err != nil {
		return fmt.Errorf("read release manifest: %w", err)
	}
	if releaseManifest.AppVersion != opts.AppVersion {
		return fmt.Errorf("release manifest app version mismatch: got=%s want=%s", releaseManifest.AppVersion, opts.AppVersion)
	}
	if strings.TrimSpace(opts.BuildDate) != "" {
		expectedBuildDate, err := normalizeBuildDate(opts.BuildDate)
		if err != nil {
			return err
		}
		if releaseManifest.BuildDate != expectedBuildDate {
			return fmt.Errorf("release manifest build date mismatch: got=%s want=%s", releaseManifest.BuildDate, expectedBuildDate)
		}
	}
	if !contractsEqual(releaseManifest.Contracts, contractsManifest.Contracts) {
		return fmt.Errorf("release manifest contracts do not match contracts manifest")
	}
	if releaseManifest.ContractsPack.Manifest != ContractsManifestName {
		return fmt.Errorf("release manifest contracts pack manifest path mismatch: got=%s want=%s", releaseManifest.ContractsPack.Manifest, ContractsManifestName)
	}
	contractsPackDigest, err := DigestFile(filepath.Join(opts.DistDir, filepath.FromSlash(releaseManifest.ContractsPack.Archive)))
	if err != nil {
		return fmt.Errorf("digest contracts pack: %w", err)
	}
	if contractsPackDigest != releaseManifest.ContractsPack.Digest {
		return fmt.Errorf("contracts pack digest mismatch: got=%s want=%s", contractsPackDigest, releaseManifest.ContractsPack.Digest)
	}

	chartMetadataPath := filepath.Join(opts.DistDir, "chart-metadata.json")
	var chartMetadata ChartMetadata
	if err := readJSON(chartMetadataPath, &chartMetadata); err != nil {
		return fmt.Errorf("read chart metadata: %w", err)
	}
	if chartMetadata.AppVersion != opts.AppVersion {
		return fmt.Errorf("chart appVersion mismatch: got=%s want=%s", chartMetadata.AppVersion, opts.AppVersion)
	}
	if chartMetadata.Version != opts.AppVersion && !opts.AllowChartVersionMismatch {
		return fmt.Errorf("chart version %s drifted from app version %s without ALLOW_CHART_VERSION_MISMATCH=1", chartMetadata.Version, opts.AppVersion)
	}
	if releaseManifest.HelmChart != chartMetadata {
		return fmt.Errorf("release manifest helm_chart does not match chart-metadata.json")
	}
	packagedChart, err := ReadPackagedChartMetadata(filepath.Join(opts.DistDir, filepath.FromSlash(chartMetadata.Package)))
	if err != nil {
		return fmt.Errorf("read packaged chart metadata: %w", err)
	}
	if packagedChart.Name != chartMetadata.Name || packagedChart.Version != chartMetadata.Version || packagedChart.AppVersion != chartMetadata.AppVersion {
		return fmt.Errorf("packaged chart metadata does not match chart-metadata.json")
	}
	packageDigest, err := DigestFile(filepath.Join(opts.DistDir, filepath.FromSlash(chartMetadata.Package)))
	if err != nil {
		return fmt.Errorf("digest chart package: %w", err)
	}
	if packageDigest != chartMetadata.PackageDigest {
		return fmt.Errorf("chart package digest mismatch: got=%s want=%s", packageDigest, chartMetadata.PackageDigest)
	}
	if opts.RequirePublishedOCI && (!chartMetadata.Published || strings.TrimSpace(chartMetadata.Digest) == "") {
		return fmt.Errorf("chart must be published with a digest in release mode")
	}

	var imageManifest OCIImageManifest
	if err := readJSON(filepath.Join(opts.DistDir, "oci-image-manifest.json"), &imageManifest); err != nil {
		return fmt.Errorf("read image metadata: %w", err)
	}
	if len(releaseManifest.OCIImages) != 1 || !ociImageManifestEqual(releaseManifest.OCIImages[0], imageManifest) {
		return fmt.Errorf("release manifest oci_images does not match oci-image-manifest.json")
	}
	if opts.RequirePublishedOCI {
		if !imageManifest.Published {
			return fmt.Errorf("oci image must be published in release mode")
		}
		if strings.TrimSpace(imageManifest.Digest) == "" {
			return fmt.Errorf("oci image digest is missing")
		}
	}

	if err := validateChecksums(filepath.Join(opts.DistDir, ReleaseChecksumsName), opts.DistDir); err != nil {
		return fmt.Errorf("release checksums validation failed: %w", err)
	}
	checksumsDigest, err := DigestFile(filepath.Join(opts.DistDir, ReleaseChecksumsName))
	if err != nil {
		return fmt.Errorf("digest release checksums: %w", err)
	}
	if releaseManifest.Checksums.File != ReleaseChecksumsName || releaseManifest.Checksums.Digest != checksumsDigest {
		return fmt.Errorf("release manifest checksums metadata does not match %s", ReleaseChecksumsName)
	}

	return nil
}

func validateJSONFileAgainstSchema(schemaPath, schemaURI, documentPath string) error {
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	documentRaw, err := os.ReadFile(documentPath)
	if err != nil {
		return err
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaURI, bytes.NewReader(schemaRaw)); err != nil {
		return err
	}
	compiled, err := compiler.Compile(schemaURI)
	if err != nil {
		return err
	}

	var payload any
	if err := json.Unmarshal(documentRaw, &payload); err != nil {
		return err
	}
	return compiled.Validate(payload)
}

func validateChecksums(path, distDir string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	sort.Strings(lines)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return fmt.Errorf("invalid checksums line %q", line)
		}
		want := "sha256:" + strings.TrimSpace(parts[0])
		target := filepath.Join(distDir, filepath.FromSlash(parts[1]))
		got, err := DigestFile(target)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("checksum mismatch for %s: got=%s want=%s", parts[1], got, want)
		}
	}
	return nil
}

func contractsManifestEqual(a, b ContractsManifest) bool {
	return a.ProductName == b.ProductName &&
		a.AppVersion == b.AppVersion &&
		a.Schema == b.Schema &&
		contractsEqual(a.Contracts, b.Contracts)
}

func contractsEqual(a, b []ContractDescriptor) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ociImageManifestEqual(a, b OCIImageManifest) bool {
	if a.Repository != b.Repository ||
		a.Digest != b.Digest ||
		a.LocalLayout != b.LocalLayout ||
		a.Published != b.Published ||
		a.MediaType != b.MediaType ||
		a.BuildDate != b.BuildDate ||
		a.GitCommit != b.GitCommit ||
		a.SourceArtifact != b.SourceArtifact {
		return false
	}
	return stringSlicesEqual(a.References, b.References) &&
		stringSlicesEqual(a.Tags, b.Tags) &&
		stringSlicesEqual(a.Platforms, b.Platforms)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func normalizeBuildDate(value string) (string, error) {
	ts, err := parseBuildDate(value)
	if err != nil {
		return "", err
	}
	return ts.Format(timeRFC3339), nil
}
