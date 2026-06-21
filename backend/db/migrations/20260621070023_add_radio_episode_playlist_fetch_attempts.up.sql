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

-- One-time rollout heal. playlist_state has existed since migration 20260619020546
-- with a 'pending' DEFAULT, but nothing ever advanced it, so EVERY existing episode —
-- including ones whose playlist was fully imported long ago — currently sits at
-- 'pending'. Two consequences this fixes:
--   1. ComputeEpisodeStatus derives 'archived' from playlist_state='complete', so an
--      aired episode that already has tracks reads as merely 'aired' forever — the
--      inverse of the empty-archived bug this ticket targets.
--   2. Left as-is, the post-air backfill sweep would re-fetch these already-complete
--      playlists once at rollout (bounded + idempotent, but wasted provider load).
-- Heal only episodes that aired BEFORE the default 7-day backfill lookback: their
-- playlist is final (a provider won't add plays days later), so play_count>0 ⇒ the
-- playlist was imported ⇒ complete (design §6: aired + playlist = complete). Episodes
-- WITHIN the lookback are deliberately left 'pending' so the sweep re-fetches them —
-- that path correctly completes a caught-live partial that a bulk heal can't tell
-- apart from a finished one. status is recomputed on read, but the snapshot is set
-- here too for consistency (all matched rows are aired+complete → archived). Older
-- stragglers beyond what the sweep covers are the janitor's job (PSY-1155).
UPDATE radio_episodes
SET playlist_state = 'complete', status = 'archived'
WHERE play_count > 0
  AND playlist_state = 'pending'
  AND (
        (ends_at IS NOT NULL AND ends_at < NOW() - INTERVAL '7 days')
     OR (ends_at IS NULL AND air_date < CURRENT_DATE - 7)
  );
