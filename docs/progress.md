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
