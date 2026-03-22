# Implementation Note: Versioned Bering Schema Contract Evolution

## Baseline Before This Minor

- Bering published `io.mb3r.bering.model@1.0.0` and `io.mb3r.bering.snapshot@1.0.0`.
- The stable model contract lacked a first-class edge `id`.
- Placement, shared fate-domain, resilience, policy-scope, and observed timing metadata were not available as typed public fields across the stable handoff surface.
- The repo carried schema digests and release metadata, but the public schema source tree was still effectively a single current line rather than an explicit versioned contract history.

## Implemented Contract Evolution

- Added `io.mb3r.bering.model@1.1.0`.
- Added `io.mb3r.bering.snapshot@1.1.0`.
- Preserved `1.0.0` contracts and validators without in-place mutation.
- Switched default emitted contracts to `1.1.0`.
- Added deterministic model edge ids using `from|to|kind|blocking` when no explicit id is supplied.
- Added typed service metadata for placements, failure-domain labels, shared resource references, and `failure_eligible`.
- Added typed edge metadata for resilience policy, observed latency/error summaries, and optional policy scope.
- Added stable endpoint `method`, `path`, and endpoint metadata fields needed for future edge-policy granularity.

## Input And Enrichment Surfaces

- `topology_api` input is now a first-class source for every added typed field.
- Discovery overlays are also a first-class source for every added typed field.
- Trace ingestion now derives observed latency summaries when timestamps exist and also improves endpoint fidelity and deterministic edge identity.
- Generic trace ingestion still does not infer timeout policies, retry policies, circuit-breaker settings, placement groups, or shared resource references. Those remain explicit topology or overlay inputs.

## Release And Publication Boundary

- Product release versioning remains separate from schema contract versioning.
- The current product line is `v0.3.0`, and it now emits `1.1.0` contracts by default.
- The schema publication tag required for external publication is `schema-v1.1.0`.
- Local release plumbing is complete and validated, but external publication still depends on creating and pushing that schema tag.

## Explicit Non-Goals For This Minor

- no Sheaft-local fault DSL
- no retry amplification math
- no timeout-wave propagation logic
- no blast-radius scoring
- no CI gating semantics
- no chaos execution

This minor stops at publishing the typed, versioned data needed so those downstream capabilities are not blocked later.
