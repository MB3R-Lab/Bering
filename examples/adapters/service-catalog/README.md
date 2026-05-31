# Service Catalog Adapter Example

This is a reference non-trace adapter. It reads a small service catalog JSON file and emits Bering `topology_api` JSON.

Generate topology input:

```bash
go run ./examples/adapters/service-catalog \
  --input examples/adapters/service-catalog/catalog.sample.json \
  --out out/adapters/service-catalog/topology-api.json
```

Run Bering discovery on the generated topology:

```bash
go run ./cmd/bering discover \
  --input out/adapters/service-catalog/topology-api.json \
  --out out/adapters/service-catalog/bering-model.json \
  --snapshot-out out/adapters/service-catalog/bering-snapshot.json \
  --discovered-at 2026-03-03T00:00:00Z
```

The adapter is intentionally plain Go and has no service-catalog dependency. Real adapters should map their source-specific concepts into the same `topology_api` fields shown here: services, placements, shared resources, dependencies, resilience policy, policy scope, and endpoints.
