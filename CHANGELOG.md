# Changelog

## v0.1.0

First public product release of Bering.

### Highlights

- Batch discovery from normalized trace input, OTEL trace input, and explicit `topology_api` inputs.
- Artifact validation against pinned public JSON schemas.
- Runtime service with OTLP/HTTP ingest, optional OTLP/gRPC ingest, health endpoints, Prometheus metrics, and rolling snapshot emission.
- Discovery overlays for additive metadata such as labels, weights, predicate references, and replica overrides.
- Example artifacts, runtime config, collector wiring, Prometheus scrape config, and Grafana dashboard checked into the repository.

### Product vs schema versioning

- Product release tag: `v0.1.0`
- Public schema contracts emitted by this release remain:
  - `io.mb3r.bering.model@1.0.0`
  - `io.mb3r.bering.snapshot@1.0.0`
- Schema publishing to GitHub Pages remains a separate operation under `schema-v1.0.0`

### Packaging and install surface

- Reproducible release archives via GoReleaser for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`.
- Release payload includes checksums, SBOMs, contracts pack, OCI image metadata, Helm chart metadata, and `release-manifest.json`.
- Explicit install path documented for release binaries, source builds, OCI image usage, Helm usage, and offline contracts pack consumption.

### Sheaft integration path

- The primary downstream handoff remains the stable model artifact `io.mb3r.bering.model@1.0.0`.
- Checked-in examples and README commands show the expected handoff into Sheaft.

### Known limits

- Bering stops at discovery and publishing; it does not perform simulation, gating, or chaos orchestration.
- Runtime mode intentionally keeps memory bounded to a single active tumbling window plus the previous emitted snapshot.
- Late spans and in-memory caps are operational tradeoffs surfaced through metrics and logs, not hidden retries or buffering guarantees.
