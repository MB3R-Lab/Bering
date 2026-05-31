# Service Catalog Adapter Example

This is a reference non-trace adapter. It reads a small service catalog JSON file and emits Bering `topology_api` JSON.

Generate topology input:

```bash
go run ./examples/adapters/service-catalog \
  --input examples/adapters/service-catalog/catalog.sample.json \
  --out out/adapters/service-catalog/topology-api.json
```

The generated topology sample is checked in as `topology-api.generated.sample.json`.

Run Bering discovery on the generated topology:

```bash
go run ./cmd/bering discover \
  --input examples/adapters/service-catalog/topology-api.generated.sample.json \
  --out examples/outputs/bering-model.service-catalog-adapter.sample.json \
  --snapshot-out examples/outputs/bering-snapshot.service-catalog-adapter.sample.json \
  --discovered-at 2026-03-03T00:00:00Z
```

The generated model, snapshot, and signal-quality sidecars are checked in under `examples/outputs/`.

The adapter is intentionally plain Go and has no service-catalog dependency. Real adapters should map their source-specific concepts into the same `topology_api` fields shown here: services, placements, shared resources, dependencies, resilience policy, policy scope, and endpoints.
