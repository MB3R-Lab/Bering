# Topology Input Format

Bering supports an explicit non-trace batch source: `topology_api`.

Use this when topology, placement, or resilience metadata already exists in a service registry, mesh export, inventory API, or curated file and you want Bering to publish the same stable model and snapshot artifacts without trace inference.

A reference non-trace adapter lives at [examples/adapters/service-catalog](../examples/adapters/service-catalog/README.md). It converts a small service catalog JSON document into `topology_api` JSON before calling `bering discover`.

## Contract line

- `bering discover` now emits `io.mb3r.bering.model@1.2.0` and `io.mb3r.bering.snapshot@1.2.0` by default.
- `io.mb3r.bering.model@1.0.0` and `io.mb3r.bering.snapshot@1.0.0` remain valid and immutable for existing consumers.
- New typed placement, reliability, resilience, observed-edge, policy-scope, endpoint-fidelity, and endpoint semantic-hint fields are first-class contract fields. They are not hidden in generic `attributes` bags.

## Example

```yaml
source:
  type: topology_api
  ref: https://topology.internal.example/api/v1/topology
services:
  - id: checkout
    replicas: 3
    reliability:
      live_probability: 0.985
      source: sre-calibration
      confidence: 0.80
    failure_eligible: true
    labels:
      team: commerce
    placements:
      - replicas: 2
        reliability:
          live_probability: 0.990
          source: az-history
          confidence: 0.75
        labels:
          region: us-east-1
          az: us-east-1a
          node_pool: blue
      - replicas: 1
        labels:
          region: us-east-1
          az: us-east-1b
          node_pool: green
    shared_resource_refs:
      - kafka-orders
edges:
  - from: frontend
    to: checkout
    kind: sync
    blocking: true
    identity:
      protocol: http
      operation: GET
      route: /checkout
    reliability:
      live_probability: 0.995
      source: dependency-slo
      confidence: 0.85
    resilience:
      request_timeout_ms: 2000
      retry:
        max_attempts: 3
        budget_cap: 1.5
        retry_on: [5xx, gateway-error, timeout]
        backoff:
          initial_ms: 50
          max_ms: 500
          multiplier: 2
      circuit_breaker:
        enabled: true
        max_pending_requests: 1024
    observed:
      latency_ms:
        p50: 40
        p90: 120
        p95: 250
        p99: 900
      error_rate: 0.02
    policy_scope:
      source_endpoint_id: frontend:GET /checkout
      method: GET
      source_route: /checkout
endpoints:
  - entry_service: frontend
    method: GET
    path: /checkout
    predicate_ref: catalog.frontend.checkout.success
    semantics:
      predicate_mode: immediate_response
      mandatory_targets:
        - checkout
      dependency_modes:
        - sync
      source: service-catalog
      confidence: 0.9
```

## Fields

### `source`

- `type`: defaults to `topology_api`
- `ref`: optional upstream source reference; Bering copies it into snapshot `sources[]` and `discovery.*[].provenance`

### `services[]`

- `id`: required
- `name`: optional, defaults to `id`
- `replicas`: optional, defaults to `1`
- `support.observations`: optional, defaults to `1`
- `support.evidence`: optional, defaults to `[topology_api]`
- `first_seen`, `last_seen`: optional RFC3339 timestamps
- `labels`, `tags`, `slo_refs`, `attributes`: optional metadata copied into discovery records; `labels`, `tags`, and `slo_refs` are also mirrored into the stable model
- `reliability.live_probability`: optional service live probability evidence, consumed by Sheaft as a service-level `theta` candidate
- `reliability.source`, `reliability.confidence`: optional provenance and confidence for the reliability value
- `failure_eligible`: optional boolean mirrored into the stable model
- `placements[]`: optional typed placement or failure-domain groups
  - each item supports `replicas`
  - each item also supports free-form typed labels such as `region`, `az`, `cell`, `rack`, `node`, or `node_pool`
  - each item can carry `reliability.live_probability` for placement or replica-group live probability evidence
- `shared_resource_refs[]`: optional typed shared fate-domain references such as broker, cache, or database identifiers

