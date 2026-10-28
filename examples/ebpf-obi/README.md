# OBI eBPF source smoke

This example proves the product path from two uninstrumented Linux HTTP
processes through OpenTelemetry eBPF Instrumentation (OBI) into Bering. Neither
application imports an OpenTelemetry SDK. `edge-api` serves `GET /call` and
calls `lookup-api` at `GET /lookup`.

The smoke pins the official OBI v0.15.2 multi-platform image digest. OBI
discovers the two listening processes, injects and reads W3C trace context in
HTTP headers, and exports OTLP/HTTP protobuf directly to the declared Bering
source endpoint:

```text
edge-api -> lookup-api -> OBI -> /sources/obi-main/v1/traces -> Bering
```

Bering saves every received OTLP request under `out/ebpf-obi/obi-capture`.
After live ingestion, the smoke replays that independent raw capture with
`--input-type ebpf-obi` and requires the live and replay projections to have
the same services, caller-to-callee edge, and HTTP endpoints. Addresses and
timestamps are retained as evidence but are never used as a fallback for a
missing trace parent.

Run this from a source checkout or an extracted Linux release archive on a
disposable Linux host with Docker, Go, Python 3, and curl; OBI requires
privileged eBPF access. The archive path uses the bundled Bering binary and
builds only the two small standard-library applications:

```bash
bash ./examples/ebpf-obi/run-smoke.sh
```

The readiness loops are bounded technical startup guards. They are not model
acceptance thresholds. Logs, image inspection, raw OTLP capture, hashes, live
snapshot, and replay outputs remain in `out/ebpf-obi`.
