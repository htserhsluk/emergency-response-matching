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
