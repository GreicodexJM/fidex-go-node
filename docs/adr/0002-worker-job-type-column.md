# ADR-0002: Worker job_type — column vs payload-peek dispatch

- **Status:** Proposed
- **Date:** 2026-05-12
- **Deciders:** CTO, Lead Dev Orion (Cristiam)
- **Related ticket:** FID-5

## Context

The FideXNode queue worker (`internal/queue/worker.go`) currently dispatches jobs by **peeking inside the queued JSON payload** for a `job_type` discriminator. After the FID-3 work that added signed J-MDN emission, the dispatch path looks like this:

```go
// deliverMessage — current pattern
var probe struct {
    JobType string `json:"job_type"`
}
_ = json.Unmarshal([]byte(msg.Payload), &probe)
if probe.JobType == "send_jmdn" {
    return w.deliverJMDN(ctx, msg)
}
return w.deliverBusinessDocument(ctx, msg)
```

The `messages` table (`internal/repository/schema.go`) carries the queued payload as an opaque `TEXT` column and indexes only `message_id` and `status`. The worker selects with `ListByStatus(StatusQueued)` and then unmarshals every row to figure out what kind of work it is.

This is fine for two job types. It will not scale. The near-term roadmap adds **at least five more** job kinds:

- `process_inbound` — async handling of received envelopes
- `retry_with_backoff` — generic retry sweeper
- `webhook_notify` — push inbound notifications to ERPs
- `scheduled_cleanup` — purge old QUARANTINED rows
- `key_rotation` — periodic re-key job

Three architectural problems compound as job types grow:

1. **Polling cost.** Every poll cycle fetches every QUEUED row and unmarshals all of them just to route, even when most are irrelevant to a given worker.
2. **No per-type observability.** There is no SQL-level way to ask "how many `send_jmdn` jobs are pending?" without parsing payloads — which makes dashboards, alerts, and capacity planning awkward.
3. **No per-type backpressure / sharding.** Eventually we will want to run a dedicated worker (or pool) for, e.g., `webhook_notify` so a slow ERP doesn't head-of-line-block J-MDN receipts. That requires a SQL-filterable discriminator.

The forcing function for filing this ADR now (rather than after the 3rd job type lands) is that FID-3 already paid the cost of introducing the `job_type` convention in the JSON. We should decide where it lives **before** the convention calcifies in three more call sites.

## Decision

**Promote `job_type` to a first-class column on the queue table**, indexed for poll efficiency. The payload remains a JSON blob for type-specific fields.

Concretely:

- Add a column **`job_type VARCHAR(64) NOT NULL`** to the queue table (today: `messages`; see "Notes" below on whether to split the table later).
- Add a composite index **`(status, job_type, created_at)`** to support the dominant poll query `WHERE status = 'QUEUED' AND job_type = ? ORDER BY created_at`.
- The worker enqueue API stamps `job_type` at write time. The payload **also keeps `job_type` redundantly** for portability (a payload exported to another system stays self-describing).
- Type is `VARCHAR`, not a SQL `ENUM` — adding a new job type must not require a schema migration.
- Dispatch becomes a `switch` on the column value, not a JSON probe. Unknown `job_type` values land in a dead-letter status rather than silently falling through to the business-document path.

This ADR is **Proposed**, not Accepted. The decision will be ratified when the **third** job type lands (likely `process_inbound`), at which point we will either confirm Accepted or pivot to one of the alternatives below if circumstances have changed.

## Consequences

### Positive

- **O(1) dispatch.** The worker stops unmarshalling payloads just to route. JSON parse cost moves from "every QUEUED row, every poll" to "only the row we are about to deliver".
- **Per-type queries become trivial.** `SELECT COUNT(*) FROM messages WHERE status='QUEUED' AND job_type='send_jmdn'` powers dashboards, alerts, and Grafana panels without a payload scan.
- **Future sharding is unlocked.** A `webhook_notify` worker pool can poll independently from the J-MDN pool using the same index.
- **Failure handling can specialise.** Per-`job_type` retry policies, backoff curves, and dead-letter routing become straightforward — the dispatcher already knows the kind before it touches the payload.
- **Type-safe enqueue.** The enqueue API can validate `job_type` against a known constants set (`constants/jobtypes.go`) at write time rather than at delivery time.

