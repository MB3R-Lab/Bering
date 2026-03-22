# Milestone: Minor: Evidence-Weighted Reconciliation for Trace Gaps

Historical scope note: this milestone predates the separate schema contract evolution minor. Its `1.0.0` contract references describe the runtime-only boundary at that time, not the current default emission line.

## Goal

Replace naive last-window disappearance semantics in Bering runtime with a persistent, opportunity-aware, evidence-weighted reconciliation module that publishes conservative downstream-compatible artifacts plus richer reconciliation/debug views.

## What This Minor Covers

- current-window discovery remains available as `raw_window`
- high-confidence retained topology is published as `stable_core`
- conservative downstream-compatible publication uses `guardrail_union`
- machine-readable reconciliation/discovery reporting is emitted alongside the main runtime artifact
- bounded JSON persistence keeps reconciliation state across restarts without turning runtime into a history store

## Working Rules

- Preserve the then-published `io.mb3r.bering.model@1.0.0` and `io.mb3r.bering.snapshot@1.0.0` within this runtime-scoped minor.
- Keep policy logic centralized in one internal module.
- Keep release, versioning, and schema publishing aligned with the existing repo structure.
- Prefer thin runtime adapters and narrow operator-facing outputs.
