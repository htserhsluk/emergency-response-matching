# HTTP contract

JSON requests/responses use `Content-Type: application/json`. Mutation requests require a printable ASCII `Idempotency-Key` of 16–128 characters. Preserve the same key and payload when retrying an uncertain response; changed payload under the same operation identity is a conflict. Resource/request IDs are 32 lowercase hexadecimal characters. See contract and integration tests for exact operation scopes.

| Method and path | Success | Body / behaviour |
|---|---|---|
| `POST /requests` | 201 | `{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"details":"Drinking water needed"}`; durable save and asynchronous matching |
| `GET /requests/{id}` | 200 | Request fields and current status, plus saved assignment summary when present |
| `PUT /requests/{id}/status` | 200 | `{"status":"COMPLETED"}` after assignment; current-state no-op allowed, public assignment bypass forbidden |
| `POST /resources` | 201 | `{"type":"WATER","location":{"latitude":12.9,"longitude":77.6},"availability":true}` |
| `GET /resources` | 200 | `{items,next}`; optional `limit`/`after`, deterministic cross-shard ID order; no partial successful listing |
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
