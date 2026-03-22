# Bering

`Bering v0.3.0` is the current product release of Bering: a discovery and publishing layer for service topology and endpoint contracts.

Bering turns trace input or explicit topology input into stable JSON artifacts, and it can also run as a long-lived runtime service that emits rolling discovery snapshots plus conservative reconciliation views for trace gaps.

## What Bering Does

- deterministic batch discovery from trace files, trace directories, or explicit `topology_api` inputs
- validation of Bering JSON artifacts against pinned public schemas
- runtime OTLP ingest over HTTP, with optional OTLP/gRPC ingest
- publishing of stable topology and snapshot artifacts for downstream tooling
- discovery-side enrichment through additive overlays

## Where Bering Stops

Bering owns discovery and discovery-side public contracts. It does not own simulation math, retry-amplification math, timeout-wave propagation, blast-radius scoring, policy evaluation, gating, or chaos execution.

## Product Version vs Schema Version

Do not treat the product release tag as the schema contract version.

- Current product release for this repository: `Bering v0.3.0`
- Current product git tag / GitHub Release: `v0.3.0`
- Default emitted schema contracts now are:
  - `io.mb3r.bering.model@1.1.0`
  - `io.mb3r.bering.snapshot@1.1.0`
- Previously published contracts remain valid and immutable:
  - `io.mb3r.bering.model@1.0.0`
  - `io.mb3r.bering.snapshot@1.0.0`
- Schema publishing tag remains separate: `schema-v1.1.0`

`bering validate` accepts both the preserved `1.0.0` artifacts and the current `1.1.0` artifacts.

## Installation

### Release binaries

Download a release asset from GitHub Releases and verify it with `release-checksums.txt`.

Minimum supported release archives:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`

Also packaged today:

- `windows/amd64`

Example:

```bash
tar -xzf bering_0.3.0_linux_amd64.tar.gz
./bering help
```

### Build from source

```bash
go build ./cmd/bering
```

### OCI image

The release pipeline also publishes an OCI image and OCI Helm chart. See [docs/install.md](docs/install.md) for example commands.

## First Run

### 1) Batch discovery from checked-in traces

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --out out/bering-model.json \
  --discovered-at 2026-03-03T00:00:00Z
```

Expected result: a deterministic `io.mb3r.bering.model@1.1.0` artifact.

### 2) Validate an artifact

```bash
go run ./cmd/bering validate \
  --input examples/outputs/bering-model.normalized.sample.json
```

`bering validate` accepts both the model artifact and the snapshot envelope.

### 3) Batch discovery with snapshot output and overlay

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --out out/bering-model.json \
  --snapshot-out out/bering-snapshot.json \
  --overlay configs/discovery.overlay.sample.yaml \
  --discovered-at 2026-03-03T00:00:00Z
```

Expected result: a model artifact plus an `io.mb3r.bering.snapshot@1.1.0` envelope.

### 4) Run runtime mode

```bash
go run ./cmd/bering serve --config configs/serve.sample.yaml
```

The sample runtime config listens on:

- HTTP: `:4318`
- OTLP/gRPC: `:4317`

Useful endpoints:

- `POST /v1/traces`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

### 5) Use the result with Sheaft

The main downstream handoff is the stable model artifact:

```bash
# from a sibling Sheaft repository
go run ./cmd/sheaft run \
  --model ../Bering/examples/outputs/bering-model.normalized.sample.json \
  --policy configs/gate.policy.example.yaml \
  --out-dir out \
  --seed 42
```

## Published Artifacts

### Core model

- `name`: `io.mb3r.bering.model`
- `version`: `1.1.0`
- `uri`: `https://mb3r-lab.github.io/Bering/schema/model/v1.1.0/model.schema.json`
- `digest`: `sha256:bc9a60736c9e6bda9599243fd68f293b88f42ade65321d8267369a5c3214779a`

This is the stable topology artifact intended for file-based consumers and tools such as Sheaft. It now carries first-class edge ids, typed service placement and shared-fate metadata, typed edge resilience policy, optional observed edge timing summaries, policy scope metadata, and richer endpoint fidelity.

### Snapshot envelope

- `name`: `io.mb3r.bering.snapshot`
- `version`: `1.1.0`
- `uri`: `https://mb3r-lab.github.io/Bering/schema/snapshot/v1.1.0/snapshot.schema.json`
- `digest`: `sha256:53b127608b2aaa4fabb352b998cd6b2c5ed558764729a09abea56f4f9b40fa01`

