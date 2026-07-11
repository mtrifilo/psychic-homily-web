-- Charts cost-lever indexes, shipped alongside the in-process chart cache:
-- the cache bounds request cost, these bound the recompute each cache miss
-- pays.
--
-- Plain (non-CONCURRENT) CREATE INDEX on purpose: golang-migrate wraps this
-- multi-statement file in a transaction, which is incompatible with
-- CONCURRENTLY (same tradeoff the unaccent migration documents). The SHARE
-- lock blocks writes to each table for the build's duration — acceptable
-- here because all five tables are catalog-sized (thousands of rows,
-- sub-second builds), not ingest-sized.
--
-- 1) The new-releases module windows AND orders on
--    COALESCE(release_date, (created_at AT TIME ZONE 'UTC')::date), which no
--    plain-column index serves — every recompute was a seq-scan + top-N sort
--    over releases. The expression matches newReleaseDateSQL
--    (services/catalog/charts_service.go) exactly; the trailing columns match
--    the module's full deterministic ORDER BY, so the index eliminates the
--    top-N sort. (It does NOT bound the scan: the module's COUNT(*) OVER()
--    still reads every qualifying row through the index — fine at catalog
--    scale, but don't assume LIMIT-bounded cost when triaging perf later.)
--    The expression is immutable-safe: timezone(text, timestamptz) with a
--    constant zone and the timestamp->date cast are both IMMUTABLE (a bare
--    timestamptz::date would depend on the session timezone and be rejected).
CREATE INDEX idx_releases_new_release_date
    ON releases ((COALESCE(release_date, (created_at AT TIME ZONE 'UTC')::date)) DESC, created_at DESC, id DESC);

-- 2) The freshly-added ticker runs four per-table
--    ORDER BY created_at DESC, id DESC LIMIT n branches, and the summary's
--    added-in-window counts bound on the same created_at columns. None of
--    these tables had a created_at index, so every recompute full-scanned all
--    four. (shows is not a ticker branch; its summary count also filters on
--    status + is_cancelled and rides the 60s summary cache instead.)
CREATE INDEX idx_artists_created_at_id ON artists (created_at DESC, id DESC);
CREATE INDEX idx_venues_created_at_id ON venues (created_at DESC, id DESC);
CREATE INDEX idx_releases_created_at_id ON releases (created_at DESC, id DESC);
CREATE INDEX idx_radio_stations_created_at_id ON radio_stations (created_at DESC, id DESC);

-- Deliberately NOT indexed: the on-the-radio aired-visibility predicate binds
-- the request instant (volatile), so it cannot be a partial-index predicate.
-- A partial index was only ever the fallback for that module if caching
-- stayed deferred — caching shipped, so on-the-radio leans on the module
-- cache.
