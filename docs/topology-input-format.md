# Topology Input Format

Bering supports an explicit non-trace batch source: `topology_api`.

Use this when topology, placement, or resilience metadata already exists in a service registry, mesh export, inventory API, or curated file and you want Bering to publish the same stable model and snapshot artifacts without trace inference.

## Contract line

- `bering discover` now emits `io.mb3r.bering.model@1.1.0` and `io.mb3r.bering.snapshot@1.1.0` by default.
- `io.mb3r.bering.model@1.0.0` and `io.mb3r.bering.snapshot@1.0.0` remain valid and immutable for existing consumers.
- New typed placement, resilience, observed-edge, policy-scope, and endpoint-fidelity fields are first-class contract fields. They are not hidden in generic `attributes` bags.

## Example

```yaml
source:
  type: topology_api
  ref: https://topology.internal.example/api/v1/topology
services:
  - id: checkout
    replicas: 3
    failure_eligible: true
    labels:
      team: commerce
    placements:
      - replicas: 2
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
    id: frontend|checkout|sync|true
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
- `failure_eligible`: optional boolean mirrored into the stable model
- `placements[]`: optional typed placement or failure-domain groups
  - each item supports `replicas`
  - each item also supports free-form typed labels such as `region`, `az`, `cell`, `rack`, `node`, or `node_pool`
- `shared_resource_refs[]`: optional typed shared fate-domain references such as broker, cache, or database identifiers

### `edges[]`

- `from`, `to`, `kind`: required
- `kind`: `sync` or `async`
- `blocking`: optional, defaults to `true` for `sync` and `false` for `async`
- `id`: optional, derived as `from|to|kind|blocking`
- `support.*`, `first_seen`, `last_seen`, `labels`, `tags`, `slo_refs`, `attributes`, `weight`: optional
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
- `support.*`, `first_seen`, `last_seen`, `labels`, `tags`, `slo_refs`, `attributes`, `weight`: optional

## Current population rules

- `topology_api` input is a first-class source for every typed field above.
- Discovery overlays are also first-class sources for every typed field above.
- The stable model mirrors the high-value typed topology, placement, resilience, observed-edge, and endpoint-fidelity metadata when Bering has it.
- The snapshot keeps the same typed metadata plus provenance, support counts, runtime window fields, and discovery-only generic `attributes`.

## Notes

- `bering discover` still emits `metadata.source_type = "bering"` in stable model and snapshot metadata.
- For explicit `topology_api` input, `metadata.confidence` is set to `1.0` because the topology is supplied directly rather than inferred from traces.
- Bering does not perform retry-amplification math, timeout-wave propagation, blast-radius scoring, or Sheaft fault simulation. This input format only publishes the typed contract data those downstream systems need.
