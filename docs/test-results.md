# Day 1 verification

Local checks run on 10 November 2026 (Asia/Calcutta), using versions recorded in `versions.md`.

| Check | Observed result |
|---|---|
| `go build ./cmd/...` | Passed for API, coordinator and storage agent |
| `go vet ./...` | Passed |
| `go test -race ./... -count=1` | Passed; contract, JSON, routing, registry and readiness tests |
| `bash scripts/test-replica.sh` | Passed: replay barrier, payload conflict barrier, concurrent create, canceled transaction cleanup and actual coordinator HTTP path |
| `scripts/smoke.ps1` | Passed: 201 create, GET, stable replay, 409 changed payload and 503 unconfigured shard |
| Primary restart then same smoke | Passed; original creation timestamp and record retained |
| `pg_stat_replication` | node-0b streaming with sync_state=sync |
| Standby role and data | pg_is_in_recovery=true; acknowledged request readable; direct mutation rejected |
| Paused replay | New/identical/conflicting retries do not claim confirmed outcomes; resume preserves one request/event |
| `scripts/k8s-smoke.ps1` | Passed: API image reached through actual Deployment and Service |

Independent review identified three regressions: unconfirmed conflict response, retired boot re-registration, and readiness ignoring route eligibility. Each was reproduced with a failing test, fixed and followed by passing tests. The integration path now includes the actual coordinator Router and heartbeat-driven exclusion. No remote CI result is inferred from local verification.

Scope: one primary/standby pair and request API foundation. This evidence does not demonstrate three-shard distribution, resource matching, broker processing, promotion/rejoin, complete Kubernetes deployment, worker scaling or final product acceptance. Those checks remain required in subsequent increments.

## Day 2 verification — 11 November 2026

- `go build ./cmd/...`, `go vet ./...`, `go test -race ./... -count=1`: passed.
- Real PostGIS/resource tests: create/replay/conflict, false availability/zero coordinates, protected status, cursor ordering, spatial candidates, all-shard failure handling, concurrent reservation winner, persistent rejection, durable intent and safe internal CRUD/command routing: passed.
- `scripts/test-broker.sh`: broker confirmation/unroutable tests and 16 integration tests passed, including consumer lookup/requeue and pending outbox during broker outage/unroutable publication.
- `scripts/test-replica.sh`: passed including outbox claim, reservation outcome and intent replay barriers while standby replay is paused.
- `scripts/test-recovery.ps1 -Shard 0 -From b`: passed; controlled Compose promotion, outage write, persisted coordinator routing, idempotent activation and synchronous rebuild.
- Initial `scripts/test-k8s-recovery.ps1 -Shard 0 -From a`: passed with six real storage pods/PVCs, promotion, coordinator restart and verified synchronized old node.
- Schema4 upgrade and new images deployed to the actual saved primaries without resetting promoted roles.
- Startup order and existing-cluster bootstrap refusal: reproduced failures, corrected and passed executable operator-script checks.
- Broker startup initially failed the default one-second diagnostic timeout; ten-second timeout applied, broker became ready with existing PVC preserved.
- Independent review: partial outage removed public API Service endpoints (reproduced connection refusal). API serving readiness now uses process health while `/health/ready` remains the all-shard diagnostic. `test-k8s-partial-availability.ps1` passed through the actual Service during a shard outage, then restored the original primary.

The full assignment worker and final acceptance tests remain later milestones. No remote CI outcome is inferred from these local results.
- Review recovery fix: `test-k8s-recovery.ps1 -Shard 0 -From b -TestInterruptedRejoin` passed. A real failed Job with a partial backup directory was replaced; rejoin preserved old data, restored streaming/synchronous mode and retained pre-outage/outage requests. Kubernetes shard0 now uses node-0a at route version3, node-0b standby.
- `test-k8s-messaging.ps1` passed: new requests were located on each expected primary and standby; outbox rows became confirmed/sent; a request remained durable while RabbitMQ was stopped and published after restart; the diagnostic consumer retrieved a request and available candidates, left it PENDING, and requeued its delivery. Final queue observation: 10 ready, 0 unacknowledged; DLQ 0. These are pending diagnostic workloads, not completed matches.
