# Milestone: Minor: Versioned Bering Schema Contract Evolution

## Goal

Publish additive `1.1.0` public contract evolution for the stable Bering model and snapshot envelope so Bering can carry the topology, resilience, placement, and observed timing metadata that belong in its own public contract as first-class typed fields.

## What This Minor Covers

- preserve `io.mb3r.bering.model@1.0.0` and `io.mb3r.bering.snapshot@1.0.0` unchanged
- add `io.mb3r.bering.model@1.1.0` and `io.mb3r.bering.snapshot@1.1.0`
- switch default emission to `1.1.0` while keeping validation for preserved `1.0.0` artifacts
- add deterministic stable edge `id` emission in the model contract
- add typed service placement metadata, shared fate-domain references, and preserved `failure_eligible`
- add typed edge resilience, observed timing/error summaries, and policy scope metadata
- enrich stable model endpoints with `method`, `path`, and endpoint metadata needed for future policy scoping
- wire every new typed field through `topology_api` input and discovery overlays
- derive observed latency summaries from trace timings when Bering has enough evidence
- keep schema publishing, manifests, digests, examples, docs, and release packaging aligned

## Working Rules

- never mutate already-published `1.0.0` contracts in place
- express new machine-readable semantics as typed public fields, not generic string bags
- keep Bering focused on discovery and publishing rather than Sheaft simulation or scoring logic
- prefer additive contract evolution unless a concrete breaking reason forces a major bump
- keep release tags and schema publication tags separate
