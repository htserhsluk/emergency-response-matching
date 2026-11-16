# Implementation status

## Day 1 foundation

- Go contracts, bounded JSON validation and stable ID/shard derivation.
- API/coordinator/storage-agent HTTP path, persistent requests and guarded status updates.
- Transactional operation replay/conflict and initial outbox records; no broker consumer yet.
- Database-aware heartbeats and stale/retired boot exclusion; route-aware readiness.
- First primary/standby pair on separate volumes, synchronous replay, tested ambiguous commit/replay behavior and read-only standby.
- Container builds and Kubernetes Deployment/Service health smoke.
- Unit, race and real PostgreSQL integration suites; local-only reference/secret exclusions.

Subsequent milestones: resources and all three replicated shards; promotion/rejoin; RabbitMQ, matching and reconciliation; complete Kubernetes deployment and failure/scaling tests; browser/map/inbox extensions; final acceptance and study material.

The bootstrap Kubernetes smoke is not full-system deployment. One configured shard is not three-shard distribution. Requests remain PENDING until the matching increment exists. No throughput or production readiness claim is made.

## Day 2 storage and messaging foundations

Implemented: resource registration/status/list/spatial search across all three shards; persistent versioned routing; expiring write gates and PostgreSQL primary checks; controlled Compose and Kubernetes promotion/rejoin with preserved old data; durable reservations and assignment intents; authenticated typed internal CRUD/commands; manual match scheduling; transactional outbox claims, confirmed mandatory RabbitMQ publication and diagnostic consumer lookup/search/requeue.

Matching completion, cross-shard reconciliation, bounded business retries/dead-letter processing, worker scaling, browser/map/inbox and final acceptance remain later milestones. The diagnostic consumer does not allocate or acknowledge completion.

Decisions: each publication owns a bounded connection/channel with library recovery disabled, preventing ambiguous return/confirm reuse at the cost of connection overhead. The diagnostic search uses 50 km solely to exercise candidate lookup; the actual Day 3 matching strategy remains the planned 5 km then 10 km. Controlled promotion permits explicit degraded writes only after the old primary is fenced; restoring synchronous redundancy is part of recovery.

## Day 3 assignment and recovery

Implemented: cross-shard type/distance matching (5 km then 10 km), coordinator request serialization, candidate validation, durable intent/reserve/finalize and confirmed rejection handling, bounded reconciliation, persistent event/retry/pause/re-drive state, real worker manual acknowledgements, confirmed malformed-message DLQ publication, reconnect backoff and bounded shutdown. Migration 00005 adds processed_events; applied migrations remain immutable.

Review corrections: reserve a two-second assignment budget within the five-second operation so healthy request storage can record retry/backoff after a stalled dependency; prevent internal commands from forcing finalization/rejection/unmatched; validate type/distance before intent acceptance and reservation; require a fresh durable intent barrier for direct coordinator reservation commands.

The older Day 1/2 sections describe historical boundaries. Remaining work is the final-topology failure matrix, browser/map/durable inbox, acceptance/load/clean-start evidence and study material. Deferred review polish: consumer-session readiness, public distance projection and bounded worker/reconciliation diagnostics.

## Day 4 browser and durable inbox

Implemented: embedded browser forms and status/resource views; Leaflet coordinate selection and saved assignment map/distance with a text fallback; transactional assignment notifications, request-scoped list/read endpoints and persisted read state. Migration 00006 backfills historical finalized assignments. Worker readiness now tracks the owned consumer session, and worker/reconciliation failures emit bounded diagnostics.

The final-topology failure matrix, public/browser workflows, inbox promotion/rebuild and worker scaling/recovery checks passed on the final local deployment. A host FUSE flush stalled the old rebuild script; scoped journal/directory flushes passed the regression and real rebuild after runtime recovery. All three pairs are synchronous. Day 5 retains full acceptance, measured load, clean-start and release evidence; this milestone is not the final release.

## Day 5 acceptance and release candidate

Completed local acceptance, a 30-request/five-client measured workload, a new independent cluster bootstrap through schema 6, actual RabbitMQ backlog/drain, worker recovery/scaling, inbox persistence and separately timed recovery of all three shard pairs. Added the complete requirements/evidence map, architecture, API reference and operator/rehearsal runbook. See [release evidence](evidence-index.md) for results and precise limits.

Fresh deployment exposed cross-database recovery-journal collisions; journals now separate database identities and validate before fencing. Repeated tests exposed wall-clock and fixed-cancellation assumptions in fixtures; those now distinguish future scheduling, actual replay waits and eventual durable retry completion. Production acknowledgement and reservation invariants remain unchanged. Final build/vet/race, full replica suite, ten focused repetitions, SQL/broker race and browser/workload/operator regressions passed locally.

The original deployment is restored with two Matching workers and three synchronous pairs. Fresh rehearsal and test volumes are preserved; the fresh cluster and isolated test services are stopped. GitHub checks apply to the exact pushed PR head; local evidence does not substitute for their result. Main merge remains user-controlled. Course delivery is repository plus documentation; the students' study/explanation session is separate.
