-- PSY-1500: collection_feature_runs — a journal of collection-featuring stints.
-- collections.is_featured is a bare boolean: flipping it destroys history and
-- cannot order multiple featured collections for PSY-1411's "most recently
-- featured" lock. This interval table records one row per featuring stint
-- (mirrors radio_sync_runs, the house lifecycle pattern). is_featured stays as
-- a denormalised "has an open run" flag; SetFeatured writes both in one
-- transaction so they cannot drift.
--
-- ADDITIVE: one brand-new table + a backfill INSERT. Multi-statement =>
-- golang-migrate wraps in a transaction => no CREATE INDEX CONCURRENTLY.

CREATE TABLE collection_feature_runs (
    id BIGSERIAL PRIMARY KEY,
    collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    -- featured_at is when the stint opened; unfeatured_at NULL = still featured.
    featured_at TIMESTAMPTZ NOT NULL,
    unfeatured_at TIMESTAMPTZ,
    -- Actors are soft references: deleting the admin who featured a collection
    -- must not delete the historical run, so ON DELETE SET NULL.
    featured_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    unfeatured_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    -- featured_at_estimated marks a backfilled start reconstructed from
    -- collections.created_at rather than an observed audit event; the archive
    -- must not render a precise date for an estimated row.
    featured_at_estimated BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- At most one OPEN run per collection; closed runs are unconstrained so
-- re-featuring the same collection is legal and history accrues.
CREATE UNIQUE INDEX collection_feature_runs_one_open
    ON collection_feature_runs (collection_id) WHERE unfeatured_at IS NULL;

-- The archive's only ordering (newest-first) and the live-pick's LIMIT 1.
CREATE INDEX collection_feature_runs_featured_at_desc
    ON collection_feature_runs (featured_at DESC);

-- Backfill: open exactly one run per currently-featured collection.
--   Path 1 — the newest audit_logs 'set_collection_featured' event whose
--     metadata.featured is true and metadata.slug matches the collection's
--     current slug: featured_at_estimated = false (observed).
--   Path 2 — failing that (the entity_id:0 audit defect PSY-1502 + mutable
--     slugs mean path 1 is not always available): collections.created_at with
--     featured_at_estimated = true (reconstructed).
-- Closed historical runs are deliberately NOT reconstructed — pre-migration
-- unfeaturing events are genuinely lost and inventing them would be fiction.
INSERT INTO collection_feature_runs (collection_id, featured_at, featured_at_estimated, created_at)
SELECT
    c.id,
    COALESCE(a.featured_at, c.created_at),
    (a.featured_at IS NULL),
    NOW()
FROM collections c
LEFT JOIN LATERAL (
    SELECT al.created_at AS featured_at
    FROM audit_logs al
    WHERE al.action = 'set_collection_featured'
      AND al.metadata->>'featured' = 'true'
      AND al.metadata->>'slug' = c.slug
    ORDER BY al.created_at DESC
    LIMIT 1
) a ON true
WHERE c.is_featured = true;
