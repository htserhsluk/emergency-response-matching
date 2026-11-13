# Verified Day 1 toolchain

Recorded 10 November 2026 (Asia/Calcutta). These are the executed local versions, not a support-policy promise.

| Component | Version |
|---|---|
| Go | 1.27.3, linux/amd64; race tests use Ubuntu WSL2 GCC |
| pgx | v5.12.0; transitive versions in go.sum |
| Goose CLI | v3.29.0; PostgreSQL driver build |
| PostGIS image | postgis/postgis:17-3.7; immutable digest in Compose/CI |
| Docker Engine | 29.9.1, Linux containers through Docker Desktop |
| Docker Compose | 5.6.0 |
| Minikube | 1.39.0, Docker driver, profile erm-lab |
| Kubernetes / kubectl | 1.36.1 |
| Build/runtime images | Immutable Go and distroless digests in infra/Dockerfile |
| CI actions | checkout v4, setup-go v5; remote workflow execution recorded separately |

Host measurement: approximately 16 GB RAM, 16 logical processors. Bootstrap Kubernetes allocation: two CPUs, 3 GiB. This does not establish that the final six-database topology fits; measure it before expansion.

## Day 2 additions

RabbitMQ 4.4.0-management (immutable digest in Compose/Kubernetes/CI), amqp091-go v1.16.0. Publication owns the connection and raw transport deadline; experimental library recovery is disabled. The real broker tests exercise routed confirmations and repeated unroutable returns.

The six-node Kubernetes storage topology and recovery have now run on this host. This is functional evidence, not a throughput benchmark. Full matching-worker scaling and final memory/load measurements remain pending.

## Day 4 browser assets

Leaflet1.9.4 stable is vendored from the version-pinned npm distribution with its license. No npm runtime dependency is needed.
- leaflet.js SHA256: db49d009c841f5ca34a888c96511ae936fd9f5533e90d8b2c4d57596f4e5641a
- leaflet.css SHA256: a7837102824184820dfa198d1ebcd109ff6d0ff9a2672a074b9a1b4d147d04c6

Current manifests use day4 application/migration tags. Schema version6 adds durable notifications. Earlier Day2/Day3 resource statements above are historical; two-worker deployment was demonstrated in Day3, with Day4 regression evidence recorded separately.
