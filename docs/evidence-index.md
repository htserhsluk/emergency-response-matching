# Release acceptance evidence

Recorded 15–16 November 2026, Asia/Calcutta. Day5 begins from merged Day 4 revision `8b28d88`, containing application fix `24ba6f8`. Historical checks are in [test-results.md](test-results.md); this page records fresh release checks separately. Requirements map: [requirements.md](requirements.md).

## Fresh results

| Check | Observed result |
|---|---|
| Linux build, vet, default race suite | PASS using pinned Go1.27.3 container, compiler concurrency2 |
| Browser polling regression | PASS |
| Workload report regression tests | 4 PASS; reject failed/missing allocations and duplicate ownership, verify percentile, accept intermediate ASSIGNING |
| Full paired-replica integration | Final corrected full suite PASS15.839s; initial failures and their fixture corrections are retained below |
| Corrected fixture repetitions | PASS82.768s: ten runs each of bounded reconciliation, concurrent contention, future-due intent recovery, cross-shard replay barrier and synchronous create/replay/conflict barrier |
| Race-enabled SQL/broker integration | Final corrected suite PASS22.531s, isolated `erm-tests` vhost; earlier baseline PASS21.840s |
| Seven deployed public APIs | PASS; radius expansion, wrong-type/far exclusion, unmatched/manual retry, completion protection, invalid400/missing404 |
| Worker/reconnect rehearsal | PASS18 unique allocations, 1→2 workers, replacement UID, broker restart, coordinator restart, completion protection |
| Storage failure matrix | PASS57.633s total, including standby-only/degraded recovery, bounded both-copy503/healthy-shard access, stale-promotion refusal/catch-up/promotion/rejoin |
| Three-shard distribution | Primary0b:63 requests/88 resources; primary1b:50/76; primary2b:51/73. All three paired standbys streaming:sync after rehearsal |
| Clean bootstrap | PASS in separate `erm-release` profile with new control/archive/broker/six data PVCs, migrations1–6 and all synchronous pairs; original `erm-lab` preserved stopped |
| Fresh deployed acceptance | Public API PASS7.126s; matching/scaling/reconnect PASS57.788s with actual queued messages and final ready=0/unacknowledged=0; inbox/restart/promotion/rebuild PASS after journal correction |
| Final three-pair recovery | PASS: shard0 63.315s, shard1 41.955s, shard2 55.045s; includes fencing/promotion, outage writes, coordinator restart, rejoin and data/synchronous-replay assertions. Shard0 additionally includes an intentionally failed rebuild Job and safe replacement |
| Fresh post-recovery smoke | PASS all seven public APIs; API1/coordinator1/matching2/broker1 ready, all six storage members ready; primaries0a/1b/2b with streaming:sync standbys |

The original replica run failed `TestAssignmentTimeoutPersistsBackoffWithoutStarvingReconciliation`: the healthy request remained ASSIGNING. Five isolated repetitions, a full rerun and twenty instrumented repetitions passed. A ten-suite repetition then reproduced the failure and captured its cause: persisted due times12:23:19.864–20.032 UTC and request creation12:23:19.970 were ahead of database `clock_timestamp()`12:23:17.054, with no parent deadline expiry. A backward VM/database wall-clock step made just-created work temporarily future-due. The scheduler correctly deferred it; the fixture incorrectly assumed its work stayed immediately runnable. That run also failed the contention fixture with unavailable/ASSIGNING states; its exact individual cause was not captured, so the same timing explanation there is an inference.

The timeout and contention fixtures now explicitly prepare initial attempts as due in the past, keeping all original backoff/ownership assertions. `TestFutureDueIntentDefersAndResumesSameReservation` separately exercises future-due refusal, no premature reservation, and later convergence to the same original resource. Production due gates, retry budgets and durability barriers are unchanged. This resolves the demonstrated timing assumption rather than suppressing unavailable responses. Large clock corrections can still delay scheduled work until database time catches up; stable host time remains an operating requirement. The first historical event was not directly instrumented and is not retroactively claimed as proven.

Fresh rehearsal exposed an independent operator-script defect: local recovery journal names lacked database identity and collided between original and fresh profiles. The wrong-system safeguard prevented promotion, but it occurred after fencing the original primary. The standby was verified still in recovery before restoring the pair. A failing regression reproduced the cross-database journal selection; the fix separates journals by PostgreSQL system identifier, retains matching legacy interrupted journals and validates identity before fencing. Journal/initial-bootstrap/refused-rebuild guard tests passed; the actual fresh inbox failover/rejoin then passed. No database volume or old journal was deleted.

