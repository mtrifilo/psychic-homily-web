-- Fix KEXP archive URL casing for the 3 flagship shows.
-- NOTE: Originally numbered 000071, renumbered to 000073 to resolve a
-- duplicate-version collision with 000071_fix_tag_categories (PSY-409).
-- Migration 000068 seeded PascalCase URLs (/shows/The-Morning-Show/, etc.)
-- which return HTTP 500 from kexp.org. Their canonical URLs are lowercase.
-- KEXP's URL casing is inconsistent per-show: other seeded URLs like
-- /shows/Audioasis/ and /shows/El-Sonido/ correctly resolve as-is, so we
-- only update the three that are known broken.

UPDATE radio_shows SET archive_url = 'https://www.kexp.org/shows/the-morning-show/'
WHERE slug = 'the-morning-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp');

UPDATE radio_shows SET archive_url = 'https://www.kexp.org/shows/the-midday-show/'
WHERE slug = 'the-midday-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp');

UPDATE radio_shows SET archive_url = 'https://www.kexp.org/shows/the-afternoon-show/'
WHERE slug = 'the-afternoon-show'
  AND station_id = (SELECT id FROM radio_stations WHERE slug = 'kexp');
