# Emergency Response Matching

A Go emergency-resource matching system, developed in verified increments.

## Current increment

Day 4 adds an embedded browser workflow, a locally served Leaflet map, saved assignment distance/location and a durable shared demonstration inbox. The asynchronous matching, three replicated shards and bounded recovery remain authoritative. Final load, clean-start and release acceptance are the next milestone.

## Prerequisites

Go 1.27.3, GCC/cgo for race tests, Goose 3.29.0, Docker Desktop Linux containers, PowerShell 7, WSL Ubuntu, Minikube 1.39.0 and kubectl 1.36.x. Images and dependencies are pinned; see `docs/versions.md`. The local startup helper currently uses the development machine's WSL tool paths; adapt these to your checkout before use on another computer.

## Compose setup and restart

Run from the repository root in PowerShell:

```powershell
./scripts/init-env.ps1                    # once; refuses existing credentials
./scripts/start-lab.ps1 -InitializeControl # first installation only
# Later starts:
./scripts/start-lab.ps1
```

Install Goose in the Linux Go environment before startup:

```bash
go install -tags='no_clickhouse,no_libsql,no_mssql,no_mysql,no_sqlite3,no_vertica,no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.29.0
```

Startup follows the saved primary identities, starts their standbys before migration, and refuses to start a stale former primary. API: `http://localhost:8080`; coordinator management: localhost:8081. PostgreSQL ports 55432–55437 map to 0a,0b,1a,1b,2a,2b. Broker AMQP/management ports are localhost:5672/15672. Credentials live only in ignored `infra/compose/.env`; preserve that file and the volumes across restarts.

Use the controlled recovery commands in `docs/recovery.md`. Do not infer the primary from an `a` suffix after a promotion. A timeout can leave a local commit whose synchronous confirmation is unknown: retry the same operation key after recovery.

## Kubernetes

Build the Compose application images and the migration image:

```powershell
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml build api coordinator node-0a matching
docker build -f infra/Dockerfile.migrate -t erm-migrate:day4 .
docker build -f infra/Dockerfile --build-arg SERVICE=matching -t erm-matching:day4 .
minikube -p erm-lab image load erm-api:day4 erm-coordinator:day4 erm-storage-node:day4 erm-migrate:day4 erm-matching:day4
./scripts/bootstrap-k8s.ps1 # new namespace/storage only
```

For an existing initialized cluster, use `./scripts/upgrade-k8s.ps1`; bootstrap refuses an existing control PVC. Upgrade migrates the actual primaries and preserves promoted roles. A failed initial setup needs inspection before resuming, not a blind reapply. Namespace `erm` has six two-container storage pods, API/coordinator/Matching Deployments, Services, independent data PVCs, control/archive PVCs and a persistent RabbitMQ StatefulSet. The older `erm-smoke` namespace is only a Day 1 health demo.

Run one full deployment at a time on a small laptop. Stop the Compose broker before the Kubernetes broker demonstration. Do not remove volumes to free memory.

## Checks

```bash
go build ./cmd/...
go vet ./...
go test -race ./... -count=1
bash scripts/test-local.sh
bash scripts/test-replica.sh # dedicated synchronous pair; pauses/resumes replay
bash scripts/test-broker.sh # dedicated broker test vhost; see below
```

The local database scripts accept the current primary port; replica tests also accept standby port. For example, after promotion to 0b: `bash scripts/test-replica.sh 55433 55432`. Integration suites use unique fixtures/isolated schemas. Configure the dedicated Compose broker test vhost once:

```powershell
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml exec -T broker rabbitmqctl add_vhost erm-tests
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml exec -T broker rabbitmqctl set_permissions -p erm-tests erm '.*' '.*' '.*'
./scripts/test-startup-order.ps1
./scripts/test-bootstrap-guard.ps1
./scripts/test-k8s-partial-availability.ps1
./scripts/test-k8s-matching.ps1
```

The last command tests actual asynchronous assignments, scaling Matching from one to two pods, worker replacement, broker reconnect, coordinator restart and completion protection. The older test-k8s-messaging.ps1 is a Day 2 diagnostic fixture and expects matching workers to be stopped. Recovery demonstrations and their commands are in `docs/recovery.md`. Verification results are in `docs/test-results.md`; local checks do not imply remote CI passed.

## API

Requests require a printable 16–128 character `Idempotency-Key` for mutations. Reusing it with a changed payload returns 409. Invalid JSON/fields return 400, absent entities 404, and unavailable required shards 503.

