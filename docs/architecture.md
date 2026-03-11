# Architecture

## Overview

Bering now has two user-facing flows built on one normalized discovery core.

### Batch flow

1. `bering discover` loads trace JSON files or directories.
2. File inputs are normalized into the internal `traces.Span` shape.
3. The discovery engine infers services, edges, endpoints, confidence, and overlay-driven metadata.
4. Bering writes the stable `io.mb3r.bering.model` artifact.
5. Optional `--snapshot-out` also writes a `io.mb3r.bering.snapshot` envelope.

### Runtime flow

1. `bering serve` accepts OTLP/HTTP `POST /v1/traces` requests.
2. OTLP requests are normalized into the same internal `traces.Span` shape used by batch mode.
3. Spans are accumulated in a bounded active tumbling window.
4. On schedule, the window closes and Bering runs discovery.
5. Bering computes a stable topology digest, diffs against the previous snapshot, and writes the snapshot to sinks.
6. Metrics and health endpoints expose runtime operability.

## Internal interfaces and seams

The runtime additions are intentionally interface-driven so future connectors can slot in without rewriting the discovery engine.

- source adapters: normalize external data into internal spans or overlays
- overlay loaders: decode reusable discovery-side metadata files
- snapshot sinks: write snapshot envelopes to one or more destinations
- clocks: make windowing deterministic in tests
- storage boundary: one active in-memory tumbling window with an explicit cap

## Normalized span model

`internal/connectors/traces.Span` remains the small shared runtime shape.

Fields used today:

- trace identity: `trace_id`, `span_id`, `parent_span_id`
- service identity: `service`
- operation naming: `name`, `kind`
- timestamps: `start_time`, `end_time`
- discovery-relevant attributes: HTTP and messaging attributes

This keeps discovery logic source-agnostic. File JSON, OTLP/HTTP, and future adapters all converge on the same shape.

## Contracts

### Stable model contract

`io.mb3r.bering.model` v1.0.0 remains the simple downstream-facing topology contract.

It is still the default output for file-based users and remains deterministic for identical inputs.

### Snapshot contract

`io.mb3r.bering.snapshot` v1.0.0 is additive. It wraps the stable model with:

- `snapshot_id`
- `topology_version`
- window boundaries
- ingest counts
- support summaries
- provenance
- overlay application metadata
- diff summary

This lets runtime consumers observe topology drift without forcing existing model readers to migrate immediately.

## Overlay precedence

Overlay precedence is explicit and deterministic.

1. inferred runtime/batch discovery
2. legacy replica override file when used
3. overlay files in the order they are provided
4. command flags override config fields, not overlay content

Overlays enrich discovery artifacts. They do not introduce gating, pass/fail semantics, or execution logic.

## Async/sync edge classification

Edge kind remains heuristic and extensible.

Current signals:

- span kind `producer` or `consumer`
- messaging attributes such as `messaging.system`, `messaging.destination`, `messaging.operation`
- otherwise default to `sync`

The snapshot contract records edge support evidence so downstream consumers can reason about heuristic classification without Bering pretending it observed a hard truth.

## Memory bounds and late spans

Runtime mode intentionally trades completeness for bounded memory.

- only the active tumbling window is retained
- only the previous emitted snapshot is retained for diffs and carry-forward `first_seen` / `last_seen`
- `runtime.max_in_memory_spans` bounds retained spans in that window
- excess spans are dropped and surfaced in metrics/logs
- late spans follow one of two policies:
  - `drop`
  - `current_window`

This keeps Bering operational as a discovery/publishing layer rather than a stateful analysis engine.
