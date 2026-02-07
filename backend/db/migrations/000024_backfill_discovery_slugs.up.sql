-- Backfill slugs for artists created by discovery import or inline show creation
-- Reuses patterns from migrations 000017 (artist slugs) and 000019 (show slugs)

-- Backfill artist slugs
UPDATE artists
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(
            name,
            '[^a-zA-Z0-9\s-]', '', 'g'
        ),
        '\s+', '-', 'g'
    )
)
WHERE slug IS NULL OR slug = '';

-- Deduplicate artist slugs by appending ID
UPDATE artists a1
SET slug = a1.slug || '-' || a1.id
WHERE EXISTS (
    SELECT 1 FROM artists a2
    WHERE a2.slug = a1.slug AND a2.id < a1.id
);

-- Backfill show slugs for shows WITH titles
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

-- Backfill show slugs for shows WITHOUT titles (headliner-at-venue-date)
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

-- Fallback for any remaining shows without artists/venues
UPDATE shows
SET slug = CONCAT('show-', id, '-', TO_CHAR(event_date, 'YYYY-MM-DD'))
WHERE slug IS NULL OR slug = '';

-- Deduplicate show slugs by appending ID
UPDATE shows s1
SET slug = s1.slug || '-' || s1.id
FROM (
    SELECT slug FROM shows GROUP BY slug HAVING COUNT(*) > 1
) dups
WHERE s1.slug = dups.slug
  AND s1.id > (SELECT MIN(id) FROM shows WHERE slug = s1.slug);
