# Topology Input Format

Bering also supports an explicit non-trace batch source: `topology_api`.

This is intended for cases where topology is already available from a topology API, service registry, mesh export, or another inventory source and you want Bering to publish the same model and snapshot artifacts without going through trace inference.

## Example

```yaml
source:
  type: topology_api
  ref: https://topology.internal.example/api/v1/topology
services:
  - id: frontend
    replicas: 2
edges:
  - from: frontend
    to: checkout
    kind: sync
endpoints:
  - entry_service: frontend
    method: GET
    path: /checkout
```

## Fields

### `source`

- `type`: defaults to `topology_api`
- `ref`: optional upstream source reference that will appear in snapshot `sources[]` and per-record provenance

### `services[]`

- `id`: required
- `name`: optional, defaults to `id`
- `replicas`: optional, defaults to `1`
- `support.observations`: optional, defaults to `1`
- `support.evidence`: optional, defaults to `[topology_api]`
- `first_seen`, `last_seen`: optional RFC3339 timestamps
- metadata fields: `labels`, `tags`, `slo_refs`, `attributes`, `failure_eligible`

### `edges[]`

- `from`, `to`, `kind`: required
- `kind`: `sync` or `async`
- `blocking`: optional, defaults to `true` for `sync` and `false` for `async`
- `id`: optional, derived from `from|to|kind|blocking`
- `support.*`, `first_seen`, `last_seen`, `labels`, `tags`, `slo_refs`, `attributes`, `weight`

### `endpoints[]`

- `entry_service`: required
- `id`: optional if `method` and `path` are provided
- `method`: normalized to uppercase
- `path`: normalized to start with `/`
- `predicate_ref`: optional, defaults to endpoint `id`
- `support.*`, `first_seen`, `last_seen`, `labels`, `tags`, `slo_refs`, `attributes`, `weight`

## Notes

- `bering discover` still emits `metadata.source_type = "bering"` in the stable model and snapshot envelope metadata.
- The upstream non-trace source is represented in snapshot `sources[]` and `discovery.*[].provenance`.
- For explicit `topology_api` input, `metadata.confidence` is set to `1.0` because the topology is supplied directly rather than inferred from traces.
- Discovery overlays continue to work on top of `topology_api` input.
