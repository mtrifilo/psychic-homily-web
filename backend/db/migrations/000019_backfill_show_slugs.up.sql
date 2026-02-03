-- Backfill show slugs for existing shows
-- Format for shows WITH title: title-YYYY-MM-DD
-- Format for shows WITHOUT title: headliner-at-venue-YYYY-MM-DD

-- First, update shows WITH titles
UPDATE shows
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(
            CONCAT(title, '-', TO_CHAR(event_date, 'YYYY-MM-DD')),
            '[^a-zA-Z0-9\s-]', '', 'g'
        ),
        '\s+', '-', 'g'
    )
)
WHERE (slug IS NULL OR slug = '')
  AND title IS NOT NULL
  AND title != '';

-- Then, update shows WITHOUT titles using headliner-at-venue-date
UPDATE shows s
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(
            CONCAT(
                COALESCE(a.name, 'show'),
                '-at-',
                COALESCE(v.name, 'venue'),
                '-',
                TO_CHAR(s.event_date, 'YYYY-MM-DD')
            ),
            '[^a-zA-Z0-9\s-]', '', 'g'
        ),
        '\s+', '-', 'g'
    )
)
FROM (
    SELECT DISTINCT ON (sa.show_id) sa.show_id, art.name
    FROM show_artists sa
    JOIN artists art ON art.id = sa.artist_id
    ORDER BY sa.show_id, sa.position ASC
) a,
(
    SELECT DISTINCT ON (sv.show_id) sv.show_id, ven.name
    FROM show_venues sv
    JOIN venues ven ON ven.id = sv.venue_id
    ORDER BY sv.show_id
) v
WHERE s.id = a.show_id
  AND s.id = v.show_id
  AND (s.slug IS NULL OR s.slug = '')
  AND (s.title IS NULL OR s.title = '');

-- Handle any remaining shows without artists/venues
UPDATE shows
SET slug = CONCAT('show-', id, '-', TO_CHAR(event_date, 'YYYY-MM-DD'))
WHERE slug IS NULL OR slug = '';

-- Handle duplicate slugs by appending the ID
UPDATE shows s1
SET slug = s1.slug || '-' || s1.id
FROM (
    SELECT slug FROM shows GROUP BY slug HAVING COUNT(*) > 1
) dups
WHERE s1.slug = dups.slug
  AND s1.id > (SELECT MIN(id) FROM shows WHERE slug = s1.slug);
