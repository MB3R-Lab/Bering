#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="$ROOT/out/ebpf-obi"
OBI_IMAGE='otel/ebpf-instrument@sha256:101b5c773ac84545cac007ce3dbaf38bfb0e7091f26e813c08d649f35dca253e'
OBI_CONTAINER='bering-ebpf-obi-smoke'

mkdir -p "$OUT/bin" "$OUT/logs"

bering_pid=''
edge_pid=''
lookup_pid=''
cleanup() {
  set +e
  if docker inspect "$OBI_CONTAINER" >/dev/null 2>&1; then
    docker logs "$OBI_CONTAINER" >"$OUT/logs/obi.log" 2>&1
    docker stop --time 15 "$OBI_CONTAINER" >/dev/null 2>&1
    docker rm -f "$OBI_CONTAINER" >/dev/null 2>&1
  fi
  if [[ -n "$bering_pid" ]]; then kill -TERM "$bering_pid" 2>/dev/null; wait "$bering_pid" 2>/dev/null; fi
  if [[ -n "$edge_pid" ]]; then kill -TERM "$edge_pid" 2>/dev/null; wait "$edge_pid" 2>/dev/null; fi
  if [[ -n "$lookup_pid" ]]; then kill -TERM "$lookup_pid" 2>/dev/null; wait "$lookup_pid" 2>/dev/null; fi
  return 0
}
trap 'smoke_exit_status=$?; trap - EXIT; cleanup; exit "$smoke_exit_status"' EXIT

if [[ -f "$ROOT/go.mod" && -d "$ROOT/cmd/bering" ]]; then
  (cd "$ROOT" && go build -o "$OUT/bin/bering" ./cmd/bering)
elif [[ -x "$ROOT/bering" ]]; then
  cp "$ROOT/bering" "$OUT/bin/bering"
else
  echo 'Run this example from a Bering source checkout or extracted Linux release archive.' >&2
  exit 1
fi
go build -o "$OUT/bin/edge-api" "$ROOT/examples/ebpf-obi/edge-api/main.go"
go build -o "$OUT/bin/lookup-api" "$ROOT/examples/ebpf-obi/lookup-api/main.go"

(cd "$ROOT" && "$OUT/bin/bering" serve --config examples/ebpf-obi/serve.yaml) >"$OUT/logs/bering.log" 2>&1 &
bering_pid=$!
OTEL_SERVICE_NAME=lookup-api "$OUT/bin/lookup-api" >"$OUT/logs/lookup-api.log" 2>&1 &
lookup_pid=$!
OTEL_SERVICE_NAME=edge-api "$OUT/bin/edge-api" >"$OUT/logs/edge-api.log" 2>&1 &
edge_pid=$!

for _ in $(seq 1 60); do
  if curl --fail --silent http://127.0.0.1:4318/readyz >/dev/null \
    && curl --fail --silent http://127.0.0.1:18081/healthz >/dev/null \
    && curl --fail --silent http://127.0.0.1:18082/healthz >/dev/null; then
    break
  fi
  sleep 1
done
curl --fail --silent http://127.0.0.1:4318/readyz >/dev/null
curl --fail --silent http://127.0.0.1:18081/healthz >/dev/null
curl --fail --silent http://127.0.0.1:18082/healthz >/dev/null

docker pull "$OBI_IMAGE"
docker image inspect "$OBI_IMAGE" >"$OUT/obi-image-inspect.json"
docker run --detach --name "$OBI_CONTAINER" --privileged --pid=host --network=host \
  --env OTEL_EBPF_OPEN_PORT=18081,18082 \
  --env OTEL_EBPF_BPF_CONTEXT_PROPAGATION=headers \
  --env OTEL_EBPF_BPF_TRACK_REQUEST_HEADERS=true \
  --env OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318/sources/obi-main \
  --env OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
  --env OTEL_EBPF_LOG_LEVEL=info \
  "$OBI_IMAGE" >"$OUT/obi-container-id.txt"

# Send a fixed traffic batch independent of the inferred graph. The following
# bounded wait is only for technical export readiness.
for _ in $(seq 1 30); do
  curl --fail --silent http://127.0.0.1:18081/call >/dev/null
  sleep 1
done
captured=false
for _ in $(seq 1 60); do
  if find "$OUT/obi-capture" -type f -print -quit 2>/dev/null | grep -q .; then
    captured=true
    break
  fi
  sleep 1
done
[[ "$captured" == true ]]

# Stop the producer first, then Bering; graceful shutdown forces the final
# runtime flush without introducing a span-age admission criterion.
docker stop --time 15 "$OBI_CONTAINER" >/dev/null
docker logs "$OBI_CONTAINER" >"$OUT/logs/obi.log" 2>&1
docker rm "$OBI_CONTAINER" >/dev/null
kill -TERM "$bering_pid"
wait "$bering_pid"
bering_pid=''

find "$OUT/obi-capture" -type f -print0 | sort -z | xargs -0 sha256sum >"$OUT/obi-capture.sha256"
test -s "$OUT/obi-capture.sha256"
"$OUT/bin/bering" discover \
  --input "$OUT/obi-capture" \
  --input-type ebpf-obi \
  --source-domain smoke \
  --out "$OUT/replay-model.json" \
  --snapshot-out "$OUT/replay-snapshot.json" \
  >"$OUT/logs/replay.log" 2>&1
python3 "$ROOT/examples/ebpf-obi/verify.py" "$OUT/latest-snapshot.json" "$OUT/replay-snapshot.json"

trap - EXIT
cleanup
