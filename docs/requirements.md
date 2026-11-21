# Requirements and acceptance map

This index covers the assignment's twelve sections as R01–R15. The private assignment and planning files are deliberately not part of the repository. A test name below is a traceability link, not a claim that every invocation passed. See [test results](test-results.md) and [release evidence](evidence-index.md) for dated outcomes and limitations.

| ID | Obligation | Implementation and evidence |
|---|---|---|
| R01 | Create requests/resources, type/location matching, status, retrieval, asynchronous processing | `internal/httpapp`, `internal/coordinator/workflow.go`; `scripts/test-k8s-public-api.ps1`, `test-k8s-matching.ps1` |
| R02 | One coordinator, 2–3 business services, one broker, three shards with replicas, client | API and Matching are the two business services; coordinator/storage agents are storage infrastructure. `infra/k8s/{apps,matching,broker,storage}.yaml`; browser and HTTP scripts; live topology in release evidence |
| R03 | Unique request ID, type, location, details, status, creation time | `internal/contracts/types.go`, migration1, `TestFullHTTPStoragePath`, `TestCreateReplayAndConflict` |
| R04 | Resource ID, type, location, availability, status | `internal/contracts/resources.go`, migration2; `TestResourceTransitions`, public API acceptance. Assistance can represent a volunteer/NGO providing one supported resource type |
| R05 | Both entities sharded; coordinator routing transparent to clients | `internal/routing/hash.go`, coordinator router, `TestCrossShardNearestAndIncompleteSearch`, hash tests; SQL distribution counts in release evidence |
| R06 | Seven exact REST paths, JSON, success/error statuses | [API](api.md); HTTP validation tests and deployed public acceptance |
| R07 | Internal storage interfaces, RabbitMQ connects request and matching services | `internal/httpapp/{data,commands,workflow}.go`; `TestTypedInternalCRUDProtectsHistory`, `TestCommandHTTPPathPreservesIntentAndDelivery`; broker integration |
| R08 | Save, publish, consume, find resource, assign, update states | Durable outbox and confirmed publish; worker/coordinator assignment; `TestWorkerAssignsAcrossShardsAndUnknownSchemaToDLQ`, `TestTwoRequestsOneResourceAndDuplicateManualRace`, real queue/restart rehearsal |
| R09 | Heartbeats, failed-route exclusion, replica availability, continued service, recovered synchronization | Registry/node gate; `TestHeartbeatExpiryAndDatabaseRole`, replay-barrier tests, `test-k8s-recovery.ps1`, failure matrix. Controlled fenced promotion, not automatic election |
| R10 | Actual storage stop/restart with before/during/after evidence | StatefulSet scale-to-zero, promotion, recovery Job, preserved/outage-period records; `test-k8s-recovery.ps1` |
| R11 | Dockerfiles, built images, running containers, communication | `infra/Dockerfile`, `Dockerfile.migrate`, Compose; image IDs and fresh Kubernetes boot evidence |
| R12 | Deployments, Services, replicas, service communication, scaling, pod recovery | `infra/k8s`; `test-k8s-matching.ps1` asserts replacement UID and 1→2 workers |
| R13 | Demonstrate requests/resources/match, three shards, queue, storage recovery, pod recovery/scaling | [Demo/runbook](runbook.md), release evidence and scripts |
| R14 | Normal, concurrent and failure tests across APIs, queue, shards, replicas, pods | CI unit/HTTP/race/broker integration; local paired-replica/fault tests; measured workload |
| R15 | Justify language/framework, database, partitioning, sync, matching, queues, APIs, Kubernetes, logs/cloud | [Architecture](architecture.md), [API](api.md), [recovery](recovery.md), [versions](versions.md). Cloud deferred; student understanding is separate from implementation evidence |

## Agreed extensions

| Extension | Evidence |
|---|---|
| E1: embedded Leaflet map, selection, saved assignment/location/distance, tile-free fallback | `internal/web`, `TestAssignmentSummaryUsesSavedDistanceAndResourceLocation`, `TestSlowResourceLocationDoesNotHideAssignedRequest`; [browser checklist](../tests/acceptance/browser-checklist.md) |
| E2: atomic durable shared inbox, polling/paging, idempotent read state through restart/promotion | migration6, notification integration/replica tests, `test-k8s-inbox-recovery.ps1`, [inbox API](inbox-api.md) |

SSE, external email delivery, private user accounts, automatic failover elections and cloud deployment are not implemented or silently claimed as extras.

## 18 November revision

R01/R08/R14 now cover progressive search from zero to 4,000 km, including boundary distances, continuation and automatic retries when eligible supply becomes available. R06 supports filtered, newest-first resource listings and lookup alongside the original seven APIs. E1 adds optional geolocation, type-filtered map pins, nearest-request previews and confirmed assignments. E2 covers durable inbox state and response sequencing. Completion preserves allocation history. [Matching and upgrade details](expanded-matching.md)
