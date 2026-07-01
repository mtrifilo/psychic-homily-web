-- PSY-1287: one-time correction of WINDOWLESS radio episodes stranded 'unavailable' (or with
-- burned backfill attempts) by the pre-fix bug that treated a windowless episode as 'aired'
-- regardless of its air_date.
--
-- A windowless episode (no schedule slot → no frozen air window) was settled to 'aired' the
-- moment it was imported, even when its air_date was still in the FUTURE — the WFMU importer
-- ingests upcoming broadcasts ahead of airtime. The post-air backfill then burned all of its
-- attempts fetching an unpublished playlist and gave up to 'unavailable' BEFORE the broadcast,
-- after which the episode was never re-fetched once it actually aired. The code fix makes
-- ComputeEpisodeStatus air_date-aware (a windowless episode is 'scheduled' until its air_date
-- day has passed in the broadcaster's timezone — see windowlessNotYetAired, the Go source of
-- truth this CASE mirrors), so these never strand going forward, and NormalizeScheduledPlaylistState
-- resets a future-dated one on its next re-list.
--
-- This brings the ALREADY-stored backlog forward — but ONLY the rows a sweep can still act on:
-- those whose air_date is within the NIGHTLY JANITOR's straggler-backfill reach
-- (DefaultJanitorBackfillLookbackDays = 30; radio_fetch_service.go) or in the future. 'unavailable'
-- is reset to 'pending' nowhere else (ShouldBackfillPlaylist excludes it, so neither the dedicated
-- sweep nor a re-list ever recovers it), so without this reset those rows stay hidden forever; once
-- reset to 'pending', the 30-day janitor sweep re-fetches them — a publishing show finally gets the
-- playlist it was denied; a show that genuinely never publishes a tracklist correctly re-settles to
-- 'unavailable' over the next few nightly cycles (accepted one-time churn — the reset can't tell a
-- bug-stranded row from a legitimately-empty one without per-row history). Rows older than the
-- janitor reach are intentionally LEFT hidden (PSY-1285): no sweep would pick them up even if reset.
--
-- Dates use America/New_York (WFMU's broadcast day) to match the Go air-phase boundary, NOT UTC.
-- This is the exact COMPLEMENT of the PSY-1285 correction migration (which reset SCHEDULED/windowed
-- strands and explicitly excluded windowless rows as PSY-1287's concern). Idempotent: re-running, or
-- running where no such row exists, updates 0 rows.
UPDATE radio_episodes
   SET playlist_state          = 'pending',
       playlist_fetch_attempts = 0,
       status                  = CASE WHEN air_date < (now() AT TIME ZONE 'America/New_York')::date
                                      THEN 'aired' ELSE 'scheduled' END,
       updated_at              = now()
 WHERE starts_at IS NULL
   AND play_count = 0
   AND air_date >= (now() AT TIME ZONE 'America/New_York')::date - 30
   AND (playlist_state = 'unavailable'
        OR (playlist_state = 'pending' AND playlist_fetch_attempts > 0));
