# Release Assets

The release payload is designed for both humans and automation.

## Primary Assets

- `bering_<version>_<os>_<arch>.tar.gz`
- `bering_<version>_windows_amd64.zip`
- `bering_<version>_<os>_<arch>.sbom.json`
- `bering_<version>_checksums.txt`
- `bering-contracts_<version>.tar.gz`
- `contracts-manifest.json`
- `contracts-manifest.schema.json`
- `charts/bering-<chart-version>.tgz`
- `chart-metadata.json`
- `oci-image-manifest.json`
- `release-checksums.txt`
- `release-manifest.json`
- `release-manifest.schema.json`
- `release-notes.md`

## What Each File Is For

### `release-manifest.json`

Canonical machine-readable summary of the release. Downstream systems should prefer this over scraping GitHub Releases.

Fields include:

- product name
- app version
- git commit and build date
- binary archive metadata
- OCI image tags and digest
- Helm chart package metadata
- owned contract versions and digests
- release notes summary fields for automation

### `oci-image-manifest.json`

Records the OCI image repository, tags, published digest, and platforms. This is the source that `release-manifest.json` imports for image metadata.

### `chart-metadata.json`

Records the chart package path, chart version, app version, package digest, and published OCI reference when available.

### `contracts-manifest.json`

Records the public schema contract names, versions, canonical URIs, and digests generated from repo sources.

### `release-checksums.txt`

Stable SHA-256 manifest for the generated release files in `dist/`.

## Consumer Guidance

Automation should:

1. Download `release-manifest.json`.
2. Validate it against `release-manifest.schema.json`.
3. Resolve desired binaries, chart package, image references, or schema contracts from the manifest.
4. Use `release-checksums.txt` to verify the file assets.
5. Use the OCI digest fields for immutable runtime deployment references.
