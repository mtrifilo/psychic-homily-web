-- Update KEXP Afternoon Show host: Kevin Cole is no longer the current host
-- as of 2026-04-16. Verified via https://www.kexp.org/shows/the-afternoon-show/.

UPDATE radio_shows SET host_name = 'Larry Mizell, Jr.'
WHERE slug = 'the-afternoon-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp')
  AND host_name = 'Kevin Cole';
