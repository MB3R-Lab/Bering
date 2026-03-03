# Bering

Bering is a model discovery and contract-producing tool for resilience analysis.
It consumes trace artifacts and produces canonical `bering-model.json` in
`BeringResilienceModel` v1.0.0 format for any downstream consumer.
Sheaft is a reference consumer, not an exclusive target.

## Contract (strict)

Bering pins `metadata.schema` exactly:

- `name`: `io.mb3r.bering.model`
- `version`: `1.0.0`
- `uri`: `https://schemas.mb3r.dev/bering/model/v1.0.0/model.schema.json`
- `digest`: `sha256:7dc733936a9d3f94ab92f46a30d4c8d0f5c05d60670c4247786c59a3fe7630f7`

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
api/schema                    canonical public schema
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

### 3) Optional: run Sheaft on generated model (reference consumer)

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

## CI and local checks

```bash
make lint
make test
make build
```

## License

MIT (see [LICENSE](LICENSE)).
