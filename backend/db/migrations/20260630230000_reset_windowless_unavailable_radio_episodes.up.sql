-- PSY-1287: one-time reopen of windowless aired episodes that falsely gave up on
-- playlist fetch before their schedule could be matched (PSY-1283 off-by-one).
--
-- Such rows were windowless → immediately 'aired' → post-air backfill burned attempts
-- on empty pre-air playlists → 'unavailable', which then EXCLUDED the show from the
-- backfill candidate query (a catch-22). The code fix (NormalizeWindowHealPlaylistState
-- + NormalizeStrandedWindowlessPlaylistState) prevents recurrence; this brings already-
-- stored rows forward so the next backfill tick can heal the window and fetch.
--
-- Scope: windowless, zero plays, exhausted — the F4 "Freeform Jazz Dance" shape. Rows
-- with imported plays are left alone. Idempotent.
UPDATE radio_episodes
   SET playlist_state          = 'pending',
       playlist_fetch_attempts = 0,
       updated_at              = now()
 WHERE starts_at IS NULL
   AND playlist_state = 'unavailable'
   AND play_count = 0
   AND air_date >= CURRENT_DATE - INTERVAL '30 days';
