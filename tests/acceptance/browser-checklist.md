# Browser acceptance checklist

Run against the local demonstration API, with the existing database volumes preserved. Use synthetic coordinates only. The final release repeats these checks after clean deployment.

1. Register a resource and submit a same-type request at (0, 0). Observe PENDING then ASSIGNED, the saved resource ID, zero distance and coordinates. Complete the request; its resource remains allocated.
2. Click the map and confirm the named latitude/longitude fields change. The map displays the saved request and assigned-resource markers; it does not choose a resource.
3. Run `scripts/test-k8s-public-api.ps1`. Look up the printed request ID in the browser and compare resource ID, location and distance with GET. Its fixture includes a nearer wrong-type resource, an out-of-radius resource and an eligible resource about 5.55 km away. The script also verifies invalid coordinates, manual matching, completion and resource protection.
4. Open `/?tiles=off`. Register and match a synthetic request at negative latitude/longitude. Status, forms, markers and inbox must work without background tiles. If the map library fails, coordinate forms remain the fallback; if all JavaScript fails, use the documented HTTP scripts.
5. Mark a notification read, reload and look up the request again. Exactly one logical notification remains read. Run `scripts/test-k8s-inbox-recovery.ps1`, then inspect its printed request in the browser: read state must survive API restart, request-shard promotion and replica rebuild.
6. Repeat a lookup while a refresh is in flight. The Node regression `node --test internal/web/app_test.cjs` asserts only one polling loop survives.

## Observed on 12 November 2026

- Zero-coordinate registration, assignment and completion passed. Map click updated both coordinate fields. A tile-free assignment at (-12, -77) passed.
- Final deployed fixture request `adee38940323acd0fd94732840077c4e` (shard 2) selected resource `1d3890bf0768a0d354d605bebc54c3fb` (shard 1). Browser showed COMPLETED, resource location (-39.95, 150), and 5551.7 metres, matching GET's stored 5551.70760983 metres. The wrong-type and distant resources were not selected.
- After API restart and shard promotion/rebuild, request `857fb8acb53a6ba58b1efd658e98a5fd` displayed ASSIGNED and one Read notification in a freshly loaded browser page. Its persisted notification ID is `de0fb6f332596e8959e162582a555853`.
- These IDs belong to the preserved local fixture, not a fresh installation. Future runs generate new IDs.
