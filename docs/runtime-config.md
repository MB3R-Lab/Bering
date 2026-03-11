# Runtime Config

`bering serve` reads YAML or JSON config. Flags may override config values.

Sample file: [configs/serve.sample.yaml](../configs/serve.sample.yaml)

## Fields

### `server.listen_address`

TCP listen address for the HTTP server.

Example: `":4318"`

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

### `runtime.late_span_policy`

How Bering handles spans whose event time falls before the active window start.

Allowed values:

- `drop`
- `current_window`

`drop` keeps window boundaries strict. `current_window` preserves more evidence at the cost of blur between scheduled windows.

### `sink.directory`

Directory sink for one file per emitted snapshot.

Example: `out/snapshots`

### `sink.latest_path`

Optional stable path that is rewritten on every successful snapshot.

Example: `out/latest-snapshot.json`

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
- `--flush-interval`
- `--window-size`
- `--max-in-memory-spans`
- `--late-span-policy`
- `--sink-dir`
- `--latest-path`
- `--log-format text|json`
- `--overlay` (repeatable)
