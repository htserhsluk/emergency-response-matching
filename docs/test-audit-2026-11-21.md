# Emergency Response Matching test audit

21 November 2026. Tested application revision `9640025` on local Minikube (`erm-lab`), a separate Docker PostgreSQL/PostGIS primary and synchronous standby, and GitHub Actions on Linux. The test branch adds acceptance tests and this report. The 60 new live HTTP cases and 24 new history cases passed. Three issues surfaced in the original run. Their fixes and verification results are recorded below. Counts below describe separate suites with some overlapping scenarios.

## Results and coverage

| Assignment section | What was exercised | Evidence from this run |
|---|---|---|
| 1. Objective and example workflow | Create, retrieve, register, match and update. All five assistance types. Asynchronous assignment with matching type, availability and distance. | All five end-to-end lifecycle cases passed. Cases include the equator, both poles and the international date line. |
| 2. Configuration | One coordinator, API and Matching services, one RabbitMQ broker, three primary/standby pairs, HTTP and browser clients. | Inspected running pods and Services. Used the deployed public API, the live browser and cross-shard SQL/broker tests. |
| 3. Data | IDs, type, coordinates, request details, status, creation/registration times, resource availability. Empty/missing, invalid and boundary inputs. | Live contract suite passed. A 2,000-character Unicode description survived storage/retrieval and 2,001 was rejected. Tested zero, negative and out-of-range coordinates, omitted or mistyped fields, idempotent replay and conflicting replay. |
| 4. Distributed storage | Transparent ID routing, distribution across all shards, cross-shard lookup, nearest candidate selection and incomplete searches. | Hash/routing unit tests, three-shard integration fixtures, deployed recovery and final SQL inspection. |
| 5. Client REST | POST/GET requests, PUT request status, POST/GET resources, PUT resource status, POST match. JSON, both success codes and 400/404/409/503 paths. | Deployed seven-route public acceptance passed. New live tests validated response codes and JSON for invalid input, missing records, conflicts and operation-key boundaries. |
| 6–7. Internal APIs and request processing | Node registration/heartbeat, protected internal CRUD, atomic request/outbox, publish confirmation, consume, reserve, update states, durable redelivery. | CI broker/SQL suites passed. Live queue had at least three ready messages with workers stopped. After resumption the rehearsal completed 18 distinct assignments. Covered unroutable publication, unknown-schema dead lettering, shutdown, concurrent claims and ambiguous reservation recovery. |
| 8. Storage failure and recovery | Detect/fence failed primary, promote synchronized replica, write during outage, retain route over coordinator restart, rebuild failed node and resume synchronization. | Live shard 1 promotion/rejoin passed. Separate shard 0 promotion/rejoin retained an inbox read. The dedicated SQL/replica suite passed 49 top-level tests in 70.046 seconds. See the failure-matrix finding below. |
| 9. Docker and Kubernetes | Containers communicating through Services, StatefulSets, Deployments, worker scaling and pod replacement. | Live matching test scaled from one worker to two, compared pod identities after deletion, continued matching and survived broker and coordinator restarts. Returned to one worker afterward. Built all five local images from the Dockerfiles (API, coordinator, matching, storage agent and migration runner). |
| 10. Required demonstration | Create/retrieve, register, match, shard distribution, multiple queued submissions, storage failure/recovery, pod recovery and scaling. | Exercised using the public API, matching, recovery and inbox Kubernetes rehearsals and direct shard inspection. A live course presentation remains a team deliverable. |
| 11. Normal, concurrent and failure tests | All ten categories in the assignment, including contention, replica recovery and Kubernetes scaling. | Five concurrent HTTP clients submitted 30 requests. All 30 received distinct resources with zero errors. Observed assignment p95 was 8.094 s including polling. This was a functional workload run. |
| 12. Design justification | Checked actual routing, replication, queue behavior, service boundaries and deployment against the documented design. | See [architecture](architecture.md). Student explanations are assessed during presentation. Cloud and design patterns are optional. |

### Additional project features