This wraps the model with runtime window metadata, ingest counts, coverage, provenance, topology diffs, and discovery-side copies of the same typed placement, resilience, and observed edge metadata.

### Runtime reconciliation views

Runtime mode also keeps a conservative reconciliation layer for sparse traffic and telemetry gaps:

- `raw_window`: the current window as observed
- `stable_core`: the high-confidence retained view
- `guardrail_union`: the conservative default downstream artifact
- a machine-readable reconciliation report for operators

These runtime views do not change the public `io.mb3r.bering.model@1.1.0` or `io.mb3r.bering.snapshot@1.1.0` contracts.

### Population Today

- `topology_api` input and discovery overlays are the first-class write surfaces for typed `placements`, `shared_resource_refs`, `resilience`, `observed`, `policy_scope`, and richer endpoint metadata.
- Trace discovery can derive stable edge ids, endpoint `method` and `path`, and `observed.latency_ms.*` summaries when span timing exists.
- Generic trace ingestion does not currently infer timeout policies, retry policies, circuit-breaker settings, placement groups, or shared resource references. Those remain out of scope for Bering trace inference today.

## Release Packaging

Public release packaging is reproducible through GoReleaser plus GitHub Actions:

- [`.goreleaser.yaml`](.goreleaser.yaml) builds release archives for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`
- [`.github/workflows/release-dry-run.yml`](.github/workflows/release-dry-run.yml) validates the release path without publishing
- [`.github/workflows/release.yml`](.github/workflows/release.yml) builds the same payload from `vX.Y.Z` tags and publishes release assets plus OCI artifacts

Release outputs include:

- binary archives
- archive SBOMs
- archive checksums
- contracts pack
- Helm chart package metadata
- OCI image metadata
- `release-manifest.json`
- `release-notes.md`

## Examples

- Batch inputs: [examples/traces/normalized.sample.json](examples/traces/normalized.sample.json), [examples/traces/otel.sample.json](examples/traces/otel.sample.json)
- Topology input: [examples/topology/topology-api.sample.yaml](examples/topology/topology-api.sample.yaml)
- Batch outputs: [examples/outputs/bering-model.normalized.sample.json](examples/outputs/bering-model.normalized.sample.json), [examples/outputs/bering-snapshot.normalized.sample.json](examples/outputs/bering-snapshot.normalized.sample.json), [examples/outputs/bering-model.topology-api.sample.json](examples/outputs/bering-model.topology-api.sample.json), [examples/outputs/bering-snapshot.topology-api.sample.json](examples/outputs/bering-snapshot.topology-api.sample.json)
- Runtime config: [configs/serve.sample.yaml](configs/serve.sample.yaml)
- Discovery overlay: [configs/discovery.overlay.sample.yaml](configs/discovery.overlay.sample.yaml)
- Collector sidecar: [examples/collector/otelcol.sidecar.yaml](examples/collector/otelcol.sidecar.yaml)
- Prometheus scrape config: [examples/prometheus/bering.prometheus.yml](examples/prometheus/bering.prometheus.yml)
- Grafana dashboard: [examples/grafana/bering-runtime-dashboard.json](examples/grafana/bering-runtime-dashboard.json)

## Additional Docs

- [docs/install.md](docs/install.md)
- [docs/runtime-config.md](docs/runtime-config.md)
- [docs/adr/2026-03-21-evidence-weighted-reconciliation.md](docs/adr/2026-03-21-evidence-weighted-reconciliation.md)
- [docs/release-assets.md](docs/release-assets.md)
- [docs/schema-publishing.md](docs/schema-publishing.md)
- [docs/trace-input-format.md](docs/trace-input-format.md)
- [docs/topology-input-format.md](docs/topology-input-format.md)
- [docs/mvp-scope-and-limits.md](docs/mvp-scope-and-limits.md)
- [docs/architecture.md](docs/architecture.md)
- [RELEASING.md](RELEASING.md)
- [VERSIONING.md](VERSIONING.md)
- [CHANGELOG.md](CHANGELOG.md)

## CI and Local Checks

```bash
go test ./...
go build ./cmd/bering
```

The full release path is documented in [RELEASING.md](RELEASING.md).

## License

MIT (see [LICENSE](LICENSE)).
