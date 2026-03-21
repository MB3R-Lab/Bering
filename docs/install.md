# Install

Bering `v0.2.0` is the current product release. It still emits the stable public schema contracts `io.mb3r.bering.model@1.0.0` and `io.mb3r.bering.snapshot@1.0.0`.

## Binary Archives

Download the archive matching your platform from the GitHub Release assets and verify it against `release-checksums.txt`.

Supported release targets:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`

Example release asset names for `v0.2.0`:

- `bering_0.2.0_linux_amd64.tar.gz`
- `bering_0.2.0_linux_arm64.tar.gz`
- `bering_0.2.0_darwin_amd64.tar.gz`
- `bering_0.2.0_darwin_arm64.tar.gz`
- `bering_0.2.0_windows_amd64.zip`

Example:

```bash
tar -xzf bering_0.2.0_linux_amd64.tar.gz
./bering help
```

Build from source:

```bash
go build ./cmd/bering
```

## OCI Image

Pull the published runtime image:

```bash
docker pull ghcr.io/mb3r-lab/bering:v0.2.0
docker run --rm ghcr.io/mb3r-lab/bering:v0.2.0 help
```

The release manifest records all published image references and the final digest in `oci_images`.

## OCI Helm Chart

Install directly from the published OCI chart:

```bash
helm install bering oci://ghcr.io/mb3r-lab/charts/bering --version 0.2.0
```

Override the image repository, tag, or digest if you mirror artifacts:

```bash
helm install bering oci://ghcr.io/mb3r-lab/charts/bering --version 0.2.0 \
  --set image.repository=ghcr.io/mb3r-lab/bering \
  --set image.digest=sha256:...
```

## Contracts Pack

Download `bering-contracts_<version>.tar.gz` to consume the public schema contracts offline:

```bash
tar -xzf bering-contracts_0.2.0.tar.gz
cat contracts-manifest.json
```

The pack contains:

- `contracts-manifest.json`
- `contracts-manifest.schema.json`
- `schema/model.schema.json`
- `schema/snapshot.schema.json`

The contracts pack version is the Bering product release version. The schema contracts inside it remain independently versioned and stay pinned to their own `name@version` values.
