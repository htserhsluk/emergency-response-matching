# Progressive matching and interface revision

## Requirement authority

The assignment requires type/location matching, asynchronous processing, the seven public APIs, three replicated shards, heartbeat/recovery behavior, Docker and Kubernetes. Its section7 explicitly leaves the matching strategy to the implementation. The original 5/10 km cap and original UI were handover design choices. The 18 November 2026 change request supersedes those choices, while retaining ownership, synchronous confirmation, recovery and API guarantees.

## Matching policy

Every new generation starts at radius0. Radii in kilometres are:

`0, 1, 5, 10, 25, 50, 100, 250, 500, 1,000, 2,500, 5,000, 10,000, 20,100`.

The final radius covers the greatest geodesic separation on Earth. Distance uses PostGIS geography in metres, with inclusive radius boundaries. Exact-location resources are searched first. Every shard must respond before nearest candidates can be considered; candidates sort by observed distance, then resource ID. Only available resources of the same type are eligible. UNAVAILABLE is visible in the unallocated directory but never matchable.

Migration7 adds generation-owned radius and distance/ID cursor fields to processed events. Each definitive candidate rejection advances that cursor through the existing synchronous transaction barrier. Empty/finished pages advance exactly one radius. Unknown reservation outcomes return before cursor advancement and resume the same assignment/resource identity. Candidate checks no longer impose the old separate10km cap.

One delivery retains the existing five-second parent and two-second assignment-step deadlines. It processes at most16 search pages, checks remaining time between pages/candidates, and yields through a durable outbox continuation when approaching the budget. Continuations advance delivery identity and retry allowance together, preserving the remaining dependency-failure budget. Actual uncertain/dependency failures retain bounded retry/backoff, pause and operator-redrive behavior. A search becomes UNMATCHED only after confirmed exhaustion of the final radius with no unresolved intent.

This is an observed live search, not a distributed snapshot. A resource inserted or made available behind an already-processed cursor may require a new generation after the current search finishes. It is not a promise of a globally optimal allocation under concurrent changes, nor road routing, emergency triage, or a guarantee of physical service availability. The finite radius sequence and bounded deliveries prevent an endless synchronous search; repeated external writes can extend an asynchronous live scan.

Historical UNMATCHED records retain their status and are labeled LEGACY_UNMATCHED instead of falsely claiming global exhaustion. Manual retry creates a new generation beginning at zero. Existing assigned/completed records and allocation associations are preserved. Completion still never releases a resource.

## Interface and map

The layout uses slate, blue and teal, readable labels and distinct request/offer sections. The directory defaults to UNALLOCATED, with AVAILABLE/UNAVAILABLE filters and ALLOCATED history. Server-side filtering precedes pagination on every shard. The unfiltered API remains backward compatible. Ordering remains deterministic ID ordering, not newest-first; the interface explains this and shows the just-created resource directly with fresh lookup.

Refresh reports loading, completion, unchanged results, empty results and errors. Sequence guards reject older resource, request and inbox responses. Availability changes and observed assignments refresh affected views. Allocation history keeps its request association and cannot be made reusable.

The map now draws the initial selection and uses distinct labeled selection/request/resource pins plus a legend. Small visual offsets distinguish coincident pins without changing stored coordinates. Coordinate edits and map selection stay synchronized. Request switching clears previous markers; repeated polling and temporary loss/recovery of optional resource coordinates do not repeatedly reset a user's map view. Resize observation refreshes the map layout.

“Use my location” is explicit and optional. It requests browser geolocation with a10second timeout, displays approximate accuracy, and refuses to overwrite the current selection when reported accuracy exceeds1,000metres or a later manual edit has occurred. Denial, unavailability, timeout, unsupported browsers and invalid results leave manual entry usable. Location is not automatically submitted as a request/resource. Tiles remain optional; matching stays on the server, and configured tile attribution is retained.

## Diagnosed behavior

- The original refresh request was working: HTTP200,20 ID-sorted records. Repeated refresh lacked completion feedback and mixed allocation history with unallocated resources. It was not a chronological feed.
- The original map had no initial selection marker. Its assignment view centered only on the request. Source inspection and browser reproduction confirmed these gaps before changes.
- Live verification caught valid coordinate input leaving the selection pin at its old location until a change event. Input now updates the pin immediately without rewriting partial typing or panning on each keystroke. A failing regression and deployed overlap check verified the fix.
- A regression reproduced older same-request polling overwriting a newer COMPLETED response. A request sequence guard fixes it.
- Review caught two new implementation defects before delivery: slow candidate pages could consume retries without saving per-candidate progress; and old limited-search records were mislabeled globally exhausted. Dedicated failing regressions drove their fixes.

## Verification

See the current revision entry in the evidence index for actual executed outcomes. The new tests cover filtered multi-shard paging/outages and lookup associations; zero-distance/ties/inclusive boundaries; distant/near-antipodal matching; complete exhaustion and generation reset; slow-candidate continuation without spending failure budget; persisted progress across restart/replay barriers; stale UI results; created-card updates; map pin initialization and polling stability; and location success/error/accuracy outcomes. Existing contention, uncertain reservation, paused/redrive, replication, broker and race checks remain required.

Use separately tagged `expanded` images and the existing quiesced upgrade process to apply migration7 to saved primaries. Do not bootstrap or delete existing volumes. Deployment smoke tests use a new exact-location fixture because existing resources anywhere on Earth now matter; isolated integration tests prove globally empty searches without changing unrelated data.

## Clarified background-tile report

The reported long delay after distant coordinate changes concerns external background tiles, not just marker rendering. Tiles-enabled browser checks moved from Bangalore to New York and Sydney; destination tile images loaded successfully in both cases. The long delay was not reproduced, so no provider-latency fix is claimed. The map now explicitly reports loading, a slow-provider message after eight seconds, completion and partial failure, while keeping pins/forms usable. A controlled delayed/failing tile-event regression verifies those states and recovery; no bulk prefetch or automatic retry loop was added.
