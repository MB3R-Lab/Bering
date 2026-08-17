# Threat Model

## 1. Scope

This threat model covers Bering `v1.0.0` and the current `main` line as the runtime model-discovery and artifact-publishing layer in the MB3R toolchain:

```text
                 Procrustes
          static collection plan / overlay
                       |
                       v
trace files ----->  Bering  <----- OTLP/HTTP, OTLP/gRPC
explicit topology      |
                       | model / snapshot / signal quality
                       v
                     Sheaft
                       |
                       v
             resilience / CI gate
```

Bering owns runtime evidence, topology discovery, discovery-side validation, signal quality, reconciliation, and the public model/snapshot contracts. It does **not** own resilience simulation, policy evaluation, gating, or chaos execution.

The primary security question is:

> Can an attacker controlling telemetry, topology input, configuration, the network ingestion path, or the artifact handoff cause code execution, denial of service, disclosure of operational data, or a trusted but attacker-controlled model/snapshot downstream?

## 2. Security-relevant behavior

Bering currently supports:

- deterministic batch discovery from trace files, trace directories, or explicit `topology_api` inputs;
- validation against pinned public JSON schemas;
- runtime OTLP ingest over HTTP and optional OTLP/gRPC;
- publication of stable model and snapshot artifacts;
- additive discovery overlays;
- signal-quality sidecars;
- long-running runtime mode with rolling discovery snapshots and reconciliation views;
- HTTP endpoints including `/healthz`, `/readyz`, `/metrics`, `/reconciliation/report`, and `/reconciliation/summary`.

The current strict artifact contracts are:

- `io.mb3r.bering.model@1.3.0`;
- `io.mb3r.bering.snapshot@1.3.0`.

Pre-v1 preview schema lines are intentionally outside the current release surface.

## 3. Assets

The assets to protect are:

1. **Integrity of model and snapshot artifacts** — topology, endpoint contracts, replicas, evidence, placement/shared-resource metadata, and reconciliation state must not be attacker-controlled without being represented as untrusted/invalid evidence.
2. **Integrity of downstream decisions** — Sheaft or another consumer may rely on Bering artifacts for resilience analysis and CI/CD gating.
3. **Availability of Bering runtime ingest and reconciliation**.
4. **Confidentiality of telemetry and derived topology** — service names, endpoints, dependencies, placements, latency/error metadata, and operational structure can be sensitive.
5. **Integrity of schema/version interpretation**.
6. **Integrity and atomicity of files published to downstream consumers**.
7. **Integrity of release binaries, OCI images, Helm charts, checksums, and GitHub Actions workflows**.

## 4. Trust boundaries

### TB1 — Procrustes handoff -> Bering

Procrustes may supply a collection plan and conservative overlay containing confirmed static facts. This is useful context, not runtime proof. Bering remains responsible for validating trace evidence and runtime topology.

### TB2 — Batch trace/topology input -> Bering

Files or directories passed to discovery may be untrusted, corrupted, oversized, highly nested, or crafted to create pathological identities and graph structures.

### TB3 — Network OTLP ingest -> Bering

`POST /v1/traces` on OTLP/HTTP and the optional OTLP/gRPC listener accept remotely supplied telemetry. A compromised workload or reachable attacker may be able to submit malformed or high-cardinality data.

Deployments should not assume that a default OTLP listener is safe for unrestricted Internet exposure. Network authentication, TLS termination, tenant isolation, and rate controls may be supplied by the surrounding deployment unless explicitly implemented by Bering.

### TB4 — Runtime query endpoints -> operators/monitoring

Health, readiness, metrics, and reconciliation endpoints expose process state and potentially operational metadata. Their network exposure must match the deployment's confidentiality requirements.

### TB5 — Discovery/reconciliation engine -> artifact publisher

Internal state is turned into stable model/snapshot artifacts plus signal-quality/reconciliation views. Partial state, parse failures, or concurrent updates must not be published as a trusted complete artifact.

