# Controlled storage recovery

These commands affect only the dedicated project lab. Preserve credentials, control state, database volumes and recovery archives. First inspect current routes and roles; never select a primary from its name alone. Run one operator recovery at a time.

## Compose

```powershell
./scripts/test-recovery.ps1 -Shard 0 -From a
# Individual phases, when 0a is the current primary:
./scripts/failover-shard.ps1 -Shard 0 -From a
./scripts/rejoin-node.ps1 -Shard 0 -Target a
```

Use `-From b` after promotion to b. The operator verifies the synchronous replica/system identifier/replay position, disables restart and stops the old database and agent, promotes the candidate, explicitly enables degraded writes, waits for old heartbeat expiry and activates the new versioned route. Heartbeats alone never promote. The old copy must remain fenced until rebuilt.

Rejoin refuses the current primary and running targets, verifies the explicitly named project volume and source, takes a fresh base backup, and archives the old data to a separate recovery volume. ARCHIVE/INSTALL journals allow interrupted copies to resume. Successful streaming and synchronous replay restore paired mode. Never restart a stale old-primary volume directly.

## Kubernetes

```powershell
./scripts/test-k8s-recovery.ps1 -Shard 0 -From a
./scripts/k8s-failover.ps1 -Shard 0 -From a
./scripts/k8s-rejoin.ps1 -Shard 0 -Target a
```

The old StatefulSet is scaled to zero and its pod disappearance verified before promotion. Rejoin uses a dedicated Job, the verified old PVC and a separate archive PVC; the StatefulSet resumes only after successful backup installation. Recovery proofs write before and during the outage, restart the coordinator, verify saved routing, and verify both copies after rejoin.

If a rejoin Job fails, inspect its logs and journal first. After correcting the cause, rerun the matching rejoin command. It replaces a failed Job only after foreground deletion and verifies that its old writer pods are gone; PVCs, archive and journal remain. Running Jobs are reused and successful Jobs can finish the remaining standby-start steps. An interrupted initial bootstrap is different from normal restart/upgrade and must be inspected rather than blindly rerun.

This is controlled failover for a local demonstration, not automatic elections or arbitrary network-partition tolerance. A degraded promoted primary has no synchronous redundancy until rejoin finishes. Avoid a second storage failure in that window. Successful normal-mode mutations require the replica replay barrier.

## Stop and resume

Stop active test scripts first, then stop project Compose services or the Minikube profile. Quit Docker Desktop only after those operations complete. Restart Docker Desktop, start the existing profile and use the saved routes on resume. Do not run `down -v`, delete PVCs, factory-reset Docker or reinitialize control to recover from a routine shutdown.

## Matching workflow recovery

A PAUSED workflow is not a successful match and does not release any resource. Inspect `processed_events` (state, expected_retry, due_at, last_error), `assignment_attempts`, and `reservation_results` on their owning shards. The request and resource can be on different shards. Never clear these rows or create a replacement intent to resolve an uncertain reservation.

For a deliberate re-drive, obtain the paused event's current delivery on the active request shard:

```sql
SELECT jsonb_build_object(
  'event', o.payload, 'deliveryId', o.delivery_id,
  'retryNumber', o.retry_number, 'destination', o.destination)
FROM processed_events p
JOIN outbox_events o ON o.event_id=p.event_id
 AND o.retry_number=p.expected_retry AND o.destination='request.match'
WHERE p.request_id='<request ID>' AND p.state='PAUSED';
```

Send that JSON object as `delivery` in an authenticated POST to the coordinator's `/internal/commands/workflow` with `action: "redrive"`. Use the locally configured internal credential, never paste it into source or logs. Re-drive increments the expected attempt once, adds one outbox delivery, grants three further retries and retains the same logical event and unresolved assignment. Repeating the old command cannot grant another budget. `action: "inspect"` returns a durable workflow decision; `action: "pause"` pauses the current attempt even while its retry is scheduled for the future; ordinary processing still waits until due. Arbitrary finalize/reject/unmatched commands are refused at the coordinator boundary.

New requests must first enter through RabbitMQ. The reconciler scans already-started RUNNING workflows every two seconds and repeats their original attempt; it does not silently execute PAUSED workflows. A broker outage preserves pending outbox rows; a request-shard outage prevents acknowledgement rather than inventing a retry decision. Do not scale the coordinator beyond one instance.

## Recovery filesystem flushes

The rebuild script flushes its own journal and changed directories after each phase. `pg_basebackup` flushes the incoming database before it returns. Avoid replacing these scoped flushes with a global `sync`: during local testing, global sync blocked indefinitely in the host FUSE filesystem even though database-volume flushes completed.

If an old recovery Job is stuck terminating, keep its target fenced and preserve its PVC, archive and journal. Do not force-delete the pod and start another writer. Inspect the process and runtime first; a runtime restart may be necessary to clear an uninterruptible host-filesystem wait. Resume the same rejoin only after the old writer is gone, then verify synchronous replication before continuing failure tests.
