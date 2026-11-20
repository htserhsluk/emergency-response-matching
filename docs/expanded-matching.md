# Matching within 4,000 km

Every new request and every new matching generation searches at the following distances in kilometres.

`0, 1, 5, 10, 25, 50, 100, 250, 500, 1,000, 2,500, 4,000`

PostGIS uses geodesic distance in metres and includes the boundary. Only available resources of the requested type qualify. All three shards must answer. The coordinator merges candidates by distance and then by stable resource ID, and validates the candidate before reservation.

## Automatic retries

When a search exhausts the final radius, the request becomes UNMATCHED. The coordinator checks batches of unmatched requests in least-recently-checked order on a two-second reconciliation timer. When a probe finds available supply of the same type within the cap, the coordinator looks up the closest eligible request to that resource across all shards. If it is UNMATCHED, the coordinator schedules a new generation. If it already has active matching work, that work continues. This covers both registration and a switch from UNAVAILABLE to AVAILABLE, including a change that races the previous search's last page. Large backlogs, outages and active work can delay a pass. A browser session is optional.

The probe starts a generation only if the request still has the observed generation and remains UNMATCHED. The request change and first delivery commit together. That delivery begins at zero and chooses the nearest candidate observed during the new search. Candidate availability is checked atomically during reservation. Concurrent requests can compete for one unit. The manual match endpoint remains available.

## Continuation and upgrades

Each bounded delivery saves the generation's radius and distance/ID cursor. Definitive rejections advance the cursor. Continuations preserve the remaining dependency retry budget. An uncertain reservation retains its assignment identity until the resource shard confirms the outcome. Paused workflows require operator redrive. Completion preserves ownership and allocation history.

Schema 8 records registration times for new resources. Older rows share an unknown-time sentinel and sort behind dated rows in newest-first views. Schema 9 preserves prior search positions up to the cap. A cursor inside the old 5,000 km band resumes within the new 4,000 km band. Positions already beyond the cap are exhausted. For an unresolved old intent outside the cap, the resource shard replays its recorded reservation result or durably rejects the unexecuted command under the same assignment lock. Previously allocated resources retain their owners.

Use the existing quiesced upgrade process against the saved primaries and their replicas. Build the migration and application images from the same checkout before upgrading. The November 18–19 deployment evidence describes schema 7 and its earlier global radius policy.

## Directory and map

The browser requests newest-first resource pages. The directory filters by status. API callers can also filter by assistance type before pagination. Historical undated registrations have a stable ID tie-break. The default API order remains ascending ID for callers that omit the sort option.

Choosing an assistance type loads saved request and resource locations of that type. Selecting a resource shows up to five nearby eligible requests ordered by distance and ID. Concurrent activity may change eligibility between refreshes. Once the assignment is confirmed the map identifies the matched request from the saved association. The map also supports manual coordinates and optional browser geolocation. Background tiles are optional. See [browser controls](browser.md) and the [HTTP contract](api.md).
