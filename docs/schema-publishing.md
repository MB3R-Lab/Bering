# Schema Publishing (GitHub Pages)

This repository publishes the public model and snapshot schemas through GitHub Pages.

GitHub Pages is a distribution surface for schema URIs, not the canonical release source of truth. The canonical release metadata for automation is the generated `contracts-manifest.json` and `release-manifest.json` in `dist/`.

This workflow is separate from the product release workflow:

- product release tags: `vX.Y.Z`
- schema publishing tags: `schema-vA.B.C`

For the current release line, that means:

- product release: `v0.3.0`
- schema publishing: `schema-v1.1.0`

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
2. Add new versioned schema files under:
   - `api/schema/model/v<version>/model.schema.json`
   - `api/schema/snapshot/v<version>/snapshot.schema.json`
3. Copy the same versioned files into:
   - `internal/schema/schema/model/v<version>/model.schema.json`
   - `internal/schema/schema/snapshot/v<version>/snapshot.schema.json`
4. Update pinned refs and digests in `internal/schema/constants.go`.
5. Create and push a tag `schema-v<version>` when the pinned latest public schema contracts change.
6. Wait for workflow completion.
7. Verify these published URLs return `200`:
   - `https://mb3r-lab.github.io/Bering/schema/model/v<version>/model.schema.json`
   - `https://mb3r-lab.github.io/Bering/schema/snapshot/v<version>/snapshot.schema.json`
8. Verify the downloaded digests match the pinned digests in `internal/schema/constants.go`.

## Notes

- The stable model and the snapshot envelope are distinct contracts with separate names, URIs, and digests.
- `io.mb3r.bering.model@1.0.0` and `io.mb3r.bering.snapshot@1.0.0` remain published and immutable. Do not mutate those files in place.
- The current workflow expects the pushed `schema-v<version>` tag to match the pinned version of both schema files.
- Downstream consumers such as Sheaft can remain pinned to the model schema while runtime consumers adopt the snapshot schema.
- Never silently mutate a published schema version in place.
- The generated contracts pack now carries the versioned schema tree, for example `schema/model/v1.1.0/model.schema.json`, not a single unversioned flat copy.
- The GitHub Pages publish workflow mirrors the full versioned schema tree from `api/schema/` and refreshes `schema/model/latest/` plus `schema/snapshot/latest/` to the currently pinned contract line.
- `schema-v1.1.0` is published; the `v1.1.0` versioned URLs and both `latest/` aliases are live GitHub Pages endpoints.
