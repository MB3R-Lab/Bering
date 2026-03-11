# Install

## Binary Archives

Download the archive matching your platform from the release assets and verify it against `release-checksums.txt`.

Supported release targets:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`

Example:

```bash
tar -xzf bering_1.2.3_linux_amd64.tar.gz
./bering help
```

## OCI Image

Pull the runtime image from any OCI registry:

```bash
docker pull registry.example.com/bering:v1.2.3
docker run --rm registry.example.com/bering:v1.2.3 help
```

The release manifest records all published image references and the final digest in `oci_images`.

## OCI Helm Chart

Install directly from an OCI registry:

```bash
helm install bering oci://registry.example.com/charts/bering --version 1.2.3
```

Override the image repository, tag, or digest if you mirror artifacts:

```bash
helm install bering oci://registry.example.com/charts/bering --version 1.2.3 \
  --set image.repository=registry.example.com/bering \
  --set image.digest=sha256:...
```

## Contracts Pack

Download `bering-contracts_<version>.tar.gz` to consume the public schema contracts offline:

```bash
tar -xzf bering-contracts_1.2.3.tar.gz
cat contracts-manifest.json
```

The pack contains:

- `contracts-manifest.json`
- `contracts-manifest.schema.json`
- `schema/model.schema.json`
- `schema/snapshot.schema.json`
