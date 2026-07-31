-- PSY-1562: consolidate playlist-fetch eligibility into one write-time-enforced
-- predicate.
--
-- playlist_backfill_attempted_at is the durable memo of the last POST-AIR playlist
-- fetch attempt. Without it there was no rate limit at all: an episode found empty was
-- re-selected by the very next hourly sweep, which is how 23 stranded episodes produced
-- 269 zero-yield backfill runs a day on production. It mirrors location_enrich_attempted_at
-- (PSY-1250) and is deliberately separate from playlist_fetched_at, which ANY fetch
-- stamps including a live-window refresh — only post-air attempts are cooled down.
ALTER TABLE radio_episodes
    ADD COLUMN playlist_backfill_attempted_at TIMESTAMPTZ;

-- Seed the memo from the existing last-fetch stamp so the deploy doesn't hand every
-- historical episode a free immediate retry at once. Conservative in the right
-- direction: where playlist_fetched_at came from a LIVE fetch this delays that
-- episode's first post-air attempt by at most one cooldown, once.
UPDATE radio_episodes
SET playlist_backfill_attempted_at = playlist_fetched_at
WHERE playlist_fetched_at IS NOT NULL;

-- Partial index for the backfill candidate scan, which now also filters on the memo.
-- Partial on "not complete" because complete episodes are the overwhelming majority
-- and are never candidates.
CREATE INDEX idx_radio_episodes_backfill_candidates
    ON radio_episodes (air_date DESC, playlist_backfill_attempted_at)
    WHERE playlist_state <> 'complete';

-- Converge the PSY-1285 invariant immediately instead of waiting for each row's next
-- re-list. RederivePlaylistState enforces it on every write path, so this is not
-- load-bearing — but a scheduled episode that is never re-listed again would otherwise
-- keep displaying a give-up it cannot have earned, since its playlist legitimately does
-- not exist yet.
UPDATE radio_episodes
SET playlist_state = 'pending',
    playlist_fetch_attempts = 0,
    updated_at = NOW()
WHERE starts_at IS NOT NULL
  AND starts_at > NOW()
  AND play_count = 0
  AND (playlist_state = 'unavailable' OR playlist_fetch_attempts > 0);
