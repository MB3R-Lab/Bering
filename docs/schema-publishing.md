# Schema Publishing (GitHub Pages)

This repository publishes the public model and snapshot schemas through GitHub Pages.

GitHub Pages is a distribution surface for schema URIs, not the canonical release source of truth. The canonical release metadata for automation is the generated `contracts-manifest.json` and `release-manifest.json` in `dist/`.

This workflow is separate from the product release workflow:

- product release tags: `vX.Y.Z`
- schema publishing tags: `schema-vA.B.C`

For the first public product release, that means:

- product release: `v0.1.0`
- schema publishing: `schema-v1.0.0`

## One-time repository setup

1. Open `Settings -> Pages`.
2. Set source to `GitHub Actions`.
3. Ensure `Settings -> Actions -> General` allows workflows to deploy Pages.

After first deployment, GitHub creates the `github-pages` environment automatically.

## Publish workflow

- File: `.github/workflows/publish-schema.yml`
- Trigger: `push` tags matching `schema-v*`
- Optional emergency path: `workflow_dispatch`

The workflow should publish both schema families:

- `schema/model/v<version>/model.schema.json`
- `schema/model/latest/model.schema.json`
- `schema/snapshot/v<version>/snapshot.schema.json`
- `schema/snapshot/latest/snapshot.schema.json`
- `schema/index.json`

`schema/index.json` is a convenience index. The canonical public schema URIs remain the versioned files above.

## Release operation model

1. Merge schema changes into `main`.
2. Create and push a tag `schema-v<version>` when the pinned public schema contracts change.
3. Wait for workflow completion.
4. Verify these published URLs return `200`:
   - `https://mb3r-lab.github.io/Bering/schema/model/v<version>/model.schema.json`
   - `https://mb3r-lab.github.io/Bering/schema/snapshot/v<version>/snapshot.schema.json`
5. Verify the downloaded digests match the pinned digests in `internal/schema/constants.go`.

## Notes

- The stable model and the snapshot envelope are distinct contracts with separate names, URIs, and digests.
- The current workflow expects the pushed `schema-v<version>` tag to match the pinned version of both schema files.
- Downstream consumers such as Sheaft can remain pinned to the model schema while runtime consumers adopt the snapshot schema.
- Never silently mutate a published schema version in place.
