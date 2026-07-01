# Migration Notes

## Batch users

Existing file-based workflows continue to work, but the default emitted contract line has advanced.

- `bering discover` now writes `io.mb3r.bering.model@1.2.0` by default
- `bering validate` validates `1.0.0`, `1.1.0`, and `1.2.0` model and snapshot artifacts
- deterministic output for identical inputs and flags is preserved

If you are pinned to `1.0.0`, keep consuming the already-published `1.0.0` artifacts you have. Bering does not mutate those contracts in place.

## New optional capabilities

You can now opt into additional discovery-side features.

- `--overlay` for generic discovery metadata overlays
- `--snapshot-out` for a snapshot envelope in batch mode
- `bering discover` can now ingest explicit `topology_api` YAML/JSON files in addition to trace inputs
- `bering serve` for OTLP/HTTP runtime discovery
- optional OTLP/gRPC ingest on a separate runtime listener

## Sheaft and other downstream model consumers

The stable model contract now carries additional typed topology metadata:

- deterministic operation-aware edge ids
- service placements and shared resource references
- service, placement, and edge reliability evidence
- edge resilience policy
- observed edge latency summaries when trace timing exists
- policy scope metadata
- endpoint `method` and `path`
- endpoint `metadata.semantics` hints for immediate, eventual, or external-predicate intent

Downstream model readers still do not need to understand the snapshot envelope unless they want runtime drift, support, or provenance details.

Recommended migration path for downstream consumers:

1. keep consuming `io.mb3r.bering.model@1.0.0` unchanged if you are pinned there
2. move to `io.mb3r.bering.model@1.2.0` when you need the typed placement, reliability, operation-aware edge identity, resilience, observed-edge, and endpoint-fidelity fields
3. add optional snapshot-envelope parsing only when runtime observability features are needed
4. treat overlay-derived metadata as published discovery context, not execution policy

For Sheaft `v1.0.0`, the canonical compatibility checkpoint is [examples/compatibility/sheaft-v1](../examples/compatibility/sheaft-v1/README.md). The same files are included in the contracts pack under `fixtures/sheaft-v1/` so release dry-runs validate both schema sync and fixture packaging.

Edge IDs remain backward-compatible when an edge has no logical identity metadata. When Bering has protocol, operation, route, topic, or span-kind evidence, the ID adds stable discriminator segments. Existing overlays that use the old base ID still work only when the base ID maps to one edge; if multiple operation-aware edges share the same service pair and kind, update the overlay to use the full ID or an explicit `identity`.

## Success predicate references

Bering does not infer brittle success predicates from traces alone.

The stable model still carries `success_predicate_ref` for compatibility. Without an overlay, this remains a stable self-reference rather than a claim that Bering learned a complete success rule from traces.

If you have an external endpoint predicate catalog, provide its stable reference through `predicate_ref` / `success_predicate_ref` and, when known, add `metadata.semantics.predicate_mode`, `mandatory_targets`, and `dependency_modes` through `topology_api` or overlays. These are producer hints for downstream tools such as Sheaft, not Bering-side availability evaluation.
