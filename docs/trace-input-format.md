# Trace Input Format

Bering supports two batch JSON formats and two runtime ingest paths.

For explicit non-trace topology input, see [topology-input-format.md](topology-input-format.md).

## Contract line

- `bering discover` and `bering serve` emit `io.mb3r.bering.model@1.1.0` and `io.mb3r.bering.snapshot@1.1.0` by default.
- `bering validate` still accepts preserved `1.0.0` artifacts.
- Trace inputs can now populate observed edge latency summaries when span timing exists.

## 1) Normalized spans JSON

Top-level shape:

```json
{
  "spans": [
    {
      "trace_id": "trace-1",
      "span_id": "span-1",
      "parent_span_id": "",
      "service": "frontend",
      "kind": "server",
      "name": "GET /checkout",
      "start_time": "2026-03-11T12:00:00Z",
      "end_time": "2026-03-11T12:00:00.050Z",
      "attributes": {
        "http.request.method": "GET",
        "http.route": "/checkout"
      }
    }
  ]
}
```

Accepted aliases:

- `traceId` for `trace_id`
- `spanId` for `span_id`
- `parentSpanId` for `parent_span_id`
- `service_name` or `service.name` for `service`
- `startTime` for `start_time`
- `endTime` for `end_time`

Timestamp values may be RFC3339 strings.

## 2) Raw OTLP JSON payload

Expected hierarchy:

- `resourceSpans[]`
- `resourceSpans[].resource.attributes[]`
- `resourceSpans[].scopeSpans[]` or `instrumentationLibrarySpans[]`
- `...scopeSpans[].spans[]`

Supported OTLP fields:

- `traceId`, `spanId`, `parentSpanId`, `name`, `kind`
- `startTimeUnixNano`, `endTimeUnixNano`
- resource and span attributes in key/value form

`service.name` is resolved from resource attributes first, then span attributes.

## 3) Runtime OTLP/HTTP ingest

`bering serve` accepts OTLP/HTTP at `POST /v1/traces`.

Supported encodings:

- protobuf: `application/x-protobuf`
- JSON: `application/json`
- optional `Content-Encoding: gzip`

## 4) Runtime OTLP/gRPC ingest

`bering serve` optionally accepts OTLP/gRPC on `server.grpc_listen_address`.

Supported request shape:

- standard `opentelemetry.proto.collector.trace.v1.TraceService/Export`
- protobuf OTLP payloads normalized through the same conversion path as OTLP/HTTP

## What Bering derives from traces today

### Stable topology and endpoint shape

- services
- edges
- deterministic edge ids using `from|to|kind|blocking`
- endpoint ids
- endpoint `method`
- endpoint `path`

### Edge kind and blocking hints

Async edge detection uses:

- span kind `producer` or `consumer`
- `messaging.system`
- `messaging.destination`
- `messaging.operation`

### Observed edge timing summary

When a discovered cross-service edge has child spans with both `start_time` and `end_time`, Bering computes:

- `observed.latency_ms.p50`
- `observed.latency_ms.p90`
- `observed.latency_ms.p95`
- `observed.latency_ms.p99`

The current implementation uses the discovered child-span duration as the directly observed timing sample for that edge. If timestamps are absent, Bering leaves `observed` unset rather than fabricating values.

## What traces do not populate generically today

Generic trace ingestion does not currently infer:

- `resilience.request_timeout_ms`
- `resilience.per_try_timeout_ms`
- retry budgets or retry policies
- circuit-breaker limits
- placement groups
- shared resource references

Populate those fields through `topology_api` input or discovery overlays instead.

## Discovery-relevant attributes

### HTTP endpoint inference

- `http.request.method`, `http.method`
- `http.route`, `url.path`, `http.target`
- span-name fallback such as `GET /checkout`

### Messaging and async inference

- `messaging.system`
- `messaging.destination`
- `messaging.operation`

## Input mode

`--input` can point to:

- a single JSON file
- a directory; Bering reads all `*.json` files recursively in path order

## Scope boundary

Trace ingestion in Bering stops at discovery and publication. It does not implement retry-amplification math, timeout-wave propagation, blast-radius scoring, or Sheaft fault simulation.
