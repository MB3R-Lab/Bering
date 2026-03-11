# Bering

Bering is a discovery and publishing layer for service topology and endpoint contracts.

It supports two operating modes:

- deterministic batch discovery from trace files and directories
- long-running runtime discovery that accepts OTLP/HTTP spans and publishes rolling snapshot envelopes for observability consumers

Bering owns discovery and discovery-side public contracts. It does not own simulation, gating, chaos execution, or policy decisions.

## Public Artifacts

Bering currently publishes two versioned JSON artifact types.

### Core model

- `name`: `io.mb3r.bering.model`
- `version`: `1.0.0`
- `uri`: `https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json`
- `digest`: `sha256:272277c093f37580adcd2dded225bd37c86539d642d7910baad7e4228227d1a7`

This remains the simple stable topology artifact for file-based users and downstream tools such as Sheaft.

### Snapshot envelope

- `name`: `io.mb3r.bering.snapshot`
- `version`: `1.0.0`
- `uri`: `https://mb3r-lab.github.io/Bering/schema/snapshot/v1.0.0/snapshot.schema.json`
- `digest`: `sha256:87e4e887ed4a37b72f6136e268b73552eccb92941c4de2c6f3a514dd066ea972`

This wraps the core model with runtime window metadata, ingest counts, support summaries, provenance, and topology diffs.

`bering validate` accepts either artifact type.

## Repository layout

```text
cmd/bering                    CLI entrypoint
internal/app                  command wiring
internal/config               serve-mode config parsing and validation
internal/connectors/traces    file/dir trace loading and normalization
internal/connectors/otlp      OTLP/HTTP request decoding into normalized spans
internal/discovery            source-agnostic discovery engine and overlay application
internal/model                stable core model structs, semantic checks, canonical IO
internal/overlay              generic discovery overlay loader
internal/runtime              long-running service, tumbling windows, sinks, metrics
internal/schema               pinned contract constants + JSON Schema validation
internal/snapshot             snapshot envelope structs, diffing, canonical IO
api/schema                    public schemas published via GitHub Pages
configs                       sample serve and overlay configs
examples                      traces, outputs, collector/prometheus/grafana examples
docs                          architecture, contract, config, migration, limits
scripts/ci                    CI helper scripts
```

## Commands

```bash
bering discover --input <trace-file|dir> [--out bering-model.json] [--snapshot-out bering-snapshot.json] [--replicas replicas.yaml|json] [--overlay overlay.yaml] [--discovered-at RFC3339]
bering validate --input <bering-model.json|bering-snapshot.json>
bering serve --config configs/serve.sample.yaml [--listen :4318] [--window-size 30s] [--flush-interval 5s]
```

## Quickstart

### 1) Batch discovery from traces

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --out examples/outputs/bering-model.normalized.sample.json \
  --discovered-at 2026-03-03T00:00:00Z
```

### 2) Validate the model artifact

```bash
go run ./cmd/bering validate \
  --input examples/outputs/bering-model.normalized.sample.json
```

### 3) Generate a snapshot envelope in batch mode

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --out out/bering-model.json \
  --snapshot-out examples/outputs/bering-snapshot.normalized.sample.json \
  --overlay configs/discovery.overlay.sample.yaml \
  --discovered-at 2026-03-03T00:00:00Z
```

### 4) Run the runtime service

```bash
go run ./cmd/bering serve --config configs/serve.sample.yaml
```

The runtime service exposes:

- `POST /v1/traces` for OTLP/HTTP trace ingest
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

The primary integration path is standard OpenTelemetry Collector or SDK exporters sending spans to Bering over OTLP/HTTP. No custom Collector build is required.

### 5) Use the stable model with Sheaft

```bash
# from a sibling Sheaft repository
go run ./cmd/sheaft run \
  --model ../Bering/examples/outputs/bering-model.normalized.sample.json \
  --policy configs/gate.policy.example.yaml \
  --out-dir out \
  --seed 42
```

## Examples

- Batch inputs: [examples/traces/normalized.sample.json](examples/traces/normalized.sample.json), [examples/traces/otel.sample.json](examples/traces/otel.sample.json)
- Batch outputs: [examples/outputs/bering-model.normalized.sample.json](examples/outputs/bering-model.normalized.sample.json), [examples/outputs/bering-snapshot.normalized.sample.json](examples/outputs/bering-snapshot.normalized.sample.json)
- Runtime config: [configs/serve.sample.yaml](configs/serve.sample.yaml)
- Discovery overlay: [configs/discovery.overlay.sample.yaml](configs/discovery.overlay.sample.yaml)
- Collector sidecar: [examples/collector/otelcol.sidecar.yaml](examples/collector/otelcol.sidecar.yaml)
- Prometheus scrape config: [examples/prometheus/bering.prometheus.yml](examples/prometheus/bering.prometheus.yml)
- Grafana dashboard: [examples/grafana/bering-runtime-dashboard.json](examples/grafana/bering-runtime-dashboard.json)

## Determinism and Runtime Tradeoffs

Batch output remains deterministic for identical inputs and flags:

- services sorted by `id`
- edges sorted by `(from,to,kind,blocking)`
- endpoints sorted by `id`
- canonical JSON output with stable object-key ordering
- optional `--discovered-at` for reproducible timestamps

Runtime mode is intentionally bounded, not lossless:

- one active tumbling window is retained in memory
- the previous emitted snapshot is retained for diffs and carry-forward runtime timestamps
- `runtime.max_in_memory_spans` bounds retained spans per active window
- late spans follow `drop` or `current_window` policy
- spans beyond the configured in-memory cap are dropped and surfaced via metrics/logs
- empty windows are advanced without emitting empty snapshots

## Discovery overlays

Discovery overlays are additive metadata inputs with explicit precedence by file order. They are intended for discovery-side enrichment, not policy evaluation.

Supported examples include:

- service labels and failure-eligibility labels
- endpoint predicate references
- workload or endpoint weights
- SLO references or tags
- replica overrides

See [configs/discovery.overlay.sample.yaml](configs/discovery.overlay.sample.yaml).

## Metrics

The runtime service exports Prometheus/OpenMetrics metrics including:

- `spans_ingested_total`
- `spans_dropped_total`
- `snapshots_emitted_total`
- `snapshot_build_duration_seconds`
- `current_services`
- `current_edges`
- `current_endpoints`
- `window_lag_seconds`
- `last_snapshot_unixtime`
- `snapshot_age_seconds`
- `diff_added_*`
- `diff_removed_*`
- `diff_changed_*`

## Additional docs

- [docs/architecture.md](docs/architecture.md)
- [docs/runtime-config.md](docs/runtime-config.md)
- [docs/trace-input-format.md](docs/trace-input-format.md)
- [docs/schema-publishing.md](docs/schema-publishing.md)
- [docs/migration-notes.md](docs/migration-notes.md)
- [docs/mvp-scope-and-limits.md](docs/mvp-scope-and-limits.md)

## CI and local checks

```bash
make lint
make test
make build
```

## License

MIT (see [LICENSE](LICENSE)).
