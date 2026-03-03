# Contributing

Thanks for contributing to Bering.

## Development

1. Install Go 1.23+.
2. Run `make lint test build` before opening a PR.
3. Keep model contract compatibility intact:
   - `metadata.schema.name = io.mb3r.bering.model`
   - `metadata.schema.version = 1.0.0`
   - `metadata.schema.uri = https://schemas.mb3r.dev/bering/model/v1.0.0/model.schema.json`
   - `metadata.schema.digest = sha256:7dc733936a9d3f94ab92f46a30d4c8d0f5c05d60670c4247786c59a3fe7630f7`

## PR checklist

- [ ] New behavior is covered by tests.
- [ ] `bering discover` output stays deterministic.
- [ ] `bering validate` passes for examples.
- [ ] Docs updated if flags, formats, or heuristics changed.

