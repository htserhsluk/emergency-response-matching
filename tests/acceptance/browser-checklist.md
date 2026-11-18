# Browser acceptance checklist

Run against the local demonstration API, with the existing database volumes preserved. Use synthetic coordinates only. The final release repeats these checks after clean deployment.

1. Register a resource and submit a same-type request at (0, 0). Observe PENDING then ASSIGNED, the saved resource ID, zero distance and coordinates. Complete the request; its resource remains allocated.
2. Click the map and confirm the named latitude/longitude fields change. The map displays the saved request and assigned-resource markers; it does not choose a resource.
3. Run `scripts/test-k8s-public-api.ps1`. Look up the printed request ID in the browser and compare resource ID, location and distance with GET. Its current fixture includes a wrong-type resource, a farther same-type resource and a unique exact-location eligible resource. Global empty/distant/boundary cases are verified in isolated integration tests. The script also verifies invalid coordinates, manual matching, completion and resource protection.
4. Open `/?tiles=off`. Register and match a synthetic request at negative latitude/longitude. Status, forms, markers and inbox must work without background tiles. If the map library fails, coordinate forms remain the fallback; if all JavaScript fails, use the documented HTTP scripts.
5. Mark a notification read, reload and look up the request again. Exactly one logical notification remains read. Run `scripts/test-k8s-inbox-recovery.ps1`, then inspect its printed request in the browser: read state must survive API restart, request-shard promotion and replica rebuild.
6. Repeat a lookup while a refresh is in flight. The Node regression `node --test internal/web/app_test.cjs` asserts only one polling loop survives.

## Observed on 12 November 2026

- Zero-coordinate registration, assignment and completion passed. Map click updated both coordinate fields. A tile-free assignment at (-12, -77) passed.
- Final deployed fixture request `adee38940323acd0fd94732840077c4e` (shard 2) selected resource `1d3890bf0768a0d354d605bebc54c3fb` (shard 1). Browser showed COMPLETED, resource location (-39.95, 150), and 5551.7 metres, matching GET's stored 5551.70760983 metres. The wrong-type and distant resources were not selected.
- After API restart and shard promotion/rebuild, request `857fb8acb53a6ba58b1efd658e98a5fd` displayed ASSIGNED and one Read notification in a freshly loaded browser page. Its persisted notification ID is `de0fb6f332596e8959e162582a555853`.
- These IDs belong to the preserved local fixture, not a fresh installation. Future runs generate new IDs.

## Expanded interface checks

- Directory defaults to UNALLOCATED; AVAILABLE/UNAVAILABLE filters and ALLOCATED history page through server-filtered results. Refresh returns to page1 and reports unchanged results. Created resources remain accessible directly even if their ID is not on page1.
- Availability changes update both the created card and directory; allocation removes availability controls and retains request associations in history.
- Initial selection pin is visible. Edit coordinates, click the map, switch tracked requests and allow repeated polls; check synchronization, overlap labels and absence of unwanted view resets.
- At390 px mobile viewport, verify no horizontal overflow and map reflow. Tiles-off and tile failures leave selection/markers usable.
- Geolocation is explicit; validate success/approximate accuracy, denial, timeout, unsupported, poor accuracy and late callbacks after manual edits. Automated tests inject outcomes; actual hardware accuracy and permission presentation vary by browser.
- Reordered request/list/inbox responses must not regress a newer view. Run both Node test files.

## Executed revision checks (18–19 November)

Desktop and 390 px responsive rendering, tile-free initial selection, coordinate input, coincident selection/request markers, request/resource fit, unallocated/history switching, refresh completion, created-resource lookup, 111 km assignment, inbox read and completion/history retention were exercised against the upgraded local deployment. Fourteen Node regressions cover event ordering, paging/filter reset and simulated geolocation/tile failures. Geolocation permission outcomes are simulated boundary tests, not a claim of testing every native browser prompt. See the [evidence index](../../docs/evidence-index.md).
