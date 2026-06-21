-- PSY-1149: shared source-config registry for stale-first catalog refresh
-- (Catalog Refresh M2). One polymorphic table tracking an external source per
-- catalog entity — a venue's calendar page or a label's roster page — so the
-- refresh loop (M3) can pick the stalest sources first across BOTH entity types.
--
-- Deliberately DECOUPLED from the legacy venue_source_configs (which is tied to
-- the retiring AI extraction pipeline — PSY-1158). The executor here is the
-- /ingest skill (agent / upstream-API extraction), which stamps last_refreshed_at
-- after each run. See docs/open-questions/catalog-refresh-venue-pipeline-reconciliation.md.
--
-- Polymorphic over catalog entities like the comments/reports tables: a single
-- FK can't reference two parent tables, so there is intentionally NO FK on
-- entity_id. Orphan rows on entity delete are the app's responsibility to clean
-- up (acceptable for a registry). entity_type uses a CHECK (not a Postgres enum)
-- so new source kinds are cheap to add — matching the recent radio-schema choice.
--
-- ADDITIVE: one brand-new table; nothing existing is touched. Multi-statement
-- file => golang-migrate wraps it in a transaction => no CREATE INDEX
-- CONCURRENTLY (illegal in a txn, and unnecessary on an empty new table).

CREATE TABLE source_configs (
    id BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(20) NOT NULL CHECK (entity_type IN ('venue', 'label')),
    entity_id BIGINT NOT NULL,
    source_url TEXT,
    last_refreshed_at TIMESTAMPTZ,
    last_content_hash VARCHAR(64),
    consecutive_failures INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (entity_type, entity_id)
);

-- Stale-first ordering for the refresh loop: never-refreshed rows (NULL) sort
-- first, then oldest last_refreshed_at.
CREATE INDEX idx_source_configs_stale ON source_configs (last_refreshed_at ASC NULLS FIRST);
