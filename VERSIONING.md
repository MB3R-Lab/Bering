# Versioning

Bering separates product release versioning from public schema contract versioning.

For the current published product release:

- product release tag: `v0.3.1`
- product version in release automation: `0.3.1`
- schema publishing tag: `schema-v1.1.0`
- default emitted schema contracts:
  - `io.mb3r.bering.model@1.1.0`
  - `io.mb3r.bering.snapshot@1.1.0`
- previously published and still valid contracts:
  - `io.mb3r.bering.model@1.0.0`
  - `io.mb3r.bering.snapshot@1.0.0`

Do not infer a schema contract version from the product release tag.

## App Version

- App releases use SemVer: `X.Y.Z`
- Git tags use a leading `v`: `vX.Y.Z`
- `VERSION` in local and CI entrypoints is always the bare SemVer value without the leading `v`

App version changes govern the Bering CLI, the runtime service behavior, the install surface, release automation, and the Helm chart `appVersion`.

Examples:

- product release `v0.3.1` identifies the current Bering release line
- `dist/release-manifest.json` records `"app_version": "0.3.1"`

## Schema Contract Versions

Bering currently owns these public JSON schema contracts:

- `io.mb3r.bering.model`
- `io.mb3r.bering.snapshot`

Each schema contract has its own version, URI, and digest. Those values are independent from the app version and are recorded in:

- [internal/schema/constants.go](internal/schema/constants.go)
- `dist/contracts-manifest.json`
- `dist/release-manifest.json`
- the published artifact payloads themselves in `metadata.schema`

Examples:

- Bering `v0.3.1` emits `io.mb3r.bering.model@1.1.0`
- Bering `v0.3.1` emits `io.mb3r.bering.snapshot@1.1.0`
- `bering validate` still accepts `1.0.0` artifacts without mutation or migration
- `schema-v1.1.0` publishes the schema files to GitHub Pages; it is not a product release tag

Rules:

- Schema contract versions never change in place.
- Published contract URIs and digests are immutable.
- Breaking schema changes require a new contract major version.
- Non-breaking schema additions require a new contract minor version.
- Pure fixes that do not change contract meaning require a new contract patch version.
- This repository currently keeps the model and snapshot contracts on the same schema version line for publication and default emission.

## Image Tags

OCI image tags are release labels, not separate version streams.

Every release publishes:

- `vX.Y.Z`
- `vX.Y`
- `sha-<full-commit-sha>`

Rules:

- `vX.Y.Z` is immutable for a given release payload.
- `vX.Y` is a moving convenience tag for the latest patch in that minor line.
- `sha-<commit>` is immutable and commit-addressable.

## Chart Version

The Helm chart has a distinct chart version because Helm treats chart packaging as its own artifact contract.

Rules:

- `appVersion` always tracks the Bering app version.
- `version` defaults to the app version for normal product releases.
- If chart packaging must change independently, `version` may diverge, but that must be explicit and validated with `ALLOW_CHART_VERSION_MISMATCH=1`.

## Breaking Changes

### CLI, API, and install surface

A breaking change to any of these surfaces requires a new app major version:

- CLI commands, flags, defaults, or output behavior used by automation
- runtime HTTP or OTLP ingest behavior that consumers depend on
- install behavior for binaries, images, chart values, or deployment expectations

### Schema contracts

Breaking changes to public schema contracts are handled by the contract version itself, not silently by the app version alone.

Rules:

- A breaking schema change increments that contract's major version.
- Existing published contract versions remain valid and immutable.
- If Bering changes which schema version it emits by default, call that out explicitly in release notes and keep older published contract versions available to validators and pinned consumers.
