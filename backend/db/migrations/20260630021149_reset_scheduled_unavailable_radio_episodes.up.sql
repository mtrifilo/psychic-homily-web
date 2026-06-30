-- PSY-1285: one-time correction of radio episodes stranded in a not-yet-aired
-- (scheduled) state with a terminal 'unavailable' playlist_state or burned backfill
-- attempts.
--
-- A windowless episode is settled to 'aired', so the post-air backfill burns attempts
-- on it → 'unavailable'; once it is given a FUTURE window (PSY-1283's schedule correction,
-- or a heal-on-relist) it is 'scheduled' but its playlist_state stays stuck 'unavailable'.
-- The code fix (NormalizeScheduledPlaylistState, called from reimportExistingEpisode)
-- resets such a row on its next re-list — but a row whose airtime passes before a re-list
-- lands would never be corrected while scheduled. This brings the ALREADY-stored rows
-- forward immediately — the same posture as the PSY-1283 schedule correction — so the AC
-- "a scheduled episode is never 'unavailable'" holds at deploy, not just for future episodes.
--
-- Scope (mirrors NormalizeScheduledPlaylistState exactly): episodes whose frozen window is
-- still in the FUTURE (starts_at > now → genuinely not-yet-aired) carrying a terminal
-- 'unavailable', OR burned attempts on a still-'pending' row. A scheduled 'partial'/'complete'
-- carries real plays and is left intact. A WINDOWLESS ('aired') episode that is 'unavailable'
-- is the legitimate no-playlist case (PSY-1287), NOT this invariant, and is left alone
-- (starts_at IS NOT NULL excludes it). status is recomputed on read, but the stored snapshot
-- is set to 'scheduled' for consistency. Idempotent: re-running, or running where no such row
-- exists, updates 0 rows.
UPDATE radio_episodes
   SET playlist_state          = 'pending',
       playlist_fetch_attempts = 0,
       status                  = 'scheduled',
       updated_at              = now()
 WHERE starts_at IS NOT NULL
   AND starts_at > now()
   AND (playlist_state = 'unavailable'
        OR (playlist_state = 'pending' AND playlist_fetch_attempts > 0));
