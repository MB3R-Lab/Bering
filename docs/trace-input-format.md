# Trace Input Format

Bering supports two batch JSON formats and two runtime network ingest paths.

For explicit non-trace batch topology input, see [topology-input-format.md](topology-input-format.md).

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
- `resourceSpans[].scopeSpans[]` (or `instrumentationLibrarySpans[]`)
- `...scopeSpans[].spans[]`

Supported OTLP fields:

- `traceId`, `spanId`, `parentSpanId`, `name`, `kind`
- `startTimeUnixNano`, `endTimeUnixNano`
- resource/span attributes in key/value form

`service.name` is resolved from resource attributes first, then span attributes.

## 3) Runtime OTLP/HTTP ingest

`bering serve` accepts OTLP/HTTP at `POST /v1/traces`.

Supported request encodings:

- protobuf (`application/x-protobuf`)
- JSON (`application/json`)
- optional `Content-Encoding: gzip`

This is the primary integration path for any standard OpenTelemetry Collector or SDK exporter.

## 4) Runtime OTLP/gRPC ingest

`bering serve` optionally accepts OTLP/gRPC on `server.grpc_listen_address`.

Supported request shape:

- standard `opentelemetry.proto.collector.trace.v1.TraceService/Export`
- protobuf OTLP payloads normalized through the same span conversion path as OTLP/HTTP

## Discovery-relevant attributes

### HTTP endpoint inference

- `http.request.method`, `http.method`
- `http.route`, `url.path`, `http.target`
- span name fallback such as `GET /checkout`

### Async edge heuristic

- span kind `producer` or `consumer`
- `messaging.system`
- `messaging.destination`
- `messaging.operation`

## Input mode

`--input` can point to:

- a single JSON file
- a directory (all `*.json` files recursively, sorted by path)
