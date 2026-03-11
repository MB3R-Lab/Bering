# Schema Publishing (GitHub Pages)

This repository publishes the public model and snapshot schemas through GitHub Pages.

GitHub Pages is a distribution surface for schema URIs, not the canonical release source of truth. The canonical release metadata for automation is the generated `contracts-manifest.json` and `release-manifest.json` in `dist/`.

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

## Release operation model

1. Merge schema changes into `main`.
2. Create and push a tag `schema-v<version>` when a public schema changes.
3. Wait for workflow completion.
4. Verify each published URL returns `200`.
5. Verify the downloaded digest matches the pinned digest in `internal/schema/constants.go`.

## Notes

- The stable model and the snapshot envelope are versioned independently by schema name.
- Downstream consumers such as Sheaft can remain pinned to the model schema while runtime consumers adopt the snapshot schema.
- Never silently mutate a published schema version in place.
