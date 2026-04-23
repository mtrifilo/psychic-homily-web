-- PSY-510: seed "Three Chord Monte" (WFMU) and "The NTS Breakfast Show" (NTS).
-- Both surfaced during PSY-406 dogfooding as partially-imported Discover rows
-- (slug 'three-chord-monte' on WFMU, 'breakfast-show' on NTS) with real
-- metadata missing. Research on 2026-04-22 confirmed both are current active
-- shows; see PSY-510 for evidence.
--
-- Idempotent shape: UPDATE path converts existing zombie rows in place
-- (preserving child radio_episodes), INSERT path covers fresh environments
-- where Discover hasn't run yet. In prod the UPDATEs are no-ops and the
-- INSERTs do the work; in dev with zombies the UPDATEs rename + enrich and
-- the INSERTs ON CONFLICT DO NOTHING.

-- WFMU: Three Chord Monte (Joe Belock, code TM) ---------------------------
UPDATE radio_shows
SET slug             = 'three-chord-monte-wfmu',
    name             = 'Three Chord Monte',
    host_name        = 'Joe Belock',
    description      = 'Garage, punk, and power pop from longtime WFMU DJ Joe Belock.',
    schedule_display = 'Mondays 12 PM-3 PM ET',
    archive_url      = 'https://wfmu.org/playlists/TM',
    external_id      = 'TM',
    updated_at       = NOW()
WHERE slug = 'three-chord-monte'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active)
VALUES
    ((SELECT id FROM radio_stations WHERE slug = 'wfmu'),
     'Three Chord Monte',
     'three-chord-monte-wfmu',
     'Joe Belock',
     'Garage, punk, and power pop from longtime WFMU DJ Joe Belock.',
     'Mondays 12 PM-3 PM ET',
     'https://wfmu.org/playlists/TM',
     'TM',
     TRUE)
ON CONFLICT (slug) DO NOTHING;

-- NTS: The NTS Breakfast Show (rotating residents, slug "breakfast") ------
-- host_name stays NULL intentionally — the show rotates through Louise Chen,
-- Flo, Zakia, Coco María, and others. Filling it with any single DJ would
-- misrepresent the show.
UPDATE radio_shows
SET slug             = 'breakfast-show-nts',
    name             = 'The NTS Breakfast Show',
    host_name        = NULL,
    description      = 'Daily morning show on NTS, rotating through residents including Louise Chen, Flo, Zakia, Coco María, and others.',
    schedule_display = 'Weekdays, mornings GMT',
    archive_url      = 'https://www.nts.live/shows/breakfast',
    external_id      = 'breakfast',
    updated_at       = NOW()
WHERE slug = 'breakfast-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'nts-radio');

INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active)
VALUES
    ((SELECT id FROM radio_stations WHERE slug = 'nts-radio'),
     'The NTS Breakfast Show',
     'breakfast-show-nts',
     NULL,
     'Daily morning show on NTS, rotating through residents including Louise Chen, Flo, Zakia, Coco María, and others.',
     'Weekdays, mornings GMT',
     'https://www.nts.live/shows/breakfast',
     'breakfast',
     TRUE)
ON CONFLICT (slug) DO NOTHING;
