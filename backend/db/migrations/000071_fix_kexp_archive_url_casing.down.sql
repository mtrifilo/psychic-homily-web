-- Revert archive URLs to the original (broken) PascalCase form.

UPDATE radio_shows SET archive_url = 'https://www.kexp.org/shows/The-Morning-Show/'
WHERE slug = 'the-morning-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp');

UPDATE radio_shows SET archive_url = 'https://www.kexp.org/shows/The-Midday-Show/'
WHERE slug = 'the-midday-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp');

UPDATE radio_shows SET archive_url = 'https://www.kexp.org/shows/The-Afternoon-Show/'
WHERE slug = 'the-afternoon-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp');
