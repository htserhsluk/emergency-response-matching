# Additional inbox API

These endpoints extend, and do not replace, the seven matching APIs. The demo has one shared recipient and no user-private authentication model.

| Method and path | Success | Behavior |
|---|---|---|
| GET /requests/{id}/notifications?after={notificationId}&limit=20 | 200 | `{items,nextAfter}`; ascending opaque ID order; limit1–100; nextAfter omitted when exhausted |
| PUT /requests/{id}/notifications/{notificationId}/read | 200 | Send `{}` and an Idempotency-Key; returns persisted notification/readAt |

A notification contains `id`, `requestId`, `assignmentId`, `resourceId`, `recipient`, `createdAt` and optional `readAt`. Missing requests/notifications or a notification under the wrong request return404. Invalid identifiers, limits or mutation keys return400. Dependencies unavailable return503; retry the identical mutation key. The cursor is pagination, not chronological ordering or an SSE replay token.

GET /requests/{id} also includes optional `assignment: {resourceId,distanceMeters,location?}` for finalized assignments. Distance is the observed persisted matching distance in metres. Coordinates are retrieved from the immutable resource; omission means its shard could not currently supply them. The durable assignedResourceId/status do not disappear in that case.

Migration00006 creates notifications and backfills prior finalized assignments. Historical notification createdAt records the migration insertion time. Subsequent finalization and inbox insertion are atomic. Read timestamps are not reset by repeated calls or different read-operation keys.
