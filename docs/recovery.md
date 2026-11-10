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
