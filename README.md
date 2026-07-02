# Bering

[![release](https://img.shields.io/badge/release-v1.0.0-blue)](https://github.com/MB3R-Lab/Bering/releases/tag/v1.0.0)
[![checks](https://img.shields.io/github/actions/workflow/status/MB3R-Lab/Bering/ci.yml?branch=main&label=checks)](https://github.com/MB3R-Lab/Bering/actions/workflows/ci.yml)
[![schema](https://img.shields.io/badge/schema-1.3.0-blue)](https://github.com/MB3R-Lab/Bering/blob/main/VERSIONING.md)

## Related MB3R repositories

Bering publishes topology and snapshot artifacts for downstream consumers. For example, [Sheaft](https://github.com/MB3R-Lab/Sheaft) can consume Bering artifacts for resilience analysis and CI/CD gating. [mb3r-stack](https://github.com/MB3R-Lab/mb3r-stack) packages compatible Bering and Sheaft releases with integration assets.

`Bering v1.0.0` is the current product release of Bering: a discovery and publishing layer for service topology and endpoint contracts.

Bering turns trace input or explicit topology input into stable JSON artifacts, and it can also run as a long-lived runtime service that emits rolling discovery snapshots plus conservative reconciliation views for trace gaps.

The trace-discovered topology model and its downstream adequacy boundary follow the stochastic-connectivity research line.

## Research and Evidence

- Formal model: [Stochastic Connectivity as the Foundation of a Runtime Model for Microservice Availability Analysis](https://www.alphaxiv.org/abs/2607.00740)
- DeathStarBench empirical anchor: [Model Discovery and Graph Simulation: A Lightweight Gateway to Chaos Engineering](https://www.alphaxiv.org/abs/2506.11176)
- OpenTelemetry Demo async-semantics case study: [Evaluating Asynchronous Semantics in Trace-Discovered Resilience Models: A Case Study on the OpenTelemetry Demo](https://www.alphaxiv.org/abs/2512.12314v1)

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

- Current product release for this repository: `Bering v1.0.0`
- Current product git tag / GitHub Release: `v1.0.0`
- Default emitted schema contracts now are:
  - `io.mb3r.bering.model@1.3.0`
  - `io.mb3r.bering.snapshot@1.3.0`
- Schema publishing tag remains separate: `schema-v1.3.0`

Breaking v1 cleanup: `bering validate` accepts only current strict `1.3.0` model and snapshot artifacts. Pre-v1 preview schema lines were removed from the current release surface before public adoption.

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
tar -xzf bering_1.0.0_linux_amd64.tar.gz
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

Expected result: a deterministic `io.mb3r.bering.model@1.3.0` artifact.

`discover` also writes `out/bering-model.signal-quality.json` next to the model.

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

Expected result: a model artifact plus an `io.mb3r.bering.snapshot@1.3.0` envelope.

The snapshot gets its own `out/bering-snapshot.signal-quality.json` sidecar.

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
- `GET /reconciliation/report`
- `GET /reconciliation/summary`

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

For Sheaft `v1.0.0` stochastic-connectivity compatibility, use the canonical fixture checkpoint in [examples/compatibility/sheaft-v1](examples/compatibility/sheaft-v1/README.md). It pins Bering `v1.0.0` / schema line `1.3.0` and includes model and snapshot artifacts with reliability evidence, observed edge metadata, and immediate/eventual endpoint semantic hints.

## Published Artifacts

### Core model

- `name`: `io.mb3r.bering.model`
- `version`: `1.3.0`
- `uri`: `https://mb3r-lab.github.io/Bering/schema/model/v1.3.0/model.schema.json`
- `digest`: `sha256:4fa1a34e64703524cfe2289341fcea79986265db08c0220d6c89e38c0ff76bf8`

This is the stable topology artifact intended for file-based consumers and tools such as Sheaft. It now carries first-class operation-aware edge ids, typed service placement and shared-fate metadata, optional service/placement/edge reliability evidence, typed edge resilience policy, optional observed edge timing summaries, policy scope metadata, richer endpoint fidelity, and endpoint semantic hints.

### Snapshot envelope

- `name`: `io.mb3r.bering.snapshot`
- `version`: `1.3.0`
- `uri`: `https://mb3r-lab.github.io/Bering/schema/snapshot/v1.3.0/snapshot.schema.json`
- `digest`: `sha256:cb737b0a4038e0bf30a397ca7ba7ff017d684fe3b25e7d8e3ae74ac59b45210b`

This wraps the model with runtime window metadata, ingest counts, coverage, provenance, topology diffs, and discovery-side copies of the same typed placement, reliability, resilience, observed edge, and endpoint metadata.

### Signal quality report

Every batch model, batch snapshot, and runtime snapshot gets a separate `*.signal-quality.json` sidecar. The sidecar reports source mix, provenance, field coverage for typed enrichment fields, and warnings for low-coverage dimensions without changing the public model or snapshot schemas.

### Runtime reconciliation views

Runtime mode also keeps a conservative reconciliation layer for sparse traffic and telemetry gaps:

- `raw_window`: the current window as observed
- `stable_core`: the high-confidence retained view
- `guardrail_union`: the conservative default downstream artifact
- a machine-readable reconciliation report for operators
- an operator-facing reconciliation summary in Markdown

These runtime views do not change the public `io.mb3r.bering.model@1.3.0` or `io.mb3r.bering.snapshot@1.3.0` contracts.

### Population Today

- `topology_api` input and discovery overlays are the first-class write surfaces for typed `placements`, `shared_resource_refs`, `reliability`, `resilience`, `observed`, `policy_scope`, and richer endpoint metadata, including `metadata.semantics`.
- Trace discovery can derive operation-aware stable edge ids, endpoint `method` and `path`, and `observed.latency_ms.*` summaries when span timing exists.
- Generic trace ingestion does not currently infer timeout policies, retry policies, circuit-breaker settings, placement groups, shared resource references, or endpoint success semantics. Those remain out of scope for Bering trace inference today.

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
- Adapter-generated outputs: [examples/outputs/bering-model.service-catalog-adapter.sample.json](examples/outputs/bering-model.service-catalog-adapter.sample.json), [examples/outputs/bering-snapshot.service-catalog-adapter.sample.json](examples/outputs/bering-snapshot.service-catalog-adapter.sample.json)
- Sheaft v1 compatibility fixtures: [examples/compatibility/sheaft-v1](examples/compatibility/sheaft-v1/README.md)
- Runtime config: [configs/serve.sample.yaml](configs/serve.sample.yaml)
- Discovery overlay: [configs/discovery.overlay.sample.yaml](configs/discovery.overlay.sample.yaml)
- Enrichment onboarding: [examples/enrichment](examples/enrichment/README.md)
- Non-trace adapter: [examples/adapters/service-catalog](examples/adapters/service-catalog/README.md)
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
- [docs/enrichment-onboarding.md](docs/enrichment-onboarding.md)
- [docs/mvp-scope-and-limits.md](docs/mvp-scope-and-limits.md)
- [Delivery tracker](https://github.com/MB3R-Lab/Bering/issues/19)
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
