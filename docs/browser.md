# Browser workflow

The public API embeds the HTML, JavaScript, CSS and Leaflet 1.9.4 assets. All business reads/writes continue through the coordinator. No credentials are sent to browser JavaScript. Forms use latitude then longitude, including zero and negative coordinates; backend validation remains authoritative.

The browser polls selected-request status and notifications every three seconds, with a ten-second delay after errors. Pending, unavailable and unmatched are distinct from assigned. Mutation retries reuse an operation key while that page remains open. After an uncertain response, retry the same unchanged action before closing/reloading; keys are currently kept in page memory. Saved request IDs may be copied and looked up after reopening.

## Map

Click the map to select coordinates or type them directly. Blue marks the request, green its assigned resource, amber the selected location. The line is a display aid, not a route estimate. Observed distance comes from the persisted assignment intent, not browser matching. Resource coordinates are immutable and read through the coordinator. If that resource shard is unavailable, the request's saved status/distance remain available and the UI explains that coordinates are missing.

The default background is the OpenStreetMap standard tile endpoint. Its [usage policy](https://operations.osmfoundation.org/policies/tiles/) requires visible attribution, ordinary browser caching/referrers and no bulk/offline prefetch. This client requests only the displayed viewport and does not proxy, scrape or prefetch tiles. Tiles are best-effort, and requesting them reveals the viewed map area to the provider; use synthetic locations for this shared demonstration. The application works without external tiles.

`ERM_MAP_TILE_URL` and `ERM_MAP_ATTRIBUTION` on the API process configure a different permitted provider. An explicitly empty tile URL disables background tiles. The per-page `?tiles=off` switch also disables them. Keep attribution appropriate to the chosen provider. Browser configuration contains only these public values. Leaflet is served locally under its included BSD-2-Clause license; [1.9.4 reference](https://leafletjs.com/reference-1.9.4.html).

Coordinate forms, status and inbox do not depend on successful tile loading or the map library. If all JavaScript is disabled, the interactive console displays a notice; the JSON API and documented PowerShell scripts are the fallback. No server-rendered form submission path is claimed.

## Inbox

One shared local recipient (`demo`) receives an assignment notification in the same database transaction as finalization. Repeated deliveries do not create duplicate notifications. Read state is persisted on the request shard, with the same synchronous replay barrier as other mutations. The UI refreshes from the beginning on every poll so random IDs cannot hide later inserts. Cursor paging is explicit and is not a real-time event stream.