### Negative / costs

- **One-time migration.** SQLite `ALTER TABLE ADD COLUMN` is cheap, but backfilling `job_type` on existing rows requires a one-off job (peek the payload, set the column, drop default). Trivially testable, but not zero.
- **Slight row-size growth.** ~32 bytes/row average, immaterial at our cardinality.
- **Write-time discipline.** Every enqueue site must now stamp `job_type`. Mitigated by funneling all enqueues through one repository method.
- **Index maintenance.** The `(status, job_type, created_at)` index costs writes proportional to enqueue rate. Acceptable: enqueue is not the hot path; delivery is.
- **Duplication risk.** `job_type` lives in both column and payload. The column is the source of truth for dispatch; the payload copy is informational only. The repository layer enforces that they match on insert.

## Alternatives considered

- **Keep payload-peek dispatch.** Status quo. Rejected: it does not survive 5+ job types. The unmarshal-everything cost scales linearly with queue depth, and there is no SQL hook for per-type observability or sharding.

- **One table per job type** (`outbound_envelopes`, `outbound_jmdns`, `inbound_webhooks`, …). Rejected: explodes coupling. Every new job type forces a new table, new repository, new migration, new worker poll path. The reason a single queue table exists is precisely to keep dispatch policy in one place. Multi-table fan-out belongs at the worker-pool layer (multiple workers, one table), not the schema layer.

- **SQL `ENUM` / `CHECK` constraint on `job_type`.** Rejected: every new job type would require a schema migration in production. We expect the job-type set to keep growing for at least a year. `VARCHAR(64)` with a Go-side constants set (`constants.JobType*`) gives us the typo-safety of an enum at compile time without the operational cost at runtime. The check shifts left, where it is cheaper.

- **Job-type as URN in `message_id`** (e.g., `urn:fidex:job:send_jmdn:UUID`). Rejected: overloads a primary key with routing semantics. `message_id` is already the FideX envelope identifier exposed to partners; bending it for internal dispatch is a layering violation and would leak the dispatch taxonomy to peers.

- **Separate `queue` table, leave `messages` alone.** Rejected for now (but kept as a future option). The cleanest long-term shape is probably "`messages` is the canonical envelope log; `queue` is the dispatcher's worklist" with the two joined by `message_id`. That is a bigger surgery than this ADR justifies. Re-evaluate when (a) we add a job type that has no corresponding business message (`scheduled_cleanup`, `key_rotation`) or (b) queue depth crosses a measurable threshold.

## References

- **FID-3** — J-MDN emission; introduced the second job type and made this question concrete.
- **FID-5** — this ADR's tracking ticket.
- `internal/queue/worker.go` — `deliverMessage`, `deliverBusinessDocument`, `deliverJMDN`, and the `queuedJMDNJob` / `queuedOutboundPayload` shapes.
- `internal/repository/schema.go` — current `messages` table definition and its indexes.
- FideX AS5 spec §7 — disposition notification semantics (informs the `send_jmdn` job type but does not constrain dispatch).

## Notes for next visitor

- The decision to keep the queue in a single table (vs splitting) is **deliberately deferred**. If `scheduled_cleanup` or `key_rotation` lands and the row shape diverges meaningfully from outbound envelopes, revisit the "separate queue table" alternative.
- The implementation ticket is a separate Historia (linked from FID-5). Do **not** implement the schema change as part of accepting this ADR — accept the ADR, then schedule the migration with the same review rigor as any other DB change.
- If we ever move from SQLite to Postgres (the spec does not preclude this), the same shape (`VARCHAR` + composite index) carries over without change. That is a deliberate design constraint of this ADR.
