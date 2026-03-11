package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ReleaseManifestOptions struct {
	RepoRoot          string
	DistDir           string
	AppVersion        string
	GitCommit         string
	GitTag            string
	BuildDate         string
	Headline          string
	Summary           string
	Highlights        []string
	BreakingCLI       bool
	BreakingAPI       bool
	BreakingInstall   bool
	BreakingContracts []string
}

type goreleaserArtifact struct {
	Type   string         `json:"type"`
	Path   string         `json:"path"`
	Name   string         `json:"name"`
	Goos   string         `json:"goos"`
	Goarch string         `json:"goarch"`
	Extra  map[string]any `json:"extra"`
}

func GenerateReleaseManifest(opts ReleaseManifestOptions) (ReleaseManifest, error) {
	if err := ValidateSemVer(opts.AppVersion); err != nil {
		return ReleaseManifest{}, err
	}
	buildTime, err := parseBuildDate(opts.BuildDate)
	if err != nil {
		return ReleaseManifest{}, err
	}

	if err := copyFile(filepath.Join(opts.RepoRoot, "api", "release", "release-manifest.schema.json"), filepath.Join(opts.DistDir, "release-manifest.schema.json")); err != nil {
		return ReleaseManifest{}, err
	}
	if err := copyFile(filepath.Join(opts.RepoRoot, "api", "release", "contracts-manifest.schema.json"), filepath.Join(opts.DistDir, "contracts-manifest.schema.json")); err != nil {
		return ReleaseManifest{}, err
	}

	artifacts, err := loadGoreleaserArtifacts(filepath.Join(opts.DistDir, "artifacts.json"))
	if err != nil {
		return ReleaseManifest{}, err
	}

	contractsManifest, err := readContractsManifest(filepath.Join(opts.DistDir, ContractsManifestName))
	if err != nil {
		return ReleaseManifest{}, err
	}
	chartMetadata, err := readChartMetadata(filepath.Join(opts.DistDir, "chart-metadata.json"))
	if err != nil {
		return ReleaseManifest{}, err
	}
	imageManifest, err := readOCIImageManifest(filepath.Join(opts.DistDir, "oci-image-manifest.json"))
	if err != nil {
		return ReleaseManifest{}, err
	}

	binaries := buildBinaryArtifacts(opts.DistDir, artifacts)
	notes := defaultReleaseNotes(opts, chartMetadata, imageManifest)
	notesPath := filepath.Join(opts.DistDir, ReleaseNotesName)
	if err := os.WriteFile(notesPath, []byte(renderReleaseNotesMarkdown(opts.AppVersion, notes)), 0o644); err != nil {
		return ReleaseManifest{}, err
	}

	contractsArchive := filepath.Join(opts.DistDir, fmt.Sprintf("%s-contracts_%s.tar.gz", ProductName, opts.AppVersion))
	contractsDigest, err := DigestFile(contractsArchive)
	if err != nil {
		return ReleaseManifest{}, err
	}

	assetPaths := collectReleaseAssetPaths(opts.DistDir, artifacts, chartMetadata, imageManifest, contractsArchive)
	assetPaths = append(assetPaths, ReleaseNotesName)
	releaseChecksums, err := writeReleaseChecksums(opts.DistDir, assetPaths)
	if err != nil {
		return ReleaseManifest{}, err
	}

	manifest := ReleaseManifest{
		Schema:      ReleaseManifestSchema(),
		ProductName: ProductName,
		AppVersion:  opts.AppVersion,
		GitTag:      strings.TrimSpace(opts.GitTag),
		GitCommit:   opts.GitCommit,
		BuildDate:   buildTime.Format(timeRFC3339),
		Binaries:    binaries,
		Checksums:   releaseChecksums,
		OCIImages:   []OCIImageManifest{imageManifest},
		HelmChart:   chartMetadata,
		ContractsPack: ContractsPack{
			Archive:  normalizePath(filepath.Base(contractsArchive)),
			Digest:   contractsDigest,
			Manifest: normalizePath(ContractsManifestName),
		},
		Contracts:    contractsManifest.Contracts,
		ReleaseNotes: notes,
	}

	manifestPath := filepath.Join(opts.DistDir, ReleaseManifestName)
	if err := writeJSON(manifestPath, manifest); err != nil {
		return ReleaseManifest{}, err
	}

	assetPaths = append(assetPaths, filepath.Base(filepath.Join(opts.DistDir, ReleaseChecksumsName)), filepath.Base(manifestPath), filepath.Base(notesPath))
	if err := writeReleaseAssetsList(filepath.Join(opts.DistDir, "release-assets.txt"), assetPaths); err != nil {
		return ReleaseManifest{}, err
	}

	return manifest, nil
}