### TB6 — Bering artifact -> Sheaft or another consumer

The Bering-to-Sheaft handoff is security-sensitive because the downstream consumer can influence automated engineering decisions. Artifact contract identity, validation, provenance, and atomicity matter at this boundary.

### TB7 — Source/build pipeline -> distributed Bering artifacts

Go dependencies, GitHub Actions, GoReleaser, OCI publishing, Helm packaging, checksums, and release credentials form the software supply-chain boundary.

## 5. Threat actors

Relevant actors include:

- a compromised instrumented workload emitting malicious OTLP;
- an attacker with network access to the OTLP listener;
- a malicious or corrupted trace/topology file supplied to batch discovery;
- a malicious operator or CI job controlling overlay/config/output paths;
- an attacker able to replace or modify an artifact between Bering and Sheaft;
- a compromised dependency, GitHub Action, package registry, or release credential;
- accidental pathological telemetry with the same availability effect as an attack.

## 6. Threats and abuse cases

### T1 — Parser exploitation

Malformed protobuf/JSON/trace/topology/overlay input may trigger panic, memory-safety-like consequences in unsafe dependencies, deserialization bugs, or logic confusion.

**Desired controls:** strict decoding, schema validation at boundaries, fuzzing of every accepted input family, panic-to-error discipline, dependency review.

### T2 — Algorithmic denial of service

An attacker may send very large requests, extreme span counts, huge attribute sets, high-cardinality service identities, adversarial parent/child structures, or graph shapes that cause excessive CPU, memory, reconciliation work, output size, or GC pressure.

**Desired controls:** request/body limits, bounded queues/batches, cardinality and object-count limits, cancellation/timeouts, complexity tests, metrics for rejected/limited input.

### T3 — Model poisoning through telemetry

A compromised service can emit plausible but false spans that create services, edges, endpoint semantics, or evidence values intended to distort the discovered model.

**Desired controls:** provenance on evidence, confidence/signal-quality reporting, conservative reconciliation, optional allowlists or deployment constraints where appropriate, and clear separation between observed telemetry and static assertions.

Bering cannot in general prove that an authenticated workload reports truthful telemetry; the goal is to prevent untrusted evidence from silently becoming stronger than its provenance permits.

### T4 — Schema/version confusion or downgrade

Crafted artifacts may attempt to exploit the distinction between Bering product version and model/snapshot schema version, or rely on retired preview semantics.

**Desired controls:** exact contract-name/version checks, pinned schema digests where published, no implicit fallback, negative tests for retired `1.0.0`/`1.1.0`/`1.2.0` preview lines.

### T5 — Overlay authority escalation

A crafted overlay may attempt to override discovered runtime evidence, inject unsupported topology, or change contract semantics beyond additive enrichment.

**Desired controls:** schema restrictions, field-level merge rules, provenance retention, conflict detection, tests that overlays cannot manufacture runtime-discovered edges or bypass required evidence.

### T6 — Partial or non-atomic artifact publication

A downstream watcher may observe an artifact while Bering is still writing it, or after a crash has left truncated output, and treat it as current.

**Desired controls:** write-to-temp + atomic replace where supported, validate-before-publish, immutable snapshot identifiers/fingerprints, downstream schema validation.

### T7 — Artifact substitution between Bering and Sheaft

An attacker with filesystem or pipeline access may replace a valid model/snapshot with another valid-looking artifact.

**Desired controls:** documented provenance/fingerprint fields, optional digest verification at integration boundaries, least-privilege filesystem permissions, checksums/signatures for distributed release artifacts.

### T8 — Sensitive-data exposure

Trace attributes, service names, endpoint paths, placement metadata, reconciliation reports, metrics labels, or generated artifacts may reveal internal architecture or user data.

**Desired controls:** minimize retained attributes, avoid copying arbitrary span payloads into public artifacts, document endpoint exposure, support redaction/allowlisting where required, avoid secrets in logs.

