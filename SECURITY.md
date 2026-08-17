# Security Policy

## Supported versions

Bering separates product releases from schema-contract versions. Security support follows the product release line.

| Product version | Supported |
| --- | --- |
| `main` | Yes |
| `v1.0.0` | Yes |
| Pre-v1 preview releases / schemas | No |

The current strict public artifact contracts are `io.mb3r.bering.model@1.3.0` and `io.mb3r.bering.snapshot@1.3.0`. Pre-v1 preview schema lines are not part of the supported release surface.

## Reporting a vulnerability

Please do **not** open a public GitHub issue for a suspected security vulnerability.

Use GitHub Private Vulnerability Reporting if it is available for this repository. Otherwise, report the issue by email to:

`contact@mb3r-lab.ru`

Please include, when possible:

- the affected Bering version or commit;
- whether the issue affects batch discovery, validation, runtime OTLP ingest, reconciliation, artifact publishing, or release packaging;
- a minimized trace/topology/OTLP/configuration proof of concept;
- reproduction steps;
- expected and observed behavior;
- your assessment of impact and attacker prerequisites.

Do not send real production traces, credentials, customer data, or sensitive service topology unless absolutely necessary. Prefer a synthetic minimized payload.

We will review security reports as promptly as practical and coordinate remediation and disclosure with the reporter. Please avoid public disclosure until a fix or mitigation has been coordinated.

## Security-sensitive areas

Bering accepts externally produced topology and trace data and can expose long-running OTLP and reconciliation endpoints. It publishes artifacts that may be trusted by downstream automation. Security-sensitive areas therefore include:

- batch parsing of trace files and trace directories;
- explicit `topology_api` inputs;
- OTLP/HTTP and OTLP/gRPC ingestion;
- normalization of service/span identities and edge extraction;
- additive discovery overlays;
- schema validation and product/schema version separation;
- runtime reconciliation and signal-quality logic;
- HTTP health, metrics, and reconciliation endpoints;
- output and sidecar artifact generation;
- model/snapshot artifact integrity and provenance;
- Helm, OCI, binary, checksum, and GitHub Actions release paths.

## Security expectations

The following are intended security properties of Bering:

1. Trace, topology, overlay, and OTLP inputs are data and must never be executed.
2. Unsupported artifact contracts fail closed. Pre-v1 preview schemas must not be silently accepted or downgraded into the current v1 line.
3. Malformed or adversarial telemetry must not cause code execution, uncontrolled resource consumption, unintended filesystem access, or silent corruption of a published model/snapshot.
4. Runtime evidence gaps and uncertainty must remain explicit; reconciliation must not silently fabricate observed topology.
5. Additive overlays must not bypass schema validation or silently convert unvalidated static assertions into runtime-discovered evidence.
6. A failed validation or incomplete write must not be exposed as a trusted current artifact.
7. Downstream consumers must be able to distinguish a valid supported Bering artifact from malformed, incompatible, or partially written input.
8. Telemetry, topology, reconciliation, and metrics endpoints must not expose more operational information than the deployment intends.

## What counts as a security issue

Examples include, but are not limited to:

- remote code execution or arbitrary file access through parsers or runtime endpoints;
- unauthenticated or low-cost denial of service through OTLP or pathological trace cardinality;
- model poisoning that bypasses documented validation or provenance boundaries;
- schema/version confusion or downgrade;
- exposure of sensitive trace, service, topology, placement, or reconciliation data;
- output path traversal, unsafe temporary-file handling, or partial-artifact trust;
- compromise of build/release artifacts or credentials.

A difference between Bering's discovered topology and the real system is normally a model-quality/correctness issue rather than a security vulnerability unless an attacker can deliberately cause that difference to cross a documented trust boundary or influence a downstream trusted decision.
