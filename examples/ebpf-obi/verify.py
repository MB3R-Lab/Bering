#!/usr/bin/env python3
"""Validate the generic OBI live-ingest and raw-replay acceptance artifacts."""

from __future__ import annotations

import json
import sys
from pathlib import Path


EXPECTED_SERVICES = {"edge-api", "lookup-api"}
EXPECTED_EDGE = ("edge-api", "lookup-api")
EXPECTED_ENDPOINTS = {
    ("edge-api", "GET", "/call"),
    ("lookup-api", "GET", "/lookup"),
}


EVIDENCE_KEY = "bering.observation_evidence.v1"


def load_snapshot(path: Path) -> tuple[dict, dict, list[dict]]:
    document = json.loads(path.read_text(encoding="utf-8"))
    return document, document.get("model", document), document.get("sources", [])


def projection(model: dict) -> dict[str, set[tuple[str, ...]] | set[str]]:
    return {
        "services": {item["id"] for item in model.get("services", [])},
        "edges": {(item["from"], item["to"]) for item in model.get("edges", [])},
        "edge_ids": {item["id"] for item in model.get("edges", [])},
        "endpoints": {
            (item["entry_service"], item["method"], item["path"])
            for item in model.get("endpoints", [])
        },
    }


def require_expected(label: str, graph: dict) -> None:
    missing_services = EXPECTED_SERVICES - graph["services"]
    missing_endpoints = EXPECTED_ENDPOINTS - graph["endpoints"]
    if missing_services or EXPECTED_EDGE not in graph["edges"] or missing_endpoints:
        raise SystemExit(
            f"{label} graph incomplete: missing_services={sorted(missing_services)} "
            f"edge_present={EXPECTED_EDGE in graph['edges']} "
            f"missing_endpoints={sorted(missing_endpoints)} graph={graph}"
        )


def require_obi_source(label: str, sources: list[dict]) -> None:
    if not any(
        item.get("type") == "ebpf" and item.get("connector") == "obi"
        for item in sources
    ):
        raise SystemExit(f"{label} lacks declared OBI eBPF provenance: {sources}")


def require_execution_link(label: str, snapshot: dict) -> None:
    for edge in snapshot.get("discovery", {}).get("edges", []):
        if (edge.get("from"), edge.get("to")) != EXPECTED_EDGE:
            continue
        encoded = (
            edge.get("metadata", {}).get("attributes", {}).get(EVIDENCE_KEY, "")
        )
        try:
            evidence = json.loads(encoded)
        except (TypeError, json.JSONDecodeError):
            continue
        if evidence.get("relation", {}).get("kind") == "execution_link":
            return
    raise SystemExit(f"{label} expected edge lacks trace-parent execution evidence")


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit("usage: verify.py LIVE_SNAPSHOT REPLAY_SNAPSHOT")
    live_snapshot, live_model, live_sources = load_snapshot(Path(sys.argv[1]))
    replay_snapshot, replay_model, replay_sources = load_snapshot(Path(sys.argv[2]))
    live = projection(live_model)
    replay = projection(replay_model)
    require_expected("live", live)
    require_expected("replay", replay)
    require_obi_source("live", live_sources)
    require_obi_source("replay", replay_sources)
    live_counts = {Path(item["ref"]).name: item["observations"] for item in live_sources}
    replay_counts = {Path(item["ref"]).name: item["observations"] for item in replay_sources}
    if live_counts != replay_counts:
        raise SystemExit(f"live/replay source count mismatch: {live_counts} != {replay_counts}")
    require_execution_link("live", live_snapshot)
    require_execution_link("replay", replay_snapshot)
    if live != replay:
        raise SystemExit(f"live/replay graph mismatch: live={live} replay={replay}")
    print(json.dumps({key: sorted(value) for key, value in live.items()}, sort_keys=True))


if __name__ == "__main__":
    main()