### T9 — Unauthenticated runtime endpoint abuse

If OTLP or reconciliation endpoints are reachable by unintended clients, an attacker may submit traffic, scrape metadata, or exhaust runtime resources.

**Desired controls:** secure deployment guidance, bind/exposure controls, reverse-proxy or mesh authentication where authentication is not built in, network policies, rate limiting where appropriate.

### T10 — Filesystem/path handling

Batch inputs, output paths, overlay paths, or runtime publishing paths may be crafted for path traversal, symlink abuse, overwrite, or read/write outside the intended scope.

**Desired controls:** canonicalization, explicit root policies, safe create/replace behavior, symlink tests, least-privilege runtime user.

### T11 — Reconciliation confusion under missing telemetry

An attacker may intentionally create intermittent or partial telemetry to make stale topology appear current or to exploit reconciliation heuristics.

**Desired controls:** freshness metadata, explicit missingness and signal-quality state, conservative reconciliation, no silent promotion of stale evidence to current observation.

### T12 — Supply-chain compromise

A compromised dependency, GitHub Action, release credential, OCI pipeline, or Helm packaging step could alter distributed Bering binaries/images/charts.

**Desired controls:** least-privilege workflow permissions, pinned third-party Actions, `govulncheck`, dependency review, checksums/provenance, protected release credentials, reproducibility where feasible.

## 7. Security invariants

The following invariants should remain true across the supported v1 line:

1. Unsupported model/snapshot contracts fail closed.
2. Product-version metadata is never used as a substitute for schema-contract validation.
3. Pre-v1 preview schemas cannot silently enter the current strict v1 path.
4. Input telemetry and topology are always treated as data, never executable content.
5. Additive overlays cannot silently become stronger than runtime evidence without provenance/conflict representation.
6. Missing or weak telemetry is represented through signal-quality/reconciliation state rather than fabricated certainty.
7. Invalid or partially written artifacts are not published as trusted current snapshots.
8. The same valid batch input with the same discovery parameters produces deterministic contract-level output.
9. A downstream consumer can independently validate the artifact contract before trusting it.
10. A single hostile client should not be able to consume unbounded process resources at negligible cost.

## 8. Recommended security testing

High-value review and test work includes:

- Go native fuzz targets for OTLP/HTTP, OTLP/gRPC decoding, normalized trace input, topology input, overlay parsing, and model/snapshot validation;
- property tests for graph/identity normalization and order independence;
- cardinality/complexity benchmarks with hard regression thresholds;
- request-size, batch-size, span-count, attribute-count, and service-count boundary tests;
- negative tests for every retired schema line and malformed contract identifier;
- atomic-write/crash/interruption tests for model, snapshot, and sidecar publication;
- tests for reconciliation freshness and stale evidence;
- endpoint information-disclosure review;
- filesystem traversal/symlink tests;
- `govulncheck`, static analysis, dependency review, and GitHub Actions permissions review;
- OCI/Helm/release provenance and checksum validation.

## 9. Deployment assumptions

Bering is expected to run inside a controlled observability or platform environment. Unless a deployment explicitly adds the necessary controls, this threat model does not assume built-in multi-tenant isolation, Internet-facing authentication, or TLS termination for every runtime endpoint.

Operators should apply network policy, authentication/authorization, TLS, resource limits, and filesystem permissions appropriate to the environment.

## 10. Out of scope

The following are not security vulnerabilities by themselves:

- a scientifically explainable difference between the discovered model and the real system;
- unsupported rare edges caused by insufficient trace coverage;
- resilience-simulation or CI-gating correctness inside Sheaft;
- future retry, timeout-wave, overload-cascade, or chaos-execution semantics that Bering explicitly does not own.

They become security-relevant if an attacker can deliberately exploit them to violate a documented trust boundary or corrupt a trusted artifact/decision.
