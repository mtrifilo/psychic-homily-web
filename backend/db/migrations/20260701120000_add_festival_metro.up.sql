-- PSY-1278: denormalized CBSA metro key for festivals, mirroring the
-- artists/venues treatment from PSY-1255 step B (20260628025046_add_entity_metro).
--
-- Scenes are keyed by US Census CBSA metro, but festivals carried no `metro`
-- column, so GetSceneDetail's festival_count matched only the metro's PRINCIPAL
-- city/state — under-counting festivals held in other member cities (e.g. a
-- St. Paul festival not counting toward the Minneapolis-principal Twin Cities
-- scene). The value is DERIVED from (city, state, country) via geo.ResolveMetro:
-- set on the festival write paths and reconciled by cmd/backfill-entity-metro.
-- NULL for non-US, not-in-any-CBSA, and ambiguous-unpinned places.
ALTER TABLE festivals ADD COLUMN metro VARCHAR(10);

-- Partial index, matching idx_artists_metro/idx_venues_metro: the scene count
-- matches `metro = $cbsaCode`, which never matches NULL, so excluding the NULL
-- tail keeps the index small. Inlined (not CONCURRENTLY) for the same reason as
-- the step-B migration: multi-statement/transactional, and festivals is a small
-- table today.
CREATE INDEX idx_festivals_metro ON festivals (metro) WHERE metro IS NOT NULL;
