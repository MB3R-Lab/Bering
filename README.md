# Bering

Bering builds a resilience model artifact from trace artifacts. The output artifact is `bering-model.json` in `BeringResilienceModel` v1.0.0 format. Use it in any downstream tooling or analytics pipeline.

## Contract

Bering pins `metadata.schema` exactly:

- `name`: `io.mb3r.bering.model`
- `version`: `1.0.0`
- `uri`: `https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json`
- `digest`: `sha256:272277c093f37580adcd2dded225bd37c86539d642d7910baad7e4228227d1a7`

Any mismatch fails validation.

## Repository layout

```text
cmd/bering                    CLI entrypoint
internal/app                  command wiring
internal/connectors/traces    trace file/dir loading and normalization
internal/discovery            model inference from normalized spans
internal/model                model structs, semantic checks, canonical IO
internal/schema               pinned contract constants + JSON Schema validation
internal/jsoncanon            deterministic recursive JSON encoder
api/schema                    versioned public schema
configs                       sample configs (replicas override)
examples                      trace fixtures + expected output artifacts
docs                          format, heuristic, and MVP limits
scripts/ci                    CI helper scripts
```

## Commands

```bash
bering discover --input <trace-file|dir> [--out bering-model.json] [--replicas replicas.yaml|json] [--discovered-at RFC3339]
bering validate --input <bering-model.json>
```

## Quickstart

### 1) Discover from traces

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --out examples/outputs/bering-model.normalized.sample.json \
  --discovered-at 2026-03-03T00:00:00Z
```

### 2) Validate artifact

```bash
go run ./cmd/bering validate \
  --input examples/outputs/bering-model.normalized.sample.json
```

### 3) Use the model in any downstream tool

Examples:

- run your own analytics (topology checks, risk scoring, SLO diagnostics)
- feed the model into simulation tooling
- simulate failures with [Sheaft](https://github.com/MB3R-Lab/Sheaft) (one possible consumer), for example:
  ```bash
  # from sibling Sheaft repository
  go run ./cmd/sheaft run \
    --model ../Bering/examples/outputs/bering-model.normalized.sample.json \
    --policy configs/gate.policy.example.yaml \
    --out-dir out \
    --seed 42
  ```

## Deterministic output

Bering output is deterministic for identical inputs and flags:

- services sorted by `id`
- edges sorted by `(from,to,kind,blocking)`
- endpoints sorted by `id`
- stable IDs for services/edges/endpoints
- canonical JSON writer with recursive object-key ordering (future-safe for map fields)
- optional `--discovered-at` for reproducible timestamps

## Supported trace formats (MVP)

- Normalized JSON: `{"spans": [...]}` payload with canonical span fields.
- Raw OTel JSON: `resourceSpans/scopeSpans/spans` payload.

Details: [docs/trace-input-format.md](docs/trace-input-format.md)

## Schema publishing

Schema publishing is automated via GitHub Pages and release tags.

- Workflow: `.github/workflows/publish-schema.yml`
- Trigger: tags matching `schema-v*` (for example `schema-v1.0.0`)
- Published paths:
  - `https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json`
  - `https://mb3r-lab.github.io/Bering/schema/model/latest/model.schema.json`
  - `https://mb3r-lab.github.io/Bering/schema/index.json`

Operational steps are documented in [docs/schema-publishing.md](docs/schema-publishing.md).

## CI and local checks

```bash
make lint
make test
make build
```

## License

MIT (see [LICENSE](LICENSE)).
