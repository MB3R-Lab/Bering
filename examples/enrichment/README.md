# Enrichment Onboarding Examples

These examples show the same checkout flow at three maturity levels:

1. trace-only discovery from `examples/traces/normalized.sample.json`
2. trace discovery upgraded with `checkout.overlay.yaml`
3. explicit non-trace discovery from `checkout.topology-api.yaml`

## 1. Trace-only baseline

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --out out/enrichment/trace-only/bering-model.json \
  --snapshot-out out/enrichment/trace-only/bering-snapshot.json \
  --discovered-at 2026-03-03T00:00:00Z
```

Bering writes:

- `out/enrichment/trace-only/bering-model.json`
- `out/enrichment/trace-only/bering-model.signal-quality.json`
- `out/enrichment/trace-only/bering-snapshot.json`
- `out/enrichment/trace-only/bering-snapshot.signal-quality.json`

Use the signal quality report to see which typed fields are still missing from traces alone.

## 2. Trace plus overlay

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --overlay examples/enrichment/checkout.overlay.yaml \
  --out out/enrichment/overlay/bering-model.json \
  --snapshot-out out/enrichment/overlay/bering-snapshot.json \
  --discovered-at 2026-03-03T00:00:00Z
```

The overlay adds service ownership, failure eligibility, placements, shared resources, edge resilience policy, policy scope, and endpoint predicate references without replacing trace-derived topology.

## 3. Explicit topology_api source

```bash
go run ./cmd/bering discover \
  --input examples/enrichment/checkout.topology-api.yaml \
  --out out/enrichment/topology-api/bering-model.json \
  --snapshot-out out/enrichment/topology-api/bering-snapshot.json \
  --discovered-at 2026-03-03T00:00:00Z
```

Use `topology_api` when a service catalog, mesh export, deployment inventory, or another authoritative non-trace source already owns topology and policy metadata.