- `POST /requests` → 201: `{"type":"WATER","location":{"latitude":12.97,"longitude":77.59},"details":"Drinking water needed"}`.
- `GET /requests/{id}` → 200.
- `PUT /requests/{id}/status` → 200: `{"status":"PENDING"}` is an idempotent no-op; COMPLETED requires an existing assignment. Callers cannot force assignment.
- `POST /resources` → 201: `{"type":"WATER","location":{"latitude":12.97,"longitude":77.59},"availability":true}`. Availability is required; false and zero coordinates are valid.
- `GET /resources?limit=20&after={id}` → 200, `{items,next}` ordered by ID across all shards. An unavailable shard fails the whole query.
- `PUT /resources/{id}/status` → 200: `{"status":"AVAILABLE"}` or UNAVAILABLE. Public calls cannot allocate or release allocated resources.
- `POST /match/{requestId}` → 200 with scheduled/current state. Pending/assigned requests are not falsely reported as completed; unmatched requests can schedule a new generation.

All business storage calls go through the coordinator. Internal endpoints require the shared internal secret; typed `/internal/data/request:{id}` and `resource:{id}` support safe CRUD, with deletion refused for records retaining operation/assignment/delivery history. `/health/live` is process health; `/health/ready` checks complete routing/database dependencies. Kubernetes API serving readiness uses process health so healthy-shard operations remain reachable during a partial outage; it does not require broker availability (the outbox permits broker outages).

Reference PDFs, the private handover, local credentials and execution checkpoints are excluded from Git and Docker build context.

## Matching and retry behaviour

Matching searches by resource type, first within 5 km and then 10 km, ordered by observed geographical distance and stable ID. Every required shard must answer. The coordinator validates the candidate using PostGIS before reservation and serializes request mutations with a fixed set of 256 cancellable locks.

An intent is durable before reservation. Reservation atomically records success or definite rejection with resource allocation. Unknown outcomes retain the original assignment/resource IDs; reconciliation repeats the same attempt. Only a confirmed rejection permits another candidate. Finalization atomically records ASSIGNED and event completion. Public completion never releases the resource.

Valid transient failures schedule at most three durable retries at 2, 5 and 15 seconds. Exhaustion records PAUSED and a diagnostic dead-letter delivery. Reconciliation respects the same pause and due time. Operator re-drive resumes the original intent with a recorded additional retry budget; see `docs/recovery.md`. Unscheduled future attempts and malformed/unknown-schema messages cannot change workflow state. Raw malformed messages are acknowledged only after a confirmed dead-letter publish.

The API remains the single outbox publisher. Run exactly one coordinator; scale Matching, not the coordinator. Each worker owns its AMQP channel with prefetch 1 and manual acknowledgements. Reconnects back off, rebuild topology and leave uncertain work unacknowledged. Shutdown stops intake and bounds in-flight work to 20 seconds. `--inspect-one` remains a diagnostic option, not the normal worker mode.

Worker readiness now requires an active consumer session; liveness remains process health. GET requests include saved assignment distance and resource coordinates when their shard is available. Worker/reconciliation failures produce bounded diagnostic messages without payloads or credentials. This is a controlled local lab, not production HA.

## Browser and inbox

Open `http://localhost:8080` for Compose. For Kubernetes, run `kubectl --context erm-lab -n erm port-forward service/api 18080:8080`, then open `http://localhost:18080`. The API serves the page and its versioned map library; no frontend build or extra service is required.

Register assistance, submit a request, look up its ID, inspect the saved assignment and mark it completed. The resource directory allows AVAILABLE/UNAVAILABLE transitions while allocated resources remain protected. The inbox belongs to the selected request and the shared `demo` recipient; it is not a private user account. Its read state survives refresh and service/database recovery.

Map background tiles are optional. Use `?tiles=off` for the tile-free coordinate/marker workflow. See [browser documentation](docs/browser.md) for configuration, attribution and fallback limits. See [inbox API](docs/inbox-api.md) for the two additional endpoints; the original seven APIs are unchanged.

```powershell
./scripts/test-k8s-public-api.ps1
./scripts/test-k8s-failure-matrix.ps1
./scripts/test-k8s-inbox-recovery.ps1
./scripts/test-k8s-matching.ps1
```

Run failure scripts sequentially in the local demonstration cluster. They stop pods and temporarily allow single-copy writes where explicitly documented, then restore paired synchronous operation. Preserve volumes and inspect a failed recovery before retrying. The inbox migration is version6; existing finalized assignments receive one historical inbox entry at migration time.
