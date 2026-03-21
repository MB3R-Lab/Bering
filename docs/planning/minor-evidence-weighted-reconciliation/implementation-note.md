# Implementation Note: Evidence-Weighted Reconciliation for Trace Gaps

## Current Runtime Data Flow

- `bering serve` accepts OTLP/HTTP and optional OTLP/gRPC spans, normalizes them into `internal/connectors/traces.Span`, and batches them into a bounded tumbling window.
- `internal/runtime/engine.go` closes the active window on schedule, runs discovery, computes a stable model plus snapshot envelope, and writes that envelope to the configured sink.
- The runtime keeps only the active window and the previous snapshot for diffs and carry-forward metadata. Empty windows are skipped rather than published as new artifacts.

## Current Artifact and Publication Behavior

- Batch discovery publishes the stable `io.mb3r.bering.model@1.0.0` artifact and can optionally emit `io.mb3r.bering.snapshot@1.0.0`.
- Runtime publication currently centers on the snapshot envelope plus the latest sink file path.
- Release packaging is tag-driven, `dist/`-centric, and keeps public schema contracts pinned and separate from product versioning.
- Schema publishing is a separate GitHub Pages flow and must remain separate.

## Compatibility Boundaries

- Do not change the public contract names or versions for `io.mb3r.bering.model@1.0.0` or `io.mb3r.bering.snapshot@1.0.0`.
- Keep Sheaft-compatible downstream outputs conservative.
- Treat the richer reconciliation state as an internal/operator surface, not as a new public schema family.
- Preserve bounded runtime memory and bounded persisted state.

## Existing Repo Practices To Preserve

- Release tags are SemVer product tags (`vX.Y.Z`); schema publication uses separate `schema-vA.B.C` tags.
- Release assets are produced from `dist/` and `dist/release-manifest.json`.
- The repo documents release flow, versioning, and schema publishing separately; new minor work should follow that split rather than introduce a parallel mechanism.
- Planning artifacts should live under `docs/planning/`, and issue creation should stay GitHub-native.

## Proposed Insertion Points

- Add a dedicated internal reconciliation module, likely `internal/reconciliation/`, to own state, evidence weighting, projections, persistence, and compaction.
- Keep `internal/runtime/` thin: it should adapt runtime window observations into the reconciliation module and publish the returned projections and report.
- Extend runtime config with a single reconciliation block rather than scattering thresholds across the codebase.
- Add a read-only report output path and optionally a narrow operator HTTP surface if the runtime service already exposes one.

## Explicit Non-Goals For This Minor

- No new public schema contract versions.
- No Sheaft changes.
- No SQLite, BoltDB, Gob, or other heavyweight persistence layer.
- No broad ingest rewrite for route-perfect opportunity semantics.
- No release artifact composition changes unless a later code change proves they are strictly necessary and repo-consistent.