A later repetition exposed the same scheduling dependency in the cross-shard replay-barrier fixture's post-resume phase. That case now prepares already-due workflow state before pausing replay, so its rejection and no-reservation assertions exercise unconfirmed WAL rather than possibly future scheduling. Its individual historical cause was not instrumented; the correction strengthens separation of the two conditions without weakening the durability assertions. One final topology-print command mistakenly passed two resource names as separate PowerShell arguments; the corrected quoted multi-resource command passed. All preceding recovery assertions had already passed, and the final public smoke was rerun.

Further repetition exposed two other fixture assumptions. `TestReplayRequiresSynchronousBarrier` canceled after300ms and sometimes observed zero locally committed operations, so it had not reached the ambiguity it intended to test. It now uses an isolated schema/application identity, waits for that backend's actual SyncRep state, cancels and observes backend settling before checking original/replayed/changed-payload operations. The contention test sometimes received durable retry acknowledgements under load and saw both requests still ASSIGNING; it now drives due-aware reconciliation to bounded convergence and still requires exactly one owner of the sole resource. Unexpected errors and missing convergence remain failures; no production timeout, retry, uniqueness or acknowledgement rule was relaxed.

Fresh database distribution after recovery was11/8/8 requests and16/15/14 resources across0/1/2; all paired replicas were streaming:sync. The separate original deployment was retained throughout. Rehearsal timing excludes initial image downloads/cluster bootstrap; recovery timings include the assertions and controller restart described above, not just database promotion downtime.

The fresh profile was stopped with its data intact. The original `erm-lab` was restored with primaries0b/1b/2b, all three standbys streaming:sync, two Matching workers and a passing public API smoke. There are no intended destructive cleanup steps in this handoff.

Initial workload instrumentation also rejected the valid intermediate ASSIGNING state. A focused regression failed with that state; the polling harness was corrected and its full unit suite passed before the measurement below. This was a harness defect, not evidence of a failed allocation. Test database host-port startup was rejected by Windows; tests instead used the same preserved volumes over Docker's private network, without published ports. A local shell runner's CRLF issue was corrected before tests ran. Broker startup commands initially ran before readiness; the subsequent actual race suite completed successfully.

## Measured workload

Command: `python scripts/workload.py --count 30 --concurrency 5`. Five closed-loop HTTP clients create then poll, 250ms interval, no retry masking. Setup creates 30 MEDICINE resources at (-20,30), outside the timed interval. Observed 15 November at12:04:51 UTC.

| Measurement | Result |
|---|---|
| Attempts / assignments / unique resource owners / errors | 30 / 30 / 30 / 0 |
| Total workload interval | 14.403 seconds |
| Create latency p50 / p95 / max | 13.302 / 26.873 / 79.640 ms |
| Assignment observed p50 / p95 / max | 2390.860 / 4325.225 / 5601.099 ms |
| Host | Windows11, Ryzen7 7735HS, 8 cores/16 logical processors, approximately16GB RAM |
| Local Kubernetes allocation | 2 CPUs, 3GiB; two Matching workers |
| Cluster memory snapshot after workload | 1.541GiB /3GiB (not peak memory) |

Percentiles use nearest rank. Assignment times include HTTP/polling overhead. The fixture retains real records and writes per-attempt IDs/keys/timings to an ignored JSON file. These results do not establish maximum throughput, tail guarantees, multi-host HA or production sizing. Queue backlog and recovery are separate rehearsal checks, not inferred from latency.

## Application image identities

Application source remains Day 4 `24ba6f8`; Day5 changes acceptance tooling, diagnostic test output and documentation. Existing application images were built sequentially from that source with bounded compiler concurrency; the fresh install uses these exact artifacts rather than rebuilding unmodified application code.

| Local tag | Image ID (SHA256) |
|---|---|
| erm-api:day4 | 8bc83ca10297b6826dee2be87ef488b817b100320349eda73d9486f22302f4b6 |
| erm-coordinator:day4 | b80fce7745af591e59cdc4cc986bb9203fcd2aecbed9871f926b7d3cd43636ab |
| erm-storage-node:day4 | fd26ae25adb83a4b2dacac749ba7a6c41c61a1311676748ca0d29c486710e4c6 |
| erm-matching:day4 | 0ee1745350d1b9375042d4831dd2fd536e4fb51b8e803d432ad23e299ac85b2c |
| erm-migrate:day4 | 5108f5c344dd13df40107af055b82d415c03b45837ea51de9254704262a40e96 |

These are local image IDs, not registry release digests. Pinned base images/dependencies are in source manifests and [versions.md](versions.md). A course-specific package and students' explanation/live presentation are not automatically satisfied by this index.

## Expanded matching and interface revision — 18–19 November 2026

This revision implements the requested policy and usability changes described in [expanded matching](expanded-matching.md). Earlier milestones above remain historical evidence, not claims that the new policy still stops at 10 km.

