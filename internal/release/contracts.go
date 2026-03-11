package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MB3R-Lab/Bering/internal/schema"
)

type ContractsPackOptions struct {
	RepoRoot   string
	DistDir    string
	AppVersion string
	BuildDate  string
}

type contractSource struct {
	Name    string
	Version string
	URI     string
	Digest  string
	File    string
	Kind    string
}

func publicContractSources(repoRoot string) []contractSource {
	return []contractSource{
		{
			Name:    schema.ExpectedSchemaName,
			Version: schema.ExpectedSchemaVersion,
			URI:     schema.ExpectedSchemaURI,
			Digest:  schema.ExpectedSchemaDigest,
			File:    filepath.Join(repoRoot, "api", "schema", "model.schema.json"),
			Kind:    "json-schema",
		},
		{
			Name:    schema.ExpectedSnapshotSchemaName,
			Version: schema.ExpectedSnapshotSchemaVersion,
			URI:     schema.ExpectedSnapshotSchemaURI,
			Digest:  schema.ExpectedSnapshotSchemaDigest,
			File:    filepath.Join(repoRoot, "api", "schema", "snapshot.schema.json"),
			Kind:    "json-schema",
		},
	}
}

func BuildContractsManifest(repoRoot, appVersion, buildDate string) (ContractsManifest, error) {
	if err := ValidateSemVer(appVersion); err != nil {
		return ContractsManifest{}, err
	}
	timestamp, err := parseBuildDate(buildDate)
	if err != nil {
		return ContractsManifest{}, err
	}

	contracts := make([]ContractDescriptor, 0, len(publicContractSources(repoRoot)))
	for _, source := range publicContractSources(repoRoot) {
		raw, err := os.ReadFile(source.File)
		if err != nil {
			return ContractsManifest{}, fmt.Errorf("read %s: %w", source.File, err)
		}
		digest := digestBytes(raw)
		if digest != source.Digest {
			return ContractsManifest{}, fmt.Errorf("digest mismatch for %s: got=%s want=%s", source.Name, digest, source.Digest)
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return ContractsManifest{}, fmt.Errorf("decode %s: %w", source.File, err)
		}
		id, _ := payload["$id"].(string)
		if strings.TrimSpace(id) != source.URI {
			return ContractsManifest{}, fmt.Errorf("schema id mismatch for %s: got=%q want=%q", source.Name, id, source.URI)
		}

		contracts = append(contracts, ContractDescriptor{
			Name:    source.Name,
			Version: source.Version,
			Kind:    source.Kind,
			File:    normalizePath(filepath.Join("schema", filepath.Base(source.File))),
			URI:     source.URI,
			Digest:  digest,
		})
	}

	return ContractsManifest{
		Schema:      ContractsManifestSchema(),
		ProductName: ProductName,
		AppVersion:  appVersion,
		GeneratedAt: timestamp.Format(timeRFC3339),
		Contracts:   contracts,
	}, nil
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func GenerateContractsPack(opts ContractsPackOptions) (ContractsManifest, string, error) {
	manifest, err := BuildContractsManifest(opts.RepoRoot, opts.AppVersion, opts.BuildDate)
	if err != nil {
		return ContractsManifest{}, "", err
	}
	buildTime, err := parseBuildDate(opts.BuildDate)
	if err != nil {
		return ContractsManifest{}, "", err
	}

	if err := ensureDir(opts.DistDir); err != nil {
		return ContractsManifest{}, "", err
	}

	manifestPath := filepath.Join(opts.DistDir, ContractsManifestName)
	if err := writeJSON(manifestPath, manifest); err != nil {
		return ContractsManifest{}, "", err
	}

	contractsSchemaPath := filepath.Join(opts.DistDir, "contracts-manifest.schema.json")
	if err := copyFile(filepath.Join(opts.RepoRoot, "api", "release", "contracts-manifest.schema.json"), contractsSchemaPath); err != nil {
		return ContractsManifest{}, "", err
	}

	files := map[string]string{
		ContractsManifestName:            manifestPath,
		"contracts-manifest.schema.json": contractsSchemaPath,
	}
	for _, contract := range manifest.Contracts {
		src := filepath.Join(opts.RepoRoot, "api", "schema", filepath.Base(contract.File))
		files[contract.File] = src
	}

	archiveName := fmt.Sprintf("%s-contracts_%s.tar.gz", ProductName, opts.AppVersion)
	archivePath := filepath.Join(opts.DistDir, archiveName)
	if err := writeTarGz(archivePath, buildTime, files); err != nil {
		return ContractsManifest{}, "", err
	}

	return manifest, archivePath, nil
}
