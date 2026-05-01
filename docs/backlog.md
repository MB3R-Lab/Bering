# Product Capability Backlog

This document is the repository-side product backlog snapshot for `MB3R-Lab/Bering`.

Source of truth for tracked implementation work: GitHub Issues, anchored by the pinned delivery index at https://github.com/MB3R-Lab/Bering/issues/19.

Snapshot date: `2026-05-01`

## Current Product Surface

Bering is the discovery and publication layer for the MB3R toolchain. The current `v0.3.2` release provides:

- deterministic batch discovery from normalized trace files, OTLP JSON, trace directories, and explicit `topology_api` inputs;
- runtime OTLP/HTTP and OTLP/gRPC ingest;
- stable `io.mb3r.bering.model` and `io.mb3r.bering.snapshot` artifacts on the `1.1.0` schema line, while preserving `1.0.0` validation;
- typed topology metadata for edge ids, placements, shared resources, resilience policy, observed timing, policy scope, and endpoint fidelity when supplied or derivable;
- discovery overlays as the explicit enrichment surface for metadata that generic traces cannot infer;
- evidence-weighted runtime reconciliation for trace gaps, with bounded memory and conservative downstream publication;
- release assets, contracts pack, OCI image metadata, and OCI Helm chart metadata.

## Product Capability Gaps

These gaps are tracked as concrete GitHub issues.

Tracked tasks:

- [#39](https://github.com/MB3R-Lab/Bering/issues/39): Emit signal quality and coverage report for every artifact.
- [#40](https://github.com/MB3R-Lab/Bering/issues/40): Add enrichment onboarding examples for trace-only to topology_api/overlay upgrades.
- [#41](https://github.com/MB3R-Lab/Bering/issues/41): Publish operator-facing runtime reconciliation summary.
- [#42](https://github.com/MB3R-Lab/Bering/issues/42): Add reference non-trace adapter example built on topology_api.

### Signal Quality And Coverage

Tracker: [#39](https://github.com/MB3R-Lab/Bering/issues/39)

Generic trace input does not infer timeout policies, retry policies, circuit-breaker limits, placement groups, or shared-resource references. Downstream tools such as Sheaft can only produce advanced diagnostics when those fields are present.

Backlog outcome:

- emit a machine-readable signal-quality summary for every model/snapshot;
- report which advanced fields are present, missing, inferred, or externally supplied;
- expose clear coverage percentages for services, edges, endpoints, placements, resilience policy, and observed timings;
- make low-confidence trace-only artifacts obvious before they become gate inputs.

### Enrichment Onboarding

Tracker: [#40](https://github.com/MB3R-Lab/Bering/issues/40)

`topology_api` and overlays are first-class enrichment inputs, but the user path from trace-only discovery to high-quality topology is still too manual.

Backlog outcome:

- provide a metadata coverage checklist for teams moving from trace-only to enriched artifacts;
- add examples that start with missing retry/timeout/placement data and then close those gaps with overlays;
- document the minimum enrichment set required for Sheaft advanced analysis.

### Runtime Operator Diagnostics

Tracker: [#41](https://github.com/MB3R-Lab/Bering/issues/41)

Runtime reconciliation already avoids deleting topology solely because one window is quiet. Operators still need a compact explanation of what changed, what was retained, and why downstream consumers should trust the latest artifact.

Backlog outcome:

- make reconciliation reports easy to consume from docs and automation;
- summarize retained, stale, and retired entities in a human-readable operator view;
- document how telemetry health affects retention and confidence.

### Source Adapter Strategy

Tracker: [#42](https://github.com/MB3R-Lab/Bering/issues/42)

Bering currently has a strong trace/topology-file path. Product adoption will depend on how easily teams can feed topology from their existing inventory, service catalog, mesh, or platform APIs.

Backlog outcome:

- define a connector strategy around `topology_api` instead of adding ad hoc source-specific semantics;
- publish one reference non-trace source adapter example;
- keep all adapters mapped back to the same stable public model/snapshot contracts.

## Near-Term Priority

1. [#39](https://github.com/MB3R-Lab/Bering/issues/39): Signal quality and coverage report for every emitted artifact.
2. [#40](https://github.com/MB3R-Lab/Bering/issues/40): Enrichment onboarding docs and examples for trace-only to overlay/topology_api upgrades.
3. [#41](https://github.com/MB3R-Lab/Bering/issues/41): Operator-facing reconciliation summary.
4. [#42](https://github.com/MB3R-Lab/Bering/issues/42): Reference non-trace adapter example built around the existing `topology_api` contract.

## Non-Goals

- Bering should not own Sheaft simulation, CI gating, blast-radius scoring, retry-amplification math, or chaos execution.
- Bering should not claim trace-only success semantics that were not supplied externally.
- Bering should not introduce source-specific contracts that bypass the stable model/snapshot surface.
