# Runtime Config

`bering serve` reads YAML or JSON config. Flags may override config values.

Sample file: [configs/serve.sample.yaml](../configs/serve.sample.yaml)

## Fields

### `runtime.publication_mode`

`windowed` (the default) publishes scheduled tumbling-window snapshots. `on_shutdown` collects one batch and publishes its full graph only after the service has drained active requests on shutdown. The latter mode does not roll over on wall-clock boundaries and does not discard spans based on their event timestamps; original timestamps and native/OBI source attribution remain in the observations.

For `on_shutdown`, set `runtime.reconciliation.enabled: false`. The reconciliation history requires multiple windows and is rejected in this mode. `runtime.window_size`, `runtime.flush_interval`, and `runtime.late_span_policy` do not control batch admission or publication. `runtime.max_in_memory_spans` remains a hard capacity: exceeding it invalidates the batch, and shutdown returns an error without publishing its accepted prefix. Each OTLP request still has the configured byte limit. Stop the service normally after sending all exports and check its exit status before accepting the snapshot.

The Helm chart exposes this as `config.runtime.publicationMode`. Set `config.runtime.reconciliation.enabled=false` with `on_shutdown`. The service itself does not set a finalization deadline in this mode, but an orchestrator can still terminate the process before publication completes; configure its termination grace period for the expected workload and check the final artifact and exit status.

### `server.listen_address`

TCP listen address for the HTTP server.

Example: `":4318"`

### `server.grpc_listen_address`

Optional TCP listen address for the OTLP/gRPC server.

Example: `":4317"`

Leave empty to disable OTLP/gRPC.

### `server.max_request_bytes`

Maximum OTLP request body size in bytes after decompression.

Example: `5242880`

### `runtime.flush_interval`

How often Bering checks whether the active window should close.

Example: `5s`

Tradeoff: smaller values reduce `window_lag_seconds` but wake the service more often.

### `runtime.window_size`

Tumbling window size for discovery snapshots.

Example: `30s`

Tradeoff: smaller windows react faster to topology drift; larger windows produce steadier topology snapshots and reduce churn.

### `runtime.max_in_memory_spans`

Maximum number of spans retained in the active window before new spans are dropped.

Example: `10000`

Tradeoff: higher values improve coverage but increase peak memory.

In `on_shutdown` mode, this is an all-or-nothing limit. An overflow returns an ingest error, prevents further ingest, and prevents final publication.

### `runtime.late_span_policy`

How Bering handles spans whose event time falls before the active window start.

Allowed values:

- `drop`
- `current_window`

`drop` keeps window boundaries strict. `current_window` preserves more evidence at the cost of blur between scheduled windows.

### `runtime.reconciliation`

Controls evidence-weighted reconciliation for runtime publication.

Suggested fields:

- `enabled`: toggle the reconciliation module on or off
- `state_path`: JSON file used to persist reconciliation state between windows and restarts
- `report_path`: machine-readable report file for the latest reconciliation result
- `summary_path`: operator-facing Markdown summary for the latest reconciliation result
- `decay_half_life`: controls how quickly older evidence loses weight
- `minimum_opportunity_windows`: minimum opportunity evidence before misses can count
- `telemetry_health_freeze_threshold`: freeze or heavily damp retirement below this score
- `soft_gap_multiplier`: threshold for the stale transition
- `hard_gap_multiplier`: threshold for retirement
- `stable_core_min_belief`: minimum belief required for inclusion in `stable_core`
- `guardrail_union_min_belief`: minimum belief required for inclusion in `guardrail_union`
- `retired_ttl`: how long retired entities remain retained before eviction
- `max_retained_retired_entities`: cap on retained retired entities
- `compaction_interval`: cadence for state compaction and eviction checks

The defaults are expected to stay conservative and bounded so runtime mode does not become an unbounded historical store.

### `sink.directory`

Directory sink for one file per emitted snapshot.

Example: `out/snapshots`

### `sink.latest_path`

Optional stable path that is rewritten on every successful snapshot.

Example: `out/latest-snapshot.json`

### `sink.signal_quality_path`

Stable path for the latest signal quality and field coverage report.

Example: `out/latest-signal-quality.json`

Each timestamped snapshot also gets an adjacent `*.signal-quality.json` sidecar.

### `logging.structured`

Emit JSON logs instead of text logs.

Example: `true`

### `overlays`

Ordered list of overlay files to apply after inferred discovery.

Example:

```yaml
overlays:
  - configs/discovery.overlay.sample.yaml
```

## Flag overrides

`bering serve` supports these override flags:

- `--config`
- `--listen`
- `--grpc-listen`
- `--publication-mode windowed|on_shutdown`
- `--flush-interval`
- `--window-size`
- `--max-in-memory-spans`
- `--late-span-policy`
- `--reconciliation-enabled`
- `--reconciliation-state-path`
- `--reconciliation-report-path`
- `--reconciliation-summary-path`
- `--sink-dir`
- `--latest-path`
- `--signal-quality-path`
- `--log-format text|json`
- `--overlay` (repeatable)
