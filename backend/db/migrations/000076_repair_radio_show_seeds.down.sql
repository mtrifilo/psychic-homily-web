-- Revert PSY-402 + PSY-408 radio seed repairs.
-- NOTE: this restores the original (broken) seed values from migration 000068.
-- Two shows that were DELETED on the up path are re-inserted with their
-- original fabricated data — any child rows (episodes, tracks) that existed
-- prior to the up migration are NOT restored, since they were cascaded away.

-- WFMU reverts ------------------------------------------------------------
UPDATE radio_shows
SET name = 'Trouble',
    slug = 'trouble-wfmu',
    description = 'Eclectic freeform with an emphasis on soul, jazz, international, and the unexpected.',
    schedule_display = 'Wednesdays 10 AM-1 PM ET',
    updated_at = NOW()
WHERE slug = 'give-the-drummer-some-wfmu'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

UPDATE radio_shows
SET name = 'Bodega Pop Live',
    slug = 'bodega-pop-live-wfmu',
    host_name = 'Mike Lupica',
    description = 'A window into the sounds of immigrant communities in New York City and beyond.',
    schedule_display = 'Saturdays 10 AM-1 PM ET',
    archive_url = 'https://wfmu.org/playlists/MX',
    external_id = 'MX',
    updated_at = NOW()
WHERE slug = 'bodega-pop-wfmu'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

UPDATE radio_shows
SET archive_url = 'https://wfmu.org/playlists/FW',
    external_id = 'FW',
    updated_at = NOW()
WHERE slug = 'downtown-soulville-wfmu'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active)
VALUES
    ((SELECT id FROM radio_stations WHERE slug = 'wfmu'), 'The Best Show', 'the-best-show-wfmu', 'Tom Scharpling', 'Comedy and music variety show, one of the longest-running call-in shows in radio history.', 'Tuesdays 9 PM-12 AM ET', 'https://wfmu.org/playlists/TS', 'TS', TRUE)
ON CONFLICT (slug) DO NOTHING;

-- NTS reverts -------------------------------------------------------------
UPDATE radio_shows
SET name = 'Charlie Bones',
    archive_url = 'https://www.nts.live/shows/charlie-bones',
    external_id = 'charlie-bones',
    updated_at = NOW()
WHERE slug = 'charlie-bones-nts'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'nts-radio');

INSERT INTO radio_shows (station_id, name, slug, host_name, description, schedule_display, archive_url, external_id, is_active)
VALUES
    ((SELECT id FROM radio_stations WHERE slug = 'nts-radio'), 'Brownswood Basement', 'brownswood-basement-nts', 'Gilles Peterson', 'Gilles Peterson digs deep into jazz, beats, soul, and global sounds.', 'Bi-weekly', 'https://www.nts.live/shows/brownswood-basement', 'brownswood-basement', TRUE)
ON CONFLICT (slug) DO NOTHING;
