# Releasing

Bering release automation is tag-driven and platform-neutral.

The canonical source of truth is the local `dist/` payload plus `dist/release-manifest.json`. GitHub Releases, OCI registries, GitLab CI, and Jenkins are publishers around that payload, not alternate metadata authorities.

Product releases and schema publishing are separate operations:

- product release tags: `vX.Y.Z`
- schema publishing tags: `schema-vA.B.C`

Product releases advance independently from the public schema contracts. As of the current release line, use `v0.3.0` for the product release while the default emitted public schema contracts are `1.1.0` and the preserved `1.0.0` contracts remain available for validation and pinned consumers.

## Prerequisites

Local release entrypoints expect these tools on `PATH`:

- Go
- Docker with `buildx`
- Helm 3 with OCI support
- GoReleaser
- Syft
- GNU Make

## Canonical Local Entrypoints

Dry-run without publishing:

```bash
make release-dry-run \
  VERSION=0.3.0 \
  GIT_SHA="$(git rev-parse HEAD)" \
  BUILD_DATE="$(git show -s --format=%cI HEAD)"
```

Full local release payload with OCI publish:

```bash
make release-local \
  VERSION=0.3.0 \
  GIT_SHA="$(git rev-parse HEAD)" \
  BUILD_DATE="$(git show -s --format=%cI HEAD)" \
  IMAGE_REPOSITORY=ghcr.io/mb3r-lab/bering \
  CHART_OCI_REPOSITORY=oci://ghcr.io/mb3r-lab/charts \
  PUBLISH_OCI=1
```

If the chart version must intentionally differ from the app version:

```bash
make chart-package \
  VERSION=0.3.0 \
  CHART_VERSION=0.3.1 \
  ALLOW_CHART_VERSION_MISMATCH=1
```

## Release Flow

1. Run `make release-dry-run`.
2. Create and push a SemVer product tag: `vX.Y.Z`.
3. CI runs tests and produces the same `dist/` payload via `make release-local`.
4. CI publishes:
   - GitHub Release assets from `dist/release-assets.txt`
   - OCI image tags
   - OCI Helm chart
5. Consumers use `release-manifest.json` to discover the released payload.

Schema contract publication to GitHub Pages is handled separately by `schema-v*` tags and [`.github/workflows/publish-schema.yml`](.github/workflows/publish-schema.yml).

## Environment

These variables control the release payload:

- `VERSION`: app SemVer without the leading `v`
- `GIT_SHA`: full git commit SHA
- `BUILD_DATE`: RFC3339 timestamp, ideally the commit timestamp
- `IMAGE_REPOSITORY`: OCI image repository
- `CHART_OCI_REPOSITORY`: OCI chart repository prefix
- `PUBLISH_OCI=1`: publish OCI image and chart instead of creating local-only metadata
- `CHART_VERSION`: optional explicit chart version
- `ALLOW_CHART_VERSION_MISMATCH=1`: required when `CHART_VERSION != VERSION`

For `v0.3.0`, `VERSION=0.3.0` while the default emitted public schema contracts are `1.1.0`.

## Expected Output

The release pipeline writes stable, automation-friendly files to `dist/`, including:

- cross-platform archives
- per-archive SBOM files
- GoReleaser checksums
- contracts pack and contracts manifest
- chart package and chart metadata
- OCI image metadata
- `release-checksums.txt`
- `release-manifest.json`
- `release-manifest.schema.json`
- `release-notes.md`

## GitHub

GitHub Actions is one publisher only:

- [`.github/workflows/release-dry-run.yml`](.github/workflows/release-dry-run.yml) validates the release path without publishing.
- [`.github/workflows/release.yml`](.github/workflows/release.yml) builds the payload from a tag and publishes GitHub Release assets plus OCI artifacts.

GitLab CI and Jenkins should call the same `make` targets instead of reimplementing the release logic.
