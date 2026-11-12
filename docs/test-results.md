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

## Day 3 local verification (deployment proof recorded separately)

- Build, vet and default race suite passed after the review fixes.
- Real SQL tests passed for cross-shard assignment, two requests/one resource, duplicate/manual races, committed reservation with lost response, restart reconciliation, pause/redrive, stale generation and future-attempt rejection.
- Replica suite passed including fresh retry-ack and cross-shard intent barriers with actual replay paused; no acknowledgement/reservation escaped an unconfirmed transaction. Controlled promotion continues to refuse stale replicas rather than forcibly promoting them before catch-up.
- Real broker worker assigned across shards and confirmed unknown-schema DLQ publication. Shutdown left unfinished work available for redelivery. The deliberately stalled HTTP fixture initially hung during cleanup; a test-owned release channel fixed the fixture, and subsequent full normal and race-enabled broker suites passed.
- Review tests reproduced timeout starvation and unsafe internal transitions, then passed after fixes. Three stalled intents now persist backoff while a later healthy intent completes within the ten-second reconciliation pass. Wrong-type/out-of-radius candidates, reservation without intent, and direct finalization/rejection/unmatched commands are refused.

No final acceptance or production-readiness claim is made. Latest exact commands/results and deployed scaling evidence follow below when observed.

### Day 3 deployed checks — 11 November 2026

- Final real PostGIS/broker integration suite passed on Windows Go 1.27.3 using an isolated broker vhost. This includes explicit 5 km to 10 km expansion, wrong-type exclusion and exhausted-search UNMATCHED behavior.
- A host/database clock difference reproduced premature retry refusal. Workflow due checks and retry scheduling now use the database clock; the full integration suite then passed.
- Final images built and upgraded in place with schema5 and saved primary routes preserved. `scripts/test-k8s-matching.ps1` exited successfully: 18 unique assignments, matching scale 1 to 2, replacement worker UID, broker restart/reconnect, coordinator restart, and completion retaining resource allocation. Six storage pods, two matching workers, one API, one coordinator and one broker were healthy afterward.
- An earlier deployed run passed all assignment assertions but failed its final display command because PowerShell split an unquoted resource list. The command was corrected and the complete script rerun successfully.
- Windows vet passed. Restricted Windows unit checks could not access local sockets; those two tests passed when local access was enabled. Windows directory fsync is unsupported by the Linux control-file durability implementation; the full suite is verified on the Linux deployment platform instead.
- Post-check runtime snapshot: Minikube container used 1.369 GiB of its 3 GiB limit; the two additional Compose test databases used about 76 MiB combined. This is a point-in-time container measurement, not a load benchmark. Kubernetes per-pod metrics were unavailable. All three primaries reported streaming synchronous standbys; all three standby role checks returned true.
- Final `go test -race ./... -count=1` passed inside the pinned Linux Go 1.27.3 container after the clock fix. Remote CI additionally runs the real SQL/broker suite with the race detector.

## Day 4 verification — 12 November 2026

- All three shard pairs passed real Kubernetes promotion/rejoin, including interrupted rebuild and coordinator restart.
- The approved local failure matrix passed: standby loss refuses unconfirmed writes; explicit degraded writes catch up after standby return; loss of both copies returns bounded errors while another shard remains readable; a fenced, lagging standby is refused promotion until catch-up. No stale replica was forcibly promoted.
- All seven required public API operations passed through the deployed Service, including wrong-type/distant exclusion, unmatched/manual matching, pagination, completion, and invalid/missing inputs.
- Real SQL/broker integration passed after review fixes for bounded resource-location enrichment and consumer readiness during in-flight broker loss. The replica suite passed notification read/replay durability barriers.
- Browser checks passed real registration, assignment and completion; map coordinate selection; saved assignment distance/location; persisted inbox read state after reload; and a real assignment at negative coordinates with external tiles disabled.
- Repeated lookup initially created duplicate polling timers. The browser regression failed before the generation-token fix and passed afterward.
- A deployed inbox failover check reached promotion but its old rebuild script hung in global `sync`. The scoped-flush fix passed a disposable regression that rejects global flushes and checks preserved old data. After a Windows/runtime restart, the corrected rebuild restored synchronous replication. The full deployed inbox test then passed API restart, promotion, rebuild and duplicate-match checks.

### Final Day 4 deployment

- Go build and vet, Node polling regression, and Linux `go test -race ./... -count=1` passed on the final source. All service images were rebuilt, loaded and rolled out with schema 6, preserving saved routes.
- `scripts/test-k8s-inbox-recovery.ps1` passed: one notification and preserved read timestamp after API restart, promotion to node-0a (route version 7), and rebuild of node-0b. Browser lookup after recovery confirmed the saved read state.
- `scripts/test-k8s-matching.ps1` passed: 18 distinct resource allocations, one-to-two worker scaling, replacement worker UID, broker restart/reconnect, coordinator restart and completion protection.
- `scripts/test-k8s-public-api.ps1` passed all seven API paths. Its final cross-shard fixture and browser comparison are recorded in `tests/acceptance/browser-checklist.md`.
- Point-in-time memory with two workers: Docker erm-lab 1.755 GiB / 3 GiB; kubelet node working set 1798.9 MiB. Pod working sets: workers 5.2/7.2 MiB, API 10.5 MiB, coordinator 8.0 MiB, broker 110.9 MiB, six storage pods 26.2–56.0 MiB. These are snapshot observations, not peak/load measurements or a capacity guarantee. Day 5 measures the workload separately.
- Independent review findings were reproduced and fixed: slow resource-location enrichment must not hide request status; broker loss must clear readiness even during in-flight work; repeated browser lookup must not multiply polling loops. No remaining finding from that review is deferred.

### Day 4 audit follow-up

The additional audit observed one pause/redrive `UNAVAILABLE` response and an intermittent outbox test failure. Repeated isolated pause runs did not reproduce the original event, so its exact historical cause is not established. A deterministic future-retry regression did reproduce the same pause error: the execution due-time check also rejected an operator pause. Pause now bypasses only that scheduling check; normal delivery still refuses early execution, and pause/re-drive retains the same intent and durability transaction.

Outbox diagnostics separately showed that client cancellation can return before PostgreSQL finishes leaving its synchronous-replication wait. The test now observes its own backend's `SyncRep` state before cancellation, then waits for the locally committed claim to become visible before testing replay. It no longer assumes that 200 ms is enough to enter and leave those states. Fixture due/lease timestamps use a fixed past date. No durability guarantee or success assertion was relaxed.

The corrected pause and outbox tests passed twenty repetitions each. The clock-jump explanation considered during investigation was not confirmed by direct sampling and is not treated as an established root cause.
Final follow-up validation: Linux build/vet/race passed; full replica integration passed (31.561 s); full SQL/broker integration passed (22.759 s) when run separately from image builds. An intervening broker run was invalidated by a test PostgreSQL backend exit and crash recovery; that failed run is not counted as passing. Recovery completed with the existing volumes, and the subsequent isolated run passed.
