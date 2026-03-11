# Scope and Limits

## In scope

- deterministic batch discovery from trace files/directories
- deterministic batch discovery from explicit `topology_api` files
- long-running runtime service that accepts OTLP/HTTP spans, with optional OTLP/gRPC ingest
- stable core model artifacts (`io.mb3r.bering.model`)
- snapshot envelopes for observability/runtime consumers (`io.mb3r.bering.snapshot`)
- generic discovery overlays for metadata and predicate references
- Prometheus/OpenMetrics runtime metrics and health endpoints

## Explicitly out of scope

- simulation execution
- gating or approval policy decisions
- chaos orchestration
- benchmark-specific rules or benchmark-specific naming
- claiming trace-only success semantics that were not supplied externally

## Confidence heuristic

`metadata.confidence` remains deterministic and informational.

- base score: `0.35`
- `+0.20` if more than one service discovered
- `+0.15` if at least one edge discovered
- `+0.15` if at least one endpoint inferred
- `+min(0.15, cross_service_parent_child_ratio * 0.15)`

Result is clamped to `[0,1]` and rounded to two decimals.

This score is not a calibrated probability and must not be treated as a gate.

## Runtime memory and lateness limits

Runtime mode intentionally keeps memory bounded.

- one active tumbling window is retained
- retained spans are capped by config
- buffer overflow drops spans and reports them in metrics/logs
- late spans follow `drop` or `current_window` policy
- empty windows are advanced without emitting empty artifacts

These are operational tradeoffs, not bugs. They keep Bering in the role of a discovery/publishing layer.