### `edges[]`

- `from`, `to`, `kind`: required
- `kind`: `sync` or `async`
- `blocking`: optional, defaults to `true` for `sync` and `false` for `async`
- `identity`: optional logical dependency discriminator used to derive operation-aware edge IDs
  - `protocol`
  - `operation`
  - `route`
  - `topic`
  - `span_kind`
- `id`: optional, derived as `from|to|kind|blocking` plus `identity` fields when present
- `support.*`, `first_seen`, `last_seen`, `labels`, `tags`, `slo_refs`, `attributes`, `weight`: optional
- `reliability.live_probability`: optional logical edge live probability evidence, consumed by Sheaft as an edge-level `rho` candidate
- `reliability.source`, `reliability.confidence`: optional provenance and confidence for the reliability value
- `resilience`: optional typed policy metadata
  - `request_timeout_ms`
  - `per_try_timeout_ms`
  - `retry.max_attempts`
  - `retry.budget_cap`
  - `retry.retry_on[]`
  - `retry.backoff.initial_ms`
  - `retry.backoff.max_ms`
  - `retry.backoff.multiplier`
  - `retry.backoff.jitter`
  - `circuit_breaker.enabled`
  - `circuit_breaker.max_pending_requests`
  - `circuit_breaker.max_requests`
  - `circuit_breaker.max_connections`
  - `circuit_breaker.consecutive_5xx`
  - `circuit_breaker.interval_ms`
  - `circuit_breaker.base_ejection_time_ms`
- `observed`: optional typed observed summary
  - `latency_ms.p50`
  - `latency_ms.p90`
  - `latency_ms.p95`
  - `latency_ms.p99`
  - `error_rate`
- `policy_scope`: optional typed scope metadata
  - `source_endpoint_id`
  - `source_route`
  - `method`
  - `operation`

### `endpoints[]`

- `entry_service`: required
- `id`: optional if `method` and `path` are provided
- `method`: normalized to uppercase and mirrored into the stable model
- `path`: normalized to start with `/` and mirrored into the stable model
- `predicate_ref`: optional, defaults to endpoint `id`
- `semantics`: optional producer evidence for downstream predicate selection
  - `predicate_mode`: `immediate_response`, `eventual_completion`, or `external_predicate`
  - `mandatory_targets[]`: service IDs that the producer says are mandatory for the endpoint property
  - `dependency_modes[]`: dependency modes relevant to the property, `sync` and/or `async`
  - `source`, `confidence`: optional provenance for the semantic hint
- `support.*`, `first_seen`, `last_seen`, `labels`, `tags`, `slo_refs`, `attributes`, `weight`: optional

## Current population rules

- `topology_api` input is a first-class source for every typed field above.
- Discovery overlays are also first-class sources for every typed field above.
- Trace-only discovery remains conservative: it can derive endpoint identity fields such as `method` and `path`, but it does not infer `semantics` without explicit topology or overlay evidence.
- Old overlay IDs in the base form `from|to|kind|blocking` are still accepted when they match exactly one operation-aware edge; ambiguous legacy matches fail instead of guessing.
- The stable model mirrors the high-value typed topology, placement, reliability, resilience, observed-edge, and endpoint-fidelity metadata when Bering has it.
- The snapshot keeps the same typed metadata plus provenance, support counts, runtime window fields, and discovery-only generic `attributes`.

## Notes

- `bering discover` still emits `metadata.source_type = "bering"` in stable model and snapshot metadata.
- For explicit `topology_api` input, `metadata.confidence` is set to `1.0` because the topology is supplied directly rather than inferred from traces.
- Bering publishes reliability evidence but does not convert it into availability math. Downstream tools such as Sheaft decide how service `theta` and edge `rho` values are selected for a given analysis profile.
- Bering publishes endpoint semantic hints but does not evaluate endpoint availability or business completion. `success_predicate_ref` remains the stable bridge to richer predicate definitions owned by downstream analysis configuration.
- Bering does not perform retry-amplification math, timeout-wave propagation, blast-radius scoring, or Sheaft fault simulation. This input format only publishes the typed contract data those downstream systems need.
