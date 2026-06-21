-- PSY-1154: post-air playlist backfill loop. The backfill loop re-fetches an
-- aired episode whose playlist is still incomplete (playlist_state pending/partial)
-- until it completes — but a playlist the provider never publishes (a legitimately
-- empty broadcast, a show pulled from the provider) must not be retried forever.
-- This per-episode counter lets the loop give up after N failed post-air attempts
-- and mark playlist_state='unavailable' (the attempt cap is RadioBackfillMaxAttempts
-- in code, default 5).
--
-- ADDITIVE: one column on an existing table. A constant DEFAULT means Postgres adds
-- it without a table rewrite. Multi-statement file => golang-migrate wraps it in a
-- transaction, so no CREATE INDEX CONCURRENTLY is involved (and none is needed — the
-- backfill candidate query runs at most hourly and radio_episodes is small).
ALTER TABLE radio_episodes
    ADD COLUMN playlist_fetch_attempts INTEGER NOT NULL DEFAULT 0,
    ADD CONSTRAINT radio_episodes_playlist_fetch_attempts_check
        CHECK (playlist_fetch_attempts >= 0);
