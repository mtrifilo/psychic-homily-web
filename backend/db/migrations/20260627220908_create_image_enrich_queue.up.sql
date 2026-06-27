-- PSY-1247: image_enrich_queue — the transactional-outbox for PROMPT, on-create
-- image enrichment (Phase B of the PSY-1245 hybrid trigger model). The single
-- catalog create funnel (PSY-1254) enqueues a job row in the SAME transaction as
-- a newly-created artist/release; the imageenrich outbox poller drains it and runs
-- the shipped fill-when-empty enrichers, so a new entity gets its image within
-- ~one poll interval instead of waiting for the slow daily Phase-A sweep
-- (PSY-1246). The two coexist: outbox = prompt path for new entities; sweep =
-- backfill + re-attempt-the-tail + safety net (idempotent via fill-when-empty +
-- the artists/releases.image_enrich_attempted_at memo).
--
-- entity_id is POLYMORPHIC (an artist id OR a release id, keyed by entity_type),
-- so there is intentionally NO foreign key — mirroring the polymorphic
-- source_configs choice. A row whose entity is deleted is harmless: the poller's
-- enricher loads the entity by id and a missing one is simply skipped.
--
-- CHECK (not Postgres enums) on the small closed sets so adding an entity_type or
-- status later is a cheap ALTER, matching the recent radio / source-config /
-- artist_link_suggestions schema choice.
--
-- ADDITIVE: one brand-new table; nothing existing is touched. Multi-statement file
-- => golang-migrate wraps it in a transaction => no CREATE INDEX CONCURRENTLY
-- (illegal in a txn, and unnecessary on an empty new table).

CREATE TABLE image_enrich_queue (
    id BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(20) NOT NULL CHECK (entity_type IN ('artist', 'release')),
    entity_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

-- At most ONE active (pending|processing) job per entity. Makes the enqueue
-- idempotent: a re-enqueue (double create-call, manual replay) is a no-op via
-- ON CONFLICT DO NOTHING, and a finished (done|failed) row never blocks a future
-- re-enqueue of the same entity. The predicate is over the ACTIVE states only so
-- the index stays tiny as done/failed rows accumulate.
CREATE UNIQUE INDEX uq_image_enrich_queue_active
    ON image_enrich_queue (entity_type, entity_id)
    WHERE status IN ('pending', 'processing');

-- The poller's claim is `WHERE status='pending' AND attempts < max_attempts
-- ORDER BY created_at LIMIT n FOR UPDATE SKIP LOCKED`. A partial index over only
-- the pending rows, ordered by created_at, covers the hot path and stays tight as
-- done/failed rows accumulate. (The stale-`processing` reclaim scans the few
-- in-flight rows and needs no dedicated index.)
CREATE INDEX idx_image_enrich_queue_pending
    ON image_enrich_queue (created_at)
    WHERE status = 'pending';
