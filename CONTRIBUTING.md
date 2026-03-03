# Contributing

Thanks for contributing to Bering.

## Development

1. Install Go 1.23+.
2. Run `make lint test build` before opening a PR.
3. Keep model contract compatibility intact:
   - `metadata.schema.name = io.mb3r.bering.model`
   - `metadata.schema.version = 1.0.0`
   - `metadata.schema.uri = https://mb3r-lab.github.io/Bering/schema/model/v1.0.0/model.schema.json`
   - `metadata.schema.digest = sha256:272277c093f37580adcd2dded225bd37c86539d642d7910baad7e4228227d1a7`

## PR checklist

- [ ] New behavior is covered by tests.
- [ ] `bering discover` output stays deterministic.
- [ ] `bering validate` passes for examples.
- [ ] Docs updated if flags, formats, or heuristics changed.

