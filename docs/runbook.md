# Operator runbook and final demonstration

Use PowerShell7 from the repository root. Tools: Docker Desktop Linux engine, Minikube1.39.0, kubectl1.36.x; Python3 for the optional measured workload. Go1.27.3/GCC is needed for local race tests, or use the pinned Go container. Application containers need neither Python nor a Go toolchain. Preserve `infra/compose/.env`, control/archive state and all PVCs/volumes.

## Initial installation

1. Clone the tested revision; review [versions](versions.md). Run `./scripts/init-env.ps1` only if `.env` does not exist. It generates local credentials; do not commit or print them.
2. Start Docker and create the cluster: `minikube start -p erm-lab --driver=docker --kubernetes-version=v1.36.1 --cpus=2 --memory=3072`.
3. Build sequentially (avoid competing compiler builds on a small host):

```powershell
foreach ($service in @('api','coordinator','storage-node','matching')) {
    docker build -f infra/Dockerfile --build-arg "SERVICE=$service" -t "erm-${service}:map-v10" .
    if ($LASTEXITCODE) { throw "Build failed: $service" }
}
docker build -f infra/Dockerfile.migrate -t erm-migrate:map-v10 .
if ($LASTEXITCODE) { throw 'Migration image build failed' }
minikube -p erm-lab image load erm-api:map-v10 erm-coordinator:map-v10 erm-storage-node:map-v10 erm-matching:map-v10 erm-migrate:map-v10
if ($LASTEXITCODE) { throw 'Image load failed' }
./scripts/bootstrap-k8s.ps1
```

Bootstrap initializes the coordinator once, migrates each initial primary through schema 10, then brings up its standby and enables synchronous replay. It refuses an existing control PVC. For subsequent upgrades use `./scripts/upgrade-k8s.ps1`; it follows saved primaries and resets Matching to one worker. Never blindly bootstrap over an interrupted installation: inspect Jobs, roles, PVCs and journal state first.

Rebuild and load all five images before upgrading an existing deployment. Schema 8 timestamps new resource registrations. Schema 9 updates the search limit and enables automatic retries for unmatched requests. Schema 10 supports allocation-time history. Use the matching `map-v10` application and migration images. Record the image IDs and source revision for each rehearsal.

## Restart, stop and inspection

The dashboard launcher uses Minikube's Docker driver and the OpenSSH client. Run it in its own PowerShell window with Docker, Minikube, kubectl and ssh on PATH. The localhost URL stays the same when Kubernetes replaces the API pod.

```powershell
minikube start -p erm-lab --driver=docker --kubernetes-version=v1.36.1
kubectl --context erm-lab -n erm get deployments,statefulsets,pods,pvc,services
./scripts/open-dashboard.ps1
# Visit http://localhost:18080/?tiles=off; stop the local Service tunnel with Ctrl+C.
# At the end, stop any owned forwards, then:
minikube stop -p erm-lab
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml stop
```

Stopping retains volumes. Never use `down -v`, Docker reset, PVC deletion or reinitialize-control as routine restart. A stale former primary stays fenced until the rejoin procedure is complete. If Docker/WSL is unresponsive, preserve files and volumes, record the active recovery Job, then restart the host only after saving work.

## Acceptance and rehearsal

Run failure scenarios sequentially. Read [recovery](recovery.md) before mutation; check active primary rather than assuming suffix `a`. Scripts assert outcomes and throw on failure. Preserve logs, timestamps and IDs. Use a private output directory such as `.local/evidence` and publish reviewed summaries only.

```powershell
./scripts/test-k8s-public-api.ps1
./scripts/test-k8s-matching.ps1
./scripts/test-k8s-inbox-recovery.ps1
# Each recovery run must receive the current primary suffix:
./scripts/test-k8s-recovery.ps1 -Shard 0 -From a
./scripts/test-k8s-failure-matrix.ps1
```

The example `-From a` is not a command to copy without checking routes. Failure-matrix deliberately permits temporary single-copy writes in this local demonstration, then restores synchronous pairs. Do not interrupt halfway and start another recovery writer. Inspect a failed Job and saved control state before retrying.

The demonstration order is topology → request/resource creation and validation → both entities on all three shards → workers paused with actual queue backlog → resume/assignment → type/radius and contention proofs → storage stop/promotion/outage writes/rejoin → worker replacement → two workers → map/inbox read-state persistence. Stored tests additionally cover unknown reservations, retry exhaustion/DLQ/redrive, both-copy loss and stale promotion refusal. Student explanation and any course-specific live demonstration remain human tasks.

For workload measurement, forward the API and run:

```powershell
python -m unittest discover -s scripts -p test_workload.py
python scripts/workload.py --count 30 --concurrency 5 --output .local/workload.json
```

The harness creates 30 available MEDICINE resources, then five closed-loop clients create and poll requests. Every attempt is recorded; errors and duplicate resource owners fail the command. Intermediate ASSIGNING is allowed. Assignment latency includes HTTP/polling overhead (250 ms interval), not server-only processing. Fixture records are retained, not destructively cleaned. Record host/cluster resources, backlog/recovery separately. A small local sample is not a production capacity claim.

## Fresh rehearsal without deleting the existing deployment

The scripts intentionally use explicit context `erm-lab` and namespace `erm`. To test fresh storage without rewriting them, stop the existing profile and use a separate profile with a private kubeconfig. The alias below exists only in that file; do not rename the normal context.

Failover journals are separated by database system identifier, so fresh and preserved databases cannot select each other's checkpoints. Matching legacy journals remain available for safe interrupted recovery. See [journal identity](recovery.md#recovery-journal-identity).

```powershell
minikube stop -p erm-lab
$savedKubeconfig = $env:KUBECONFIG
$env:KUBECONFIG = Join-Path (Get-Location) '.local/release-kubeconfig'
# Refuse/review an existing rehearsal profile or kubeconfig before starting.
minikube start -p erm-release --driver=docker --kubernetes-version=v1.36.1 --cpus=2 --memory=3072
kubectl config rename-context erm-release erm-lab
minikube -p erm-release image load erm-api:map-v10 erm-coordinator:map-v10 erm-storage-node:map-v10 erm-matching:map-v10 erm-migrate:map-v10
./scripts/bootstrap-k8s.ps1
# Run the acceptance/rehearsal commands above against this isolated kubeconfig.
# Preserve its logs and volumes; restore the original environment afterward:
minikube stop -p erm-release
$env:KUBECONFIG = $savedKubeconfig
minikube start -p erm-lab --driver=docker --kubernetes-version=v1.36.1
```

Check every external command's exit status before continuing. An existing `.local/release-kubeconfig` or `erm-release` profile is a resumable rehearsal, not a fresh one. Do not remove it automatically. Do not run two full stacks simultaneously on a small laptop. Authentication/kubeconfig files, raw private logs and credentials stay uncommitted.

The public smoke script now creates a unique exact-location fixture and preserves existing records. The isolated integration fixture controls the available supply and checks that exhausting the 4,000 km search leaves the request unmatched.
