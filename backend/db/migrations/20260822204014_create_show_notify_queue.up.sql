-- PSY-1894: show_notify_queue — the transactional outbox that fires follower
-- notifications for shows that become publicly visible OUTSIDE the admin
-- approval flow (the ingest paths).
--
-- Why an outbox and not a direct call at the write site:
--
--   1. The ingest writers are SEPARATE, SHORT-LIVED PROCESSES. `cmd/discovery-import`
--      runs the DiscoveryService directly against the DB and exits; a
--      fire-and-forget goroutine there would be killed before it delivered.
--      Only a durable row that the long-running server drains can reach the
--      notification path from those processes.
--   2. Notification delivery must never be able to roll back the ingest write.
--      The enqueue runs inside the caller's transaction through a SAVEPOINT
--      (see EnqueueShowNotify), so a failed enqueue rolls back only itself and
--      the show still commits.
--
-- ─────────────────────────────────────────────────────────────────────────────
-- THE NO-BACKFILL GUARANTEE (the load-bearing property of this table)
-- ─────────────────────────────────────────────────────────────────────────────
--
-- On deploy this table is EMPTY, and nothing ever backfills it. A row exists if
-- and only if some code path observed a show BECOME visible after this feature
-- shipped. The ~7,600 shows already in the catalogue therefore have no rows and
-- can never be notified about, no matter how long the poller runs.
--
-- That guarantee is deliberately STRUCTURAL rather than temporal. The obvious
-- alternative — a watermark ("notify shows created after timestamp T" or "after
-- id N") — would make the blast radius depend on clock skew, on when the
-- watermark row was initialised relative to the first poll, and on `created_at`
-- being monotonic with visibility. It is not: `shows` has no `approved_at`
-- column at all, so "when did this become visible" is simply not recorded, and
-- `created_at` is not a proxy for it (a show can be created `pending` and
-- approved months later). An empty table needs none of those assumptions to
-- hold.
--
-- ─────────────────────────────────────────────────────────────────────────────
-- THE DEDUP GUARANTEE
-- ─────────────────────────────────────────────────────────────────────────────
--
-- UNIQUE (show_id) is over ALL statuses, NOT a partial index over the active
-- ones. This is the one place this table intentionally diverges from
-- image_enrich_queue, whose uq_image_enrich_queue_active covers only
-- (pending, processing) so a finished entity can be re-enqueued later.
--
-- Here a terminal row must BLOCK re-enqueue forever: the row IS the durable
-- record of "this show has already been considered for notification". Re-running
-- an ingest, re-importing the same venue calendar, or editing a show must not
-- produce a second notification, and the whole-table UNIQUE plus
-- ON CONFLICT DO NOTHING makes that a no-op at the database rather than a
-- convention the callers have to remember.
--
-- The corollary is that this table is NEVER PRUNED. Pruning terminal rows would
-- re-open re-notification for every pruned show, which is exactly the failure
-- mode the UNIQUE exists to prevent. Growth is bounded at one narrow row per
-- show, the same order as `shows` itself.
--
-- The FK cascade keeps that bounded set honest: a show merged away by
-- cmd/dedup-shows or hard-deleted takes its queue row with it.
--
-- ADDITIVE: one brand-new table; nothing existing is touched. Multi-statement
-- file => golang-migrate wraps it in a transaction => no CREATE INDEX
-- CONCURRENTLY (illegal in a txn, and unnecessary on an empty new table).

CREATE TABLE show_notify_queue (
    id BIGSERIAL PRIMARY KEY,
    show_id BIGINT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'done', 'skipped', 'failed')),
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

-- At most ONE row per show, EVER. See the dedup note above for why this is a
-- whole-table UNIQUE rather than the active-states-only partial index the
-- image-enrich outbox uses.
CREATE UNIQUE INDEX uq_show_notify_queue_show
    ON show_notify_queue (show_id);

-- The poller's claim is `WHERE status='pending' AND attempts < max_attempts
-- ORDER BY created_at LIMIT n FOR UPDATE SKIP LOCKED`. A partial index over only
-- the pending rows, ordered by created_at, covers that hot path and stays tight
-- as terminal rows accumulate (and they accumulate permanently here — nothing
-- prunes them). The stale-`processing` reclaim scans the few in-flight rows and
-- needs no dedicated index.
CREATE INDEX idx_show_notify_queue_pending
    ON show_notify_queue (created_at)
    WHERE status = 'pending';
