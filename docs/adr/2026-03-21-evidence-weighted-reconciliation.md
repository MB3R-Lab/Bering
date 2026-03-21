# ADR 2026-03-21: Evidence-Weighted Reconciliation for Runtime Trace Gaps

## Status

Proposed

## Context

Runtime mode currently closes a tumbling window, runs discovery, and publishes a snapshot envelope. That model is conservative, but it is also brittle when a service or edge goes temporarily quiet for reasons unrelated to actual topology removal.

The current behavior is too eager to forget topology if the latest window does not contain fresh evidence. That is acceptable for batch discovery, but not for long-running runtime publication where sparse traffic, late spans, and telemetry instability are common.

## Decision

Add a dedicated internal reconciliation module that maintains bounded, evidence-weighted entity state across windows and process restarts.

The module will:

- separate observed current-window data from retained belief
- distinguish active, stale, and retired lifecycle states
- use qualified opportunity misses instead of treating absence as automatic negative evidence
- freeze or damp retirement when telemetry health is poor
- persist compact JSON state for operator visibility and restart continuity
- expose conservative projections for publication

Runtime publication will keep the downstream-compatible artifact conservative by default. The guardrail union view is the default runtime-facing publication path, while `raw_window` and `stable_core` are additional views and the reconciliation report is a machine-readable diagnostic surface.

The public schema contracts remain unchanged:

- `io.mb3r.bering.model@1.0.0`
- `io.mb3r.bering.snapshot@1.0.0`

## Consequences

- Runtime no longer erases topology solely because it was absent from one window.
- Operators gain a report that explains why topology was kept, downgraded, or retired.
- Memory and persisted state remain bounded through eviction and compaction.
- Future route-family opportunity tracking remains possible, but it is not required for this minor.

## Non-Goals

- No new public schema family.
- No Sheaft changes.
- No heavyweight persistence engine.
- No ingest-layer rewrite for perfect route-level opportunity semantics.