| Feature | Test evidence |
|---|---|
| Expanding radius and automatic retries | SQL integration covered exact-location search, distance boundaries and ties, 4,000 km cap, exhausted search, automatic matching after registration or availability changes, nearest waiting request, persistent progress, stale generations, restarts and legacy intents. A fresh live request remained unmatched with supply 4,000,001 m away, previewed the waiting request from an unavailable resource 3,999,999 m away, then automatically assigned it when enabled. The saved distance agreed within 0.1 m. |
| Map selection and pins | Overview, all/individual types, selected request/resource, five nearest candidates, confirmed pair, late refreshes, zoom/resize, hover labels and anchored tips. Browser tests also cover unavailable tiles and geolocation outcomes. Permission and GPS outcomes used simulated inputs. |
| Shared map and directory dialog | Real browser checked identical evidence from both entry points, keyboard close/focus restoration, mouse controls, unallocated and historical rows, saved distance, both locations, and Retry after a lookup error. A separate read-only browser test used an actual deployed allocation. |
| Resource directory and latest 50 | Verified name/ID rows, availability actions, paging and newest-first views. Live tests compared seven-row pages with a separately constructed order from individual resource lookups. SQL integration tested 62 allocations across three shards and persistence outside the window. Added 24 cases for 0/1/49/50/51/101 records and page sizes 1/7/50/100, duplicate cross-shard evidence, equal timestamps and both arrival orders. All passed. |
| Shared inbox, completion and recovery | Live test verified one notification, stable read timestamp across API restart, shard promotion/rebuild and rematch. Completed requests retained their allocation. |
| Additional recovery safeguards | Dedicated replica tests exercised paused replay, uncertain commits and retry barriers. CI exercised the recovery journal and scoped flush regression. |

## Original findings

### 1. Failure-matrix script looks for a journal in the old directory

**Impact.** The fault-injection rehearsal can stop after fencing a primary. The normal recovery helper stores new journals under a database-specific directory. The test reads the former flat path at line 52 of [test-k8s-failure-matrix.ps1](../scripts/test-k8s-failure-matrix.ps1). A pre-existing matching legacy journal can conceal the defect.

**Reproduction.** In an empty temporary directory call `Get-KubeJournalPath` with database identity 123456789, shard 0, side a and route version 1. Write a fixture at the returned path. It exists under `system-123456789/k8s-0-a-v1.json`. The test's lookup of `k8s-0-a-v1.json` at the root returns false. [Journal helper](../scripts/recovery-journal.ps1)

**Live result.** Standby loss refused an unconfirmed write, an explicit degraded write later synchronized, and loss of both copies returned 503 while a different shard stayed readable. The matrix then confirmed stale promotion was refused. After replay resumed, it failed on the missing flat journal for shard 0, side b, route version 2. This interrupted the test before its final history assertions. The existing operator commands completed promotion and rejoin. The separate shard 1 rehearsal independently verified pre-outage and outage-period data after rejoin.

### 2. Two browser tests intermittently time out during Chromium startup in CI

**Observed.** The directory keyboard/mouse test and shared map/directory comparison both timed out waiting for the browser profile's `DevToolsActivePort` file. The tests never reached their dialog assertions on that attempt. Expected outcome is a browser connection followed by the interaction checks. Local browser run passed 32/32 including the deployed comparison. The unchanged CI retry passed 31/31 checked-in frontend tests and the remaining stages. The reason Chromium startup missed the deadline is unconfirmed. [Failed attempt](https://github.com/htserhsluk/emergency-response-matching/actions/runs/44887678925/attempts/2) · [Passing retry](https://github.com/htserhsluk/emergency-response-matching/actions/runs/44887678925/attempts/3)

### 3. Existing local API port-forward dropped during live testing

**Observed.** Using localhost port 18080, one case passed, one request lost the HTTP connection, and the remaining 54 cases encountered connection refusal. The kubectl log ended with a connection reset and “lost connection to pod.” The API pod was still ready with zero restarts at inspection. All 56 initial cases passed over the Kubernetes Service proxy. The expanded 59-case suite also passed there. The precise cause of the original disconnect remains open.

**Reproduction path.** Run the live suite through a kubectl port-forward to the API, then repeat through the Service proxy. Compare the forward's stderr, API pod restarts and readiness before attributing an HTTP failure to an endpoint.

## Deployment state immediately after the original audit

The API reported ready. API, coordinator and Matching each had one available replica. All six storage pods were checked. Each primary reported synchronous streaming and its standby returned identical row counts.

