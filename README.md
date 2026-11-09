# Emergency Response Matching

A Go emergency-resource matching system, developed in verified increments.

## Current increment

Request creation, retrieval and guarded status updates through API → coordinator → storage agent → PostgreSQL/PostGIS. Stable operation IDs, transactional outbox records, database-aware heartbeats, independent standby configuration and container builds are included. Resource registration, matching consumers, three-shard expansion and the complete Kubernetes deployment are subsequent increments; this is not a finished emergency-response product.

The Day 1 profile enables only shard 0. Requests hashing to unconfigured shards correctly receive 503 rather than being placed in the wrong database. Do not use the bootstrap profile as evidence of three-shard distribution.

## Local prerequisites

- Go 1.27.3; GCC/cgo for race tests; Goose 3.29.0.
- Docker Desktop with Linux containers, Compose, PowerShell 7; WSL2/Linux for Go and Bash test commands.
- Minikube 1.39.0 and kubectl 1.36.x for the optional bootstrap health smoke.

## Bootstrap

Run PowerShell from the repository root:

```powershell
./scripts/init-env.ps1
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml up -d --wait db-0a
```

Install the pinned Goose CLI in your Go environment, then run migrations from WSL/Linux with this repository as the working directory:

```bash
go install -tags='no_clickhouse,no_libsql,no_mssql,no_mysql,no_sqlite3,no_vertica,no_ydb' github.com/pressly/goose/v3/cmd/goose@v3.29.0
bash scripts/migrate-local.sh
```

Back in PowerShell:

```powershell
docker compose --env-file infra/compose/.env -f infra/compose/compose.yaml up -d --build
./scripts/replication.ps1
```

The API listens only on `http://localhost:8080`. Internal services are not published. Local passwords are randomly generated in ignored `.env`; preserve this file when restarting existing volumes. Never replace it just to repeat setup.

After enabling synchronous replication, keep the standby running. An unavailable standby can make commits time out even when a local write is visible; retry the same operation key after recovery. Controlled degraded mode and promotion are not part of this increment.

## Checks

```bash
go build ./cmd/...
go vet ./...
go test -race ./... -count=1
bash scripts/test-local.sh
bash scripts/test-replica.sh # requires the healthy synchronous pair; pauses/resumes replay
```

Integration tests require the dedicated migrated local fixture and fail if it is absent. They create uniquely identified test data and do not delete existing volumes. The GitHub workflow uses an ephemeral database; local tests are not a claim that remote CI has run.

## Request contract

`POST /requests` takes a printable 16–128 character `Idempotency-Key` and JSON:

```json
{"type":"WATER","location":{"latitude":12.97,"longitude":77.59},"details":"Drinking water needed"}
```

Success is 201. `GET /requests/{id}` returns 200. `PUT /requests/{id}/status` takes an operation key and `{"status":"PENDING"}` for an idempotent no-op, or COMPLETED when already ASSIGNED. Public callers cannot force assignment. Missing fields, malformed JSON and invalid locations are rejected; changing a payload under the same key returns 409. A timeout is not proof of rollback.

Use `GET /health/live` for process health and `/health/ready` for dependency health. The `infra/k8s/smoke.yaml` Deployment/Service proves only image startup and cluster networking, not the full business deployment.

Run `./scripts/smoke.ps1` from PowerShell for a known shard-0 fixture, replay/conflict checks and unavailable-shard exclusion. Its stable fixture ID can be retrieved after database/container restart to verify persistence.

For the bootstrap Kubernetes check, start profile `erm-lab` with Kubernetes v1.36.1, load `erm-api:day1` using `minikube -p erm-lab image load erm-api:day1`, apply `infra/k8s/smoke.yaml`, and create its `erm-smoke` Secret in namespace `erm-smoke` from the local `.env`. Run `./scripts/k8s-smoke.ps1`. Only that dedicated namespace/profile is used.

Reference PDFs, the private handover, local credentials and execution checkpoints are excluded from version control and Docker build context.
