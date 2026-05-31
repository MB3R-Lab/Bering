# Enrichment Onboarding

Bering treats traces, overlays, and `topology_api` inputs as different discovery evidence surfaces.

## Workflow

1. Start with trace-only discovery.
2. Read the generated `*.signal-quality.json` sidecar.
3. Add an overlay for curated metadata that traces cannot infer.
4. Move to `topology_api` when a non-trace system is authoritative for topology, placement, resilience, or endpoint metadata.

## Trace-only baseline

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --out out/onboarding/trace-only/bering-model.json \
  --snapshot-out out/onboarding/trace-only/bering-snapshot.json \
  --discovered-at 2026-03-03T00:00:00Z
```

The generated signal quality report calls out missing coverage for fields such as `services.placements`, `edges.request_timeout_ms`, `edges.retry_policy`, and `endpoints.custom_success_predicate`.

## Overlay upgrade

```bash
go run ./cmd/bering discover \
  --input examples/traces/normalized.sample.json \
  --overlay examples/enrichment/checkout.overlay.yaml \
  --out out/onboarding/overlay/bering-model.json \
  --snapshot-out out/onboarding/overlay/bering-snapshot.json \
  --discovered-at 2026-03-03T00:00:00Z
```

Use overlays when the trace-derived topology is acceptable but operational metadata comes from configuration, ownership catalogs, SLO catalogs, or curated policy files.

## topology_api upgrade

```bash
go run ./cmd/bering discover \
  --input examples/enrichment/checkout.topology-api.yaml \
  --out out/onboarding/topology-api/bering-model.json \
  --snapshot-out out/onboarding/topology-api/bering-snapshot.json \
  --discovered-at 2026-03-03T00:00:00Z
```

Use `topology_api` when Bering should publish an explicit topology from a non-trace adapter rather than infer the topology from spans.

Related examples live in [examples/enrichment](../examples/enrichment/README.md).
