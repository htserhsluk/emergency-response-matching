# HTTP contract

JSON requests/responses use `Content-Type: application/json`. Mutation requests require a printable ASCII `Idempotency-Key` of 16–128 characters. Preserve the same key and payload when retrying an uncertain response; changed payload under the same operation identity is a conflict. Resource/request IDs are 32 lowercase hexadecimal characters. See contract and integration tests for exact operation scopes.

| Method and path | Success | Body / behaviour |
|---|---|---|
| `POST /requests` | 201 | `{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"details":"Drinking water needed"}`; durable save and asynchronous matching |
| `GET /requests/{id}` | 200 | Request fields and current status, plus saved assignment summary when present |
| `PUT /requests/{id}/status` | 200 | `{"status":"COMPLETED"}` after assignment; current-state no-op allowed, public assignment bypass forbidden |
| `POST /resources` | 201 | `{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"availability":true}` |
| `GET /resources` | 200 | `{items,next}`; filtered pages across all shards, ascending ID by default or newest registration with `sort=newest` |
| `PUT /resources/{id}/status` | 200 | `{"status":"AVAILABLE"}` or `UNAVAILABLE` for unallocated resources; no public allocation/release bypass |
| `POST /match/{requestId}` | 200 | `{}`; schedules eligible unmatched work or returns current state, never promises immediate assignment |

Types: WATER, FOOD, MEDICINE, TRANSPORTATION, SHELTER. Latitude is [-90,90], longitude [-180,180]; explicit zero and negative values are valid. Request details must contain non-whitespace text and at most 2000 Unicode characters. Availability is required; `false` is valid.

Requests progress through PENDING, ASSIGNING, ASSIGNED or UNMATCHED. ASSIGNING is an intermediate durable intent, not an error. Assigned requests may become COMPLETED. Resource status is AVAILABLE, UNAVAILABLE or ALLOCATED. Completion does not release the resource. Manual matching can schedule a new generation for an unmatched request.

Errors: 400 invalid JSON/fields/identities, 404 absent entity, 409 changed-payload or forbidden-state conflict, 503 required dependency unavailable. Requests and dependency calls are bounded; a timeout may leave a locally committed write awaiting confirmation. Never change the key to evade that ambiguity.

GET assignment includes stored distance; resource coordinates are optional when their shard cannot answer within 250 ms. Saved request status/distance remain available if the request shard is accessible. Inbox endpoints, limits/cursors/read idempotency are specified in [inbox-api.md](inbox-api.md).

## Internal boundaries

`X-Internal-Secret` protects internal commands. Node registration/heartbeats report boot identity, monotonic sequence, shard, database reachability/role and WAL position. The coordinator rejects stale observations and routes only to eligible current primaries. Route versions fence retired writers.

Typed `/internal/data/request:{id}` and `resource:{id}` provide safe CRUD; deletion refuses records with operation, event or assignment history. Internal intent, reservation, candidate, workflow, outbox claim/sent and pending commands preserve the same invariants. Finalize/reject/unmatched orchestration is private to the coordinator, not trusted caller assertions. See `internal/httpapp` and `TestCoordinatorRejectsUnsafeAssignmentCommands`.

`/health/live` checks process health. `/health/ready` represents service-specific dependencies; Kubernetes API traffic intentionally uses process serving readiness to retain partial-shard availability. Keep management/DB ports local and secrets outside source control.

## Resource views, map and matching progress

`GET /resources` accepts optional `status`, `type`, `sort`, `limit` and `after`. Status accepts UNALLOCATED (AVAILABLE and UNAVAILABLE), AVAILABLE, UNAVAILABLE or ALLOCATED. Type accepts the five assistance types above. Omitting sort yields the original ascending-ID ordering. Use `sort=newest` for descending registration time and descending ID. Historical registrations with unknown times follow dated records. Each shard filters before applying its limit. Pass the returned `next` as `after` with the same filters and sort. Restart pagination after changing filters. Concurrent changes can move records between pages.

`GET /resources/{id}` returns the saved resource, its registration time when known, and its assigned request ID when allocated.

`GET /map/locations?type=WATER` returns pages of up to 100 saved request and resource locations for the specified type, with kind, ID, location and status. Supply the returned `next` as `after` to continue.

`GET /resources/{id}/candidates` returns `items` with up to five eligible requests ranked by geographic distance and ID. Each item has an ID, location and distance in metres. Candidates must be within 4,000 km and have the same assistance type. Paused requests and requests with active reservations or assignment intents are excluded. Once the resource has an owner, `items` is empty. The `assignedRequest` field appears after the request shard confirms finalization and includes the saved assignment distance.

`GET /requests/{id}` may include `matching: {state,radiusMeters,reason?}`. SEARCHING, RETRYING and PAUSED describe active work or its recovery state. EXHAUSTED means the search completed through the current 4,000 km cap. LEGACY_UNMATCHED identifies a historical limited search. ASSIGNED indicates a saved allocation. A newly queued request may have no summary yet.

The coordinator automatically schedules a new generation when an unmatched request has available same-type supply within 4,000 km. Registration and availability updates are sufficient to trigger subsequent matching through reconciliation. Each new generation begins at zero. The manual match endpoint remains available.
