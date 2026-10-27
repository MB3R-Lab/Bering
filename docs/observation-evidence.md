# Observation evidence from traces

Bering keeps trace observations distinct from configuration. `bering discover` emits a strict `io.mb3r.bering.model@1.3.0` model and, with `--snapshot-out`, a strict `io.mb3r.bering.snapshot@1.3.0` snapshot. `bering validate` accepts both. The model carries the usable service and edge graph. The snapshot's discovery records and both signal-quality sidecars carry typed evidence and unresolved facts.

## Where to read it

Each relevant `snapshot.discovery.services[]` or `snapshot.discovery.edges[]` record may have a JSON string at `metadata.attributes["bering.observation_evidence.v1"]`. Decode that string as an object with `schema_version: "1.0.0"`. The model and snapshot quality sidecars (for example, `model.signal-quality.json` beside `model.json`) expose the same records in `observation_evidence[]`, keyed by `entity_type` and `entity_id`. Sidecars also report counts, coverage, source mix, dimensions, and warnings. An absent evidence field is unknown, not a negative observation.

Evidence records can contain:

| Field | Meaning |
| --- | --- |
| `instances[]` | Observed or partial runtime identities and their source reference and instrumentation scope. |
| `replicas` | Model value, its basis, distinct observed identity count, and an optional supplied configured count. |
| `database` | Database protocol, logical endpoint address and optional valid port, `network_scope`, namespace hints, identity kind, and sources. |
| `database_aliases[]` | Client-side database endpoint hints associated with an observed receiver service or retained when receiver identity is ambiguous. |
| `relation` | `execution_link` from trace ancestry or `client_dependency` from a database client span, including available instance IDs. |
| `warnings[]` | Explicit limits and identity conflicts, such as `configured_replica_count_unknown`, `database_physical_identity_unknown`, `database_endpoint_identity_incomplete`, `server_receipt_unobserved`, and `blocking_is_client_span_hint`. |

`source.ref` is an input file path for batch ingest or the runtime source reference; `scope_name` and `scope_version` come from the OTLP instrumentation scope when present. These fields attribute an observation. They do not create a causal namespace or prove that two spans belong to separate systems.

`bering.observation_evidence.v1` is a reserved generated metadata key. Discovery rejects overlays that try to write it; supply configuration through the supported overlay fields instead.

Batch OTLP JSON, OTLP/HTTP, and OTLP/gRPC use the same normalized resource and scope fields for this mapping. The evidence rules depend on the supplied span data, not on a particular collector or ingestion transport.

## Instance and replica interpretation

Bering reads identity hints from resource attributes first: `service.instance.id`, `service.namespace`, `host.id`, `host.name`, `host.boot.id`, `container.id`, `process.pid`, `process.creation.time`, `process.start_time`, Kubernetes cluster, namespace, pod and node identifiers, and telemetry SDK fields. Span attributes remain in the merged compatibility map; a conflicting resource value produces a warning. A stable `service.instance.id`, or a host/process generation, supports an `observed` identity. Incomplete host or process hints are retained as `partial` evidence.

The graph projects `service.name` to a service node. If observations under one projected name carry multiple `service.namespace` values, evidence records `service_name_has_multiple_observed_namespaces`; the projection does not prove those namespaces are one runtime identity. Conflicting duplicate OTLP attributes are removed and recorded as `attribute_conflict:resource:<key>` or `attribute_conflict:span:<key>`, and they do not establish causal or database identity.

The count of distinct observed identities is historical evidence. It is **not** the number of concurrent replicas, a configured replica count, or proof of placement. Without supplied replica configuration, the model's compatibility replica value has basis `compatibility_default_unknown` and the evidence warns `configured_replica_count_unknown`. An explicit replica overlay is recorded with basis `supplied_overlay`. Observed host, container, process, and pod attributes do not become executable `metadata.placements` or reliability assumptions. Supply those through a topology input or overlay.

## Database identity and relations

A client span with `db.system.name` or `db.system` can create a database node and a caller-to-database edge. Its `server.address`, valid `server.port`, and supported `database.network_scope` form a **logical endpoint** identity. Matching scoped endpoints can be one shared node with edges from multiple callers. `db.namespace` is a namespace hint, not a separate physical server. A missing or malformed endpoint component produces a caller-scoped `unresolved_alias`; it cannot establish a shared physical resource. Loopback is scoped by container identity. Private addresses or hostnames require an explicit resource `bering.network.scope` or supported Kubernetes scope; a global public IP can be scoped globally. Ports outside 1–65535 are invalid.

An exact trace/parent-span pair between a database client span and a distinct server span reuses the observed receiver service and retains the client endpoint as a `database_aliases[]` evidence hint. It does not add a second synthetic database node for that invocation. When one logical endpoint has multiple observed receiver services without an exact pair for a given call, the alias remains unresolved and evidence warns `database_endpoint_has_multiple_observed_receivers`; Bering does not choose a receiver arbitrarily. A single unambiguous receiver may be reused for the same scoped endpoint. This is logical endpoint reconciliation, not physical host identity.

The synthetic database node's model replica value of one is a logical placeholder, not a measured server count. `database.physical_instance_id` is currently not populated. A synthetic caller edge is marked `client_dependency` with basis `database_client_attributes`; it records a client-side dependency, not server receipt, completed execution, or verified blocking. The warning `blocking_is_client_span_hint` states the limit of the blocking hint. Cross-service parent-child spans instead use `execution_link` with basis `trace_parent` when ancestry is unambiguous. Ambiguous or conflicting span identities are reported rather than arbitrarily linked.

Bering does not extract `db.statement` or infer SQL text from database observations. The graph and evidence are suitable for inspecting a likely dependency; a model user must provide semantic policy, actual replica configuration, physical resource identity, and placement information when those are needed for simulation.
