# Migration Notes

## Batch users

Existing file-based workflows continue to work.

- `bering discover` still writes the same `io.mb3r.bering.model` v1.0.0 artifact by default
- `bering validate` still validates the stable model contract
- deterministic output for identical inputs and flags is preserved

If you do nothing, your existing batch model flow should remain unchanged.

## New optional capabilities

You can now opt into additional discovery-side features.

- `--overlay` for generic discovery metadata overlays
- `--snapshot-out` for a snapshot envelope in batch mode
- `bering discover` can now ingest explicit `topology_api` YAML/JSON files in addition to trace inputs
- `bering serve` for OTLP/HTTP runtime discovery
- optional OTLP/gRPC ingest on a separate runtime listener

## Sheaft and other downstream model consumers

The stable model contract is unchanged.

Downstream model readers do not need to understand the snapshot envelope unless they want runtime drift, support, or provenance details.

Recommended migration path for downstream consumers:

1. keep consuming `io.mb3r.bering.model` v1.0.0 unchanged
2. add optional snapshot-envelope parsing only when runtime observability features are needed
3. treat overlay-derived metadata as advisory discovery context, not execution policy

## Success predicate references

Bering does not infer brittle success predicates from traces alone.

The stable model still carries `success_predicate_ref` for compatibility. Without an overlay, this remains a stable self-reference rather than a claim that Bering learned a complete success rule from traces.

If you have an external endpoint predicate catalog, provide it through overlays.
