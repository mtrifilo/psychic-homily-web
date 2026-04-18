-- Revert Afternoon Show host back to the previous seeded value.

UPDATE radio_shows SET host_name = 'Kevin Cole'
WHERE slug = 'the-afternoon-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp')
  AND host_name = 'Larry Mizell, Jr.';
