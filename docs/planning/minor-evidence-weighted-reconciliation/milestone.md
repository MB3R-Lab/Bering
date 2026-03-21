# Milestone: Minor: Evidence-Weighted Reconciliation for Trace Gaps

## Goal

Replace naive last-window disappearance semantics in Bering runtime with a persistent, opportunity-aware, evidence-weighted reconciliation module that publishes conservative downstream-compatible artifacts plus richer reconciliation/debug views.

## What This Minor Covers

- current-window discovery remains available as `raw_window`
- high-confidence retained topology is published as `stable_core`
- conservative downstream-compatible publication uses `guardrail_union`
- machine-readable reconciliation/discovery reporting is emitted alongside the main runtime artifact
- bounded JSON persistence keeps reconciliation state across restarts without turning runtime into a history store

## Working Rules

- Preserve `io.mb3r.bering.model@1.0.0` and `io.mb3r.bering.snapshot@1.0.0`.
- Keep policy logic centralized in one internal module.
- Keep release, versioning, and schema publishing aligned with the existing repo structure.
- Prefer thin runtime adapters and narrow operator-facing outputs.

## Planned Issues

1. Umbrella: evidence-weighted reconciliation for missing trace gaps
2. ADR/spec: runtime evidence reconciliation, projections, compatibility boundaries, and memory-bounded state retention
3. New internal reconciliation module and interfaces
4. Persistent evidence store, adaptive miss tracking, bounded retention, and config surface
5. Opportunity-aware negative evidence and telemetry-health gating
6. Projection and publication wiring: raw_window, stable_core, guardrail_union, discovery report
7. Metrics, HTTP/report/file outputs, operator observability, and release-safe artifact handling
8. Tests, fixtures, docs, changelog, rollout notes, and release/backlog alignment

## Tracking

The live GitHub milestone and linked issues are the authoritative backlog for this minor.
