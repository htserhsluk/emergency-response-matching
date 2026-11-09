# Day 1 verification

Local checks run on 10 November 2026 (Asia/Calcutta), using versions recorded in `versions.md`.

| Check | Observed result |
|---|---|
| `go build ./cmd/...` | Passed for API, coordinator and storage agent |
| `go vet ./...` | Passed |
| `go test -race ./... -count=1` | Passed; contract, JSON, routing, registry and readiness tests |
| `bash scripts/test-replica.sh` | Passed: replay barrier, payload conflict barrier, concurrent create, canceled transaction cleanup and actual coordinator HTTP path |
| `scripts/smoke.ps1` | Passed: 201 create, GET, stable replay, 409 changed payload and 503 unconfigured shard |
| Primary restart then same smoke | Passed; original creation timestamp and record retained |
| `pg_stat_replication` | node-0b streaming with sync_state=sync |
| Standby role and data | pg_is_in_recovery=true; acknowledged request readable; direct mutation rejected |
| Paused replay | New/identical/conflicting retries do not claim confirmed outcomes; resume preserves one request/event |
| `scripts/k8s-smoke.ps1` | Passed: API image reached through actual Deployment and Service |

Independent review identified three regressions: unconfirmed conflict response, retired boot re-registration, and readiness ignoring route eligibility. Each was reproduced with a failing test, fixed and followed by passing tests. The integration path now includes the actual coordinator Router and heartbeat-driven exclusion. No remote CI result is inferred from local verification.

Scope: one primary/standby pair and request API foundation. This evidence does not demonstrate three-shard distribution, resource matching, broker processing, promotion/rejoin, complete Kubernetes deployment, worker scaling or final product acceptance. Those checks remain required in subsequent increments.
