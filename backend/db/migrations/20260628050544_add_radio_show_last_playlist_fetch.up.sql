-- PSY-1272: per-show last-fetch watermark.
--
-- radio_stations.last_playlist_fetch_at is a single per-STATION watermark, so the
-- incremental fetch (FetchNewEpisodes) computes one `since` for the whole station and
-- advances that watermark whenever ANY show makes progress. A station where most shows
-- succeed but ONE show 404s every run (e.g. a renamed/removed external_id) therefore
-- still advances the station watermark — leaving that one show's episodes that aired
-- during the broken window, but are now older than the lookback floor, unrecoverable by
-- the incremental path (recovery was a manual backfill). This per-SHOW watermark gives
-- each show its own high-water mark so the fetch can compute `since` and advance PER
-- show: a persistently-failing show holds its OWN watermark and recovers its gap once
-- it fetches successfully again.
--
-- The per-station watermark is KEPT (not replaced) as the total-station roll-up that the
-- PSY-1269 sustained-outage janitor (EscalateStaleFetchOutages) reads — distinct,
-- complementary roles. No index: the column is only read as part of an already-loaded
-- radio_shows row and written by advanceShowLastFetch; nothing queries shows by it.
ALTER TABLE radio_shows ADD COLUMN last_playlist_fetch_at TIMESTAMPTZ;

-- Seed each existing show from its station's watermark so the first post-deploy fetch
-- doesn't re-scan the whole window, and a station whose watermark is currently held
-- stale by an in-flight outage (PSY-1241) keeps its catch-up window at deploy time.
-- A NULL station watermark leaves the show watermark NULL = cold-start (fetchSince then
-- falls back to the lookback floor, exactly as a never-fetched station does today).
UPDATE radio_shows rs
   SET last_playlist_fetch_at = st.last_playlist_fetch_at
  FROM radio_stations st
 WHERE rs.station_id = st.id
   AND st.last_playlist_fetch_at IS NOT NULL;