| Shard | Primary | Requests | Resources | Allocated |
|---|---|---:|---:|---:|
| 0 | 0a | 30 | 35 | 29 |
| 1 | 1b | 28 | 40 | 30 |
| 2 | 2a | 22 | 30 | 21 |

The two isolated Docker test databases were stopped. Test records remain in the local deployment. The GitHub rerun used the existing application commit and passed its build, vet, race, broker and SQL stages. The newly added suites ran locally against that application. At the end of the original audit the advanced failure-matrix test was still failing on the journal lookup.

One intermediate version of the new test fixture accidentally encoded a single Unicode character as three during editing on Windows. Five nominal 2,000-character requests correctly received 400. The fixture now uses an explicit Unicode escape. All 60 final cases passed in 14.534 seconds. The test also follows the server-confirmed resource when repeated runs leave equally close resources at identical coordinates.

## Fix verification on the audit branch

| Original finding | Change | Observed result |
|---|---|---|
| Recovery journal lookup | The failure matrix resolves the journal using the standby database identity and the same path helper used during failover. | The full Minikube matrix passed, including stale promotion refusal, catch-up, promotion, rejoin and checks on the original and outage-period records. All three primaries subsequently reported synchronous streaming. |
| Chromium startup timeouts | The shared browser fixture waits for the DevTools listening announcement, checks for a usable page and reports launch errors or early exits. CI selects Google Chrome explicitly and runs the frontend test files sequentially. | All 31 tests passed three times on Windows and three times in a local Linux Chromium container, with zero skips. The revised branch has not run on a hosted GitHub runner. |
| Dashboard connection lost | The local dashboard connects over loopback SSH to the Kubernetes Service IP using the Minikube Docker node's verified host key. The listener survives API pod replacement. | Replaced the running API pod. The existing listener served all 60 HTTP acceptance cases afterward (27.302 s). A separate real-browser check through the same listener opened the deployed dashboard and compared map and directory details. API readiness passed. |

The live verification added test records and left shard 0 on primary 0b with a synchronous standby. The dashboard launcher is described in the [runbook](runbook.md).

## Repeat the tests

Use an isolated test deployment for write scenarios. The lifecycle and workload fixtures create real records and retain allocation history. The live directory comparisons assume records stay stable during each comparison. The storage drills change primary roles and briefly interrupt components.

- **60 HTTP cases** — Set `ERM_AUDIT_URL` to the test API base URL, then run `python -B -m unittest discover -s tests/acceptance -p test_live_contract.py -v`. For the Service proxy, start `kubectl --context erm-lab proxy --port=28081` and use `http://127.0.0.1:28081/api/v1/namespaces/erm/services/api:8080/proxy`.
- **24 history cases** — Run `go test ./internal/contracts -run TestAuditAllocationHistoryPageBoundaries -count=1 -v`.
- **Browser and CI suites** — Run `node --test internal/web/*_test.cjs` with Chrome or Edge installed. The [CI workflow](../.github/workflows) lists the build, vet, race, broker, migration and integration checks. The additional deployed comparison used the local live browser fixture.
- **Replica and Kubernetes tests** — Use the [runbook](runbook.md) for isolated replica database setup and the public API, matching, recovery and inbox rehearsals. The concurrency command was `python scripts/workload.py --count 30 --concurrency 5` with the Service proxy as its base URL.

## Final branch recheck

On 21 November the repaired localhost connection passed all 60 live HTTP cases. All 31 frontend tests ran without skips, and a separate browser check compared a stored allocation from its map pin and directory row. The Go build, vet, Linux race suite, contract tests, four workload-harness tests and recovery-journal regression passed. A fresh run with PostgreSQL, RabbitMQ and a synchronous standby passed all 54 top-level integration tests in 84.868 seconds. An initial test-runner readiness probe reached PostgreSQL during startup. Waiting for readiness allowed the suite to run. The isolated test services were stopped afterward.

Five concurrent clients submitted 30 more requests through the repaired tunnel. All 30 received distinct resources, with zero errors. The API finished ready. All three storage pairs reported synchronous streaming and matching primary/standby counts for requests, resources and allocations. The local deployment retained the test records. For disruptive pod replacement and failover, this recheck used the completed fix-verification drills above. The local changes remain pending hosted GitHub CI.
