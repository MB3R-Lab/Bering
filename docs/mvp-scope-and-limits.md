# MVP Scope and Limits

## In scope

- Discover model from trace files/directories.
- Emit contract-compliant `bering-model.json` (v1.0.0) compatible with Sheaft and other downstream analytics tools.
- Validate artifact with strict `metadata.schema` match.
- Deterministic output ordering and canonical JSON serialization.

## Out of scope (MVP)

- Non-trace connectors (runtime service registries, topology APIs).
- Advanced dependency semantics (timeouts, retries, circuit breakers).
- Correlated failures and probabilistic calibration from production data.
- Multi-version schema negotiation.

## Confidence heuristic (MVP)

`metadata.confidence` is deterministic and computed from discovery coverage:

- base score: `0.35`
- `+0.20` if more than one service discovered
- `+0.15` if at least one edge discovered
- `+0.15` if at least one endpoint inferred
- `+min(0.15, cross_service_parent_child_ratio * 0.15)`

Result is clamped to `[0,1]` and rounded to 2 decimals.

This score is informational in MVP and should not be interpreted as a calibrated
probability of correctness.
