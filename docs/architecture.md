# Architecture and design decisions

The API accepts requests and serves the embedded browser. All business storage operations pass through one coordinator to the owning PostgreSQL/PostGIS shard. Three independent shards each have a synchronous streaming standby. The storage-node agent reports database role/reachability and applies route fencing. RabbitMQ carries matching work from the API's outbox publisher to one or more Matching workers.

## Choices

- Go with `net/http`: small deployable binaries, bounded contexts, straightforward concurrency and race testing without a separate web framework.
- PostgreSQL/PostGIS: transactions and constraints protect state; geography distance evaluates location in metres; streaming replication supplies a physically separate copy. pgx is the SQL client and Goose manages immutable migrations.
- Hash partitioning: IDs are 32 lowercase hex characters. Entity IDs derive from SHA-256 of operation kind and idempotency key. The shard is the first eight SHA-256 bytes of the ID, interpreted as unsigned big endian, modulo three. Golden vectors live in `internal/routing/hash_test.go`. Clients use the same public URL after promotion.
- Independent data on each shard: entities are partitioned, not copied to all three shards. Each shard's two members replicate the same dataset. Cross-shard resource searches require all shards, merge nearest candidates and use stable ID tie-breaking; an unavailable shard cannot be mistaken for an empty one.
- Geographic matching: exact supported type, available resource, nearest within 5 km then 10 km. This is straight-line geography, not road routing, capacity optimization or emergency triage.
- RabbitMQ: one persistent broker, durable topic exchanges `emergency.events`/`emergency.dead`, queues `matching.work`/`matching.dlq`, routing keys `request.match`/`request.dead`. Persistent messages, mandatory publication and confirms precede marking an outbox delivery sent. Consumers use prefetch1/manual acknowledgements.
- Kubernetes: Deployments for stateless services, StatefulSets/PVCs for databases and broker, internal Services for communication. Two business services are API and Matching; coordinator and node agents implement the storage layer. Scale Matching only; one coordinator owns request serialization.
- Logs/health: bounded structured diagnostics omit request bodies and credentials. Process liveness differs from full dependency readiness; API serving remains reachable for healthy-shard reads during partial outages. Worker readiness follows its actual AMQP session.

## Durability and transaction boundaries

Request creation and its initial outbox row share a local transaction. Mutation acknowledgement requires synchronous replay (`remote_apply`), including idempotent conflict/replay paths that write a durability barrier. A visible primary row alone cannot authorize a cross-shard step after an ambiguous commit. Retry the same operation identity after unavailable responses.

Assignment uses a durable request-shard intent before attempting resource-shard reservation. The resource transaction records either a single successful owner or definite rejection. Unknown outcomes retain the original assignment and resource identity. A definite rejection permits another candidate; uncertainty does not. Request finalization and its unique `demo` inbox notification commit together. Completion retains resource allocation.

There is no distributed atomic SQL transaction and no exactly-once broker delivery claim. Idempotent durable steps, unique constraints, retry state and reconciliation provide convergence. The coordinator uses 256 cancellable request locks; workers can run concurrently on different requests, with reservation constraints deciding resource contention.

Transient attempts have persistent 2/5/15-second retries. Exhaustion pauses the workflow and records a dead-letter delivery. Reconciliation obeys the same pause and due time. Operator redrive continues the original intent with an additional bounded budget. Pause is allowed before a future retry is due. Malformed/unknown deliveries are acknowledged only after confirmed dead-letter publication.

## Recovery and limits

Heartbeats expire after eight seconds. Controlled failover excludes/fences the old primary, requires a caught-up replica, promotes it, persists route version and restores service. Rejoin rebuilds the old member from the new primary, preserving old data/journal artifacts and then restoring paired synchronous replay. It never automatically flips roles back. See [recovery](recovery.md) for exact checks and failure cleanup.

One host, coordinator and broker remain failure domains. A missing synchronous copy normally blocks confirmed writes; explicit local degraded mode temporarily trades durability for availability. No arbitrary partition tolerance, automatic election, multi-host HA, production authentication or guaranteed emergency-service suitability is claimed. The shared demonstration inbox is not private messaging. Optional map tiles can fail without preventing coordinate/text workflow.

The measured small workload is functional evidence only. Both contributors must separately study and explain these decisions; a passing implementation does not demonstrate that learning.
