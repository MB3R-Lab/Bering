# Trace Input Format (MVP)

Bering MVP supports two JSON input formats.

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

## 2) Raw OTel JSON payload

Expected hierarchy:

- `resourceSpans[]`
- `resourceSpans[].resource.attributes[]`
- `resourceSpans[].scopeSpans[]` (or `instrumentationLibrarySpans[]`)
- `...scopeSpans[].spans[]`

Supported OTel fields:

- `traceId`, `spanId`, `parentSpanId`, `name`, `kind`
- resource/span attributes in key/value form (`key` + `value.stringValue|intValue|doubleValue|boolValue`)

`service.name` is resolved from resource attributes first, then span attributes.

## Discovery-relevant attributes

- HTTP endpoint inference:
  - `http.request.method`, `http.method`
  - `http.route`, `url.path`, `http.target`
- Async edge heuristic:
  - `messaging.system`
  - `messaging.destination`
  - `messaging.operation`
  - span kind `producer` or `consumer`

## Input mode

- `--input` can point to:
  - a single JSON file
  - a directory (all `*.json` files recursively, sorted by path)