| Check | Observed result |
|---|---|
| Build, vet, default Go race suite | Passed on Linux with the pinned Go toolchain |
| Full SQL and paired-replica integration | Passed; includes search-progress synchronous replay barrier, recovery, contention and uncertain reservations |
| Broker tests and SQL/broker race integration | Passed; integration race run completed in 34.442 seconds |
| Browser logic regressions | 15 passed, covering stale responses, filters/paging, created-resource updates, map input/click synchronization, initial pins, polling, tile failure, resize and simulated geolocation outcomes |
| Workload harness and recovery-journal checks | Four Python tests and PowerShell journal regression passed |
| Deployed seven-API acceptance | Passed on upgraded schema 7 deployment, including exact-location assignment and completion/history protection |
| Deployed queue/scaling/restart rehearsal | 18 unique assignments; observed backlog at least 3 with workers stopped, then zero ready/unacknowledged; 1-to-2 workers, worker replacement, broker and coordinator restart passed |
| Deployed inbox recovery | Read state survived API restart, shard0 promotion and standby rebuild; paired synchronous mode restored |
| Live UI workflow | Registered MEDICINE at (-64,43), requested it at (-65,43), assigned at an observed 111,485 m, marked notification read and request completed; resource stayed allocated and absent from unallocated directory |
| Desktop/mobile and tile-free layout | Initial selection present;390 px viewport had 375 px client/scroll width with no horizontal overflow; distinct overlapping selection/request pins and assigned-resource pin verified |

Before additive migration 7, SQL backups and per-shard count/history fingerprints were saved privately. Immediately after upgrade, before new test fixtures, requests remained 66/53/54 and resources 88/78/76; all three allocated-history fingerprints were identical. No prior resource or allocation was reset. Subsequent acceptance fixtures intentionally add records and retain them. All three replicas were streaming synchronously after recovery.

The original refresh was HTTP 200 with 20 records in deterministic ID order: missing feedback, rather than a failed request, caused the confusion. Baseline map inspection confirmed no initial selection marker. Live checking caught a further coordinate-input event gap; a regression first failed with the old pin coordinates, then passed after immediate input synchronization. The deployed fix was checked with selection and request at the same position and resource 111 km away.

Geolocation success, denial, timeout, unsupported browser, inaccurate results and stale callbacks are exercised with controlled browser-boundary test doubles. Physical GPS accuracy and every browser's permission-dialog appearance are not claimed. These are local functional/recovery checks, not production HA, optimal routing, emergency triage or throughput guarantees. Windows restricted-shell coordinator fsync/readiness checks failed due to access restrictions; the unchanged durability logic passed the Linux full suite. Initial isolated standby permissions and an invalid broker host port were corrected before the successful suites.

The existing `erm-lab` data and separate stopped `erm-release` profile are retained. Application images now use the `expanded` tag; schema 7 is additive and rejects destructive rollback. Main merging remains user-controlled.

## Type-filtered map and automatic matching, 20 November 2026

The schema 9 revision uses the `map-v9` images. A local upgrade from schema 7 retained all three storage pairs, with each standby reporting synchronous streaming. This deployment had zero requests and resources immediately before migration. The isolated migration tests cover historical cursors, prior ownership and uncertain reservations.

The deployed API accepted one MEDICINE request while its nearby resource was unavailable. The request exhausted the 4,000 km band. The type-filtered map returned both locations and the resource preview returned the waiting request. Changing availability started generation 2 automatically. The confirmed assignment distance was 1,106.30 m. The resource directory returned its registration timestamp. The verification retained this pair in the demo database.

All 21 dashboard and map logic tests passed. A headless Edge regression measured five marker tips at three zoom and viewport combinations, all within one CSS pixel of the Leaflet projection. The Linux build, vet and default race checks passed. SQL/broker race integration passed in 75.984 seconds, and the paired-replica suite passed in 69.895 seconds. Coverage includes type-filtered pagination, newest-first ordering, six competing waiting requests, and the closest request outside the first reconciliation batch. Native Windows checks also encountered a directory-sync permission failure and a broker deadline test timeout. The Linux suites used the project toolchain and isolated database and broker instances.

## Focused map, resource details and allocation history

On 21 November 2026 the schema 10 local rollout preserved request and resource counts and allocation associations on all three shards. Each standby reported synchronous streaming. A read-only Edge check on the deployed application verified the eight-record overview, a confirmed two-pin match, returning to All locations, and opening and dismissing a persisted allocation’s details with the keyboard.

All 27 dashboard and browser tests passed, including real Edge tests for type filtering, individual views, hover labels, full popups, pin alignment, refresh stability, availability controls, Escape and focus restoration. Linux build, vet, default race tests and SQL/broker race integration passed. The full integration suite with a synchronous standby also passed. A 62-allocation fixture distributed across three shards exercised reversed registration and allocation ordering, pagination through the latest 50 and retention of all 62. The migration tests covered older records with confirmation evidence on another shard and undated allocations.
