-- Revert PSY-510: remove the two shows.
-- NOTE: child radio_episodes for these shows are cascaded away on DELETE. If
-- those episodes matter in your environment, export them before rolling back.

DELETE FROM radio_shows
WHERE slug = 'three-chord-monte-wfmu'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'wfmu');

DELETE FROM radio_shows
WHERE slug = 'breakfast-show-nts'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'nts-radio');
