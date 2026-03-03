# Schema Publishing (GitHub Pages)

This repository publishes the public model schema through GitHub Pages.

## One-time repository setup

1. Open `Settings -> Pages`.
2. Set source to `GitHub Actions`.
3. Ensure `Settings -> Actions -> General` allows workflows to deploy Pages.

After first deployment, GitHub creates the `github-pages` environment automatically.

## Publish workflow

- File: `.github/workflows/publish-schema.yml`
- Trigger: `push` tags matching `schema-v*`
- Optional emergency path: `workflow_dispatch`

The workflow:

1. Reads `ExpectedSchemaVersion`, `ExpectedSchemaURI`, and `ExpectedSchemaDigest` from `internal/schema/constants.go`.
2. Validates tag/version binding (`schema-vX.Y.Z` must match `ExpectedSchemaVersion`).
3. Validates schema JSON and `$id` binding.
4. Builds a Pages artifact with:
   - `schema/model/v<version>/model.schema.json`
   - `schema/model/latest/model.schema.json`
   - `schema/index.json`
5. Deploys to GitHub Pages.

## Release operation model

1. Merge schema changes into `main`.
2. Create and push tag `schema-v<version>` (for example, `schema-v1.0.0`).
3. Wait for workflow completion.
4. Verify:
   - schema URL returns `200`
   - downloaded schema digest matches `ExpectedSchemaDigest`

## Notes

- This stage updates Bering only.
- Sheaft currently pins strict URI and digest independently and must be migrated in a separate coordinated change.

