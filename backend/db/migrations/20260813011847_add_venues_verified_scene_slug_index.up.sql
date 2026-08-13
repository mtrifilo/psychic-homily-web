-- PSY-1804: make unresolvable scene-slug lookups index-only.
--
-- ParseSceneSlug (and sceneExists's no-CBSA fallback) match
--   LOWER(REPLACE(city, ' ', '-')) || '-' || LOWER(state) = $slug
-- on verified venues. That expression cannot use a plain city/state index, so a
-- miss was a sequential scan of venues. The expression here is byte-for-byte
-- the same as sceneSlugExprSQL in services/catalog; city and state trail so a
-- miss is an Index Only Scan (no heap fetch) and ORDER BY city, state LIMIT 1
-- can be satisfied from the index.
--
-- Partial on verified = true: unverified rows never match the query.
-- Inlined (not CONCURRENTLY) because golang-migrate wraps this file in a
-- transaction, which is incompatible with CONCURRENTLY (same tradeoff as
-- idx_venues_metro / the charts cost indexes).
CREATE INDEX idx_venues_verified_scene_slug
    ON venues (
        (LOWER(REPLACE(city, ' ', '-')) || '-' || LOWER(state)),
        city,
        state
    )
    WHERE verified = true;
