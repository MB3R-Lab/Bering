# Sheaft v1 Compatibility Fixtures

This directory is Bering's canonical fixture checkpoint for Sheaft `v1.2.0` stochastic-connectivity and failure-tolerance compatibility.

Files:

- `manifest.json`: producer/consumer checkpoint metadata, schema-line pins, and fixture hashes
- `bering-model.v1.sample.json`: standalone model artifact
- `bering-snapshot.v1.sample.json`: snapshot envelope with the same model embedded

The fixtures exercise the Bering `1.3.0` contract fields Sheaft v1 consumes:

- replica counts and placement buckets
- operation-aware edge IDs and identity metadata
- service, placement, and edge reliability evidence
- observed edge latency/error summaries
- resilience policy and policy scope
- endpoint `success_predicate_ref`
- immediate-response and eventual-completion endpoint semantic hints

Sheaft `v1.2.0` can sweep independent replica-failure probability and exact failed replica-slot counts directly from these existing replica, placement, reliability, edge, and endpoint fields. Bering does not need a new schema contract for fail-stop sweeps. Capacity, queue, routing, and overload-feedback inputs remain outside this checkpoint and require a future additive contract before Sheaft can model them honestly.

They are generated from the checked-in topology API sample and are included in the Bering contracts pack under `fixtures/sheaft-v1/`.