func loadGoreleaserArtifacts(path string) ([]goreleaserArtifact, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read goreleaser artifacts: %w", err)
	}

	var items []goreleaserArtifact
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}

	var wrapped struct {
		Artifacts []goreleaserArtifact `json:"artifacts"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("decode goreleaser artifacts: %w", err)
	}
	return wrapped.Artifacts, nil
}

func readContractsManifest(path string) (ContractsManifest, error) {
	var manifest ContractsManifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func readChartMetadata(path string) (ChartMetadata, error) {
	var metadata ChartMetadata
	raw, err := os.ReadFile(path)
	if err != nil {
		return metadata, err
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return metadata, err
	}
	return metadata, nil
}

func readOCIImageManifest(path string) (OCIImageManifest, error) {
	var manifest OCIImageManifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func buildBinaryArtifacts(distDir string, artifacts []goreleaserArtifact) []BinaryArtifact {
	archiveByTarget := map[string]goreleaserArtifact{}
	sbomByTarget := map[string]goreleaserArtifact{}
	binaries := make([]BinaryArtifact, 0)

	for _, artifact := range artifacts {
		key := targetKey(artifact.Goos, artifact.Goarch)
		switch artifact.Type {
		case "Archive":
			archiveByTarget[key] = artifact
		case "SBOM":
			sbomByTarget[key] = artifact
		}
	}

	for _, artifact := range artifacts {
		if artifact.Type != "Binary" {
			continue
		}
		key := targetKey(artifact.Goos, artifact.Goarch)
		entry := BinaryArtifact{
			ID:     strings.TrimSpace(stringValue(artifact.Extra, "ID")),
			OS:     artifact.Goos,
			Arch:   artifact.Goarch,
			Target: key,
			Binary: defaultString(stringValue(artifact.Extra, "Binary"), ProductName),
			Path:   relativeArtifactPath(distDir, artifact.Path),
		}
		if archive, ok := archiveByTarget[key]; ok {
			entry.Archive = relativeArtifactPath(distDir, archive.Path)
			entry.Format = strings.TrimSpace(defaultString(stringValue(archive.Extra, "Format"), archiveFormat(archive.Path)))
			entry.ArchiveID = strings.TrimSpace(stringValue(archive.Extra, "ID"))
			entry.Checksum = strings.TrimSpace(stringValue(archive.Extra, "Checksum"))
		}
		if sbom, ok := sbomByTarget[key]; ok {
			entry.SBOM = relativeArtifactPath(distDir, sbom.Path)
		}
		if entry.Checksum == "" {
			entry.Checksum = strings.TrimSpace(stringValue(artifact.Extra, "Checksum"))
		}
		binaries = append(binaries, entry)
	}

	sort.Slice(binaries, func(i, j int) bool {
		if binaries[i].OS != binaries[j].OS {
			return binaries[i].OS < binaries[j].OS
		}
		return binaries[i].Arch < binaries[j].Arch
	})
	return binaries
}

func collectReleaseAssetPaths(distDir string, artifacts []goreleaserArtifact, chart ChartMetadata, image OCIImageManifest, contractsArchive string) []string {
	set := map[string]struct{}{}
	add := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		set[normalizePath(path)] = struct{}{}
	}

	for _, artifact := range artifacts {
		switch artifact.Type {
		case "Archive", "SBOM", "Checksum":
			add(relativeArtifactPath(distDir, artifact.Path))
		}
	}

	add(normalizePath(filepath.Base(contractsArchive)))
	add(normalizePath(ContractsManifestName))
	add("contracts-manifest.schema.json")
	add(chart.Package)
	add("chart-metadata.json")
	add("oci-image-manifest.json")
	if image.LocalLayout != "" {
		add(image.LocalLayout)
	}
	add("release-manifest.schema.json")

	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func writeReleaseChecksums(distDir string, assetPaths []string) (ReleaseChecksums, error) {
	entries := make([]ChecksumEntry, 0, len(assetPaths))
	lines := make([]string, 0, len(assetPaths))
	for _, relPath := range assetPaths {
		absPath := filepath.Join(distDir, filepath.FromSlash(relPath))
		digest, err := DigestFile(absPath)
		if err != nil {
			return ReleaseChecksums{}, err
		}
		entries = append(entries, ChecksumEntry{
			Name:      filepath.Base(relPath),
			Path:      normalizePath(relPath),
			Algorithm: "sha256",
			Digest:    digest,
		})
		lines = append(lines, fmt.Sprintf("%s  %s", strings.TrimPrefix(digest, "sha256:"), normalizePath(relPath)))
	}
	sort.Strings(lines)

	checksumsPath := filepath.Join(distDir, ReleaseChecksumsName)
	if err := os.WriteFile(checksumsPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return ReleaseChecksums{}, err
	}

	digest, err := DigestFile(checksumsPath)
	if err != nil {
		return ReleaseChecksums{}, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return ReleaseChecksums{
		Algorithm: "sha256",
		File:      ReleaseChecksumsName,
		Digest:    digest,
		Entries:   entries,
	}, nil
}

func writeReleaseAssetsList(path string, assetPaths []string) error {
	unique := map[string]struct{}{}
	for _, asset := range assetPaths {
		unique[normalizePath(asset)] = struct{}{}
	}
	list := make([]string, 0, len(unique))
	for asset := range unique {
		list = append(list, asset)
	}
	sort.Strings(list)
	return os.WriteFile(path, []byte(strings.Join(list, "\n")+"\n"), 0o644)
}

func defaultReleaseNotes(opts ReleaseManifestOptions, chart ChartMetadata, image OCIImageManifest) ReleaseNotes {
	headline := strings.TrimSpace(opts.Headline)
	if headline == "" {
		headline = fmt.Sprintf("Bering %s", TagForVersion(opts.AppVersion))
	}
	summary := strings.TrimSpace(opts.Summary)
	if summary == "" {
		summary = "Platform-neutral release with reproducible binaries, OCI image metadata, an OCI-publishable Helm chart, and a generated contracts pack."
	}

	highlights := append([]string(nil), opts.Highlights...)
	if len(highlights) == 0 {
		highlights = []string{
			fmt.Sprintf("Cross-platform binaries for %d targets with SBOM and checksums.", len(defaultBinaryTargets())),
			fmt.Sprintf("Helm chart %s packaged at version %s.", chart.Name, chart.Version),
			fmt.Sprintf("OCI image metadata generated for %s.", image.Repository),
		}
	}

	return ReleaseNotes{
		Headline:   headline,
		Summary:    summary,
		Highlights: highlights,
		Breaking: BreakingSurface{
			CLI:             opts.BreakingCLI,
			API:             opts.BreakingAPI,
			InstallSurface:  opts.BreakingInstall,
			SchemaContracts: append([]string{}, opts.BreakingContracts...),
		},
	}
}

func renderReleaseNotesMarkdown(version string, notes ReleaseNotes) string {
	lines := []string{
		"# " + notes.Headline,
		"",
		notes.Summary,
		"",
		"## Highlights",
	}
	for _, item := range notes.Highlights {
		lines = append(lines, "- "+item)
	}
	lines = append(lines, "", "## Breaking Surface")
	lines = append(lines,
		fmt.Sprintf("- CLI/API: %t/%t", notes.Breaking.CLI, notes.Breaking.API),
		fmt.Sprintf("- Install surface: %t", notes.Breaking.InstallSurface),
	)
	if len(notes.Breaking.SchemaContracts) == 0 {
		lines = append(lines, "- Schema contracts: none")
	} else {
		lines = append(lines, "- Schema contracts: "+strings.Join(notes.Breaking.SchemaContracts, ", "))
	}
	lines = append(lines, "", "Release tag: "+TagForVersion(version), "")
	return strings.Join(lines, "\n")
}

func targetKey(goos, goarch string) string {
	return strings.TrimSpace(goos) + "/" + strings.TrimSpace(goarch)
}

func relativeArtifactPath(distDir, artifactPath string) string {
	clean := filepath.Clean(artifactPath)
	if filepath.IsAbs(clean) {
		return normalizePath(relativeTo(distDir, clean))
	}
	if strings.HasPrefix(normalizePath(clean), normalizePath(distDir)+"/") {
		return normalizePath(strings.TrimPrefix(normalizePath(clean), normalizePath(distDir)+"/"))
	}
	return normalizePath(clean)
}

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultBinaryTargets() []string {
	return []string{
		"linux/amd64",
		"linux/arm64",
		"darwin/amd64",
		"darwin/arm64",
		"windows/amd64",
	}
}

func archiveFormat(path string) string {
	normalized := normalizePath(path)
	switch {
	case strings.HasSuffix(normalized, ".tar.gz"):
		return "tar.gz"
	case strings.HasSuffix(normalized, ".zip"):
		return "zip"
	default:
		return strings.TrimPrefix(filepath.Ext(normalized), ".")
	}
}
