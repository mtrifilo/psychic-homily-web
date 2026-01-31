-- Backfill venue slugs for existing venues
-- Format: name-city-state (lowercase, spaces to hyphens, remove special chars)

UPDATE venues
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(
            CONCAT(name, '-', city, '-', state),
            '[^a-zA-Z0-9\s-]', '', 'g'  -- Remove special characters except spaces and hyphens
        ),
        '\s+', '-', 'g'  -- Replace spaces with hyphens
    )
)
WHERE slug IS NULL;

-- Handle any duplicate venue slugs by appending the ID
UPDATE venues v1
SET slug = v1.slug || '-' || v1.id
WHERE EXISTS (
    SELECT 1 FROM venues v2
    WHERE v2.slug = v1.slug AND v2.id < v1.id
);

-- Backfill artist slugs for existing artists
-- Format: name (lowercase, spaces to hyphens, remove special chars)

UPDATE artists
SET slug = LOWER(
    REGEXP_REPLACE(
        REGEXP_REPLACE(
            name,
            '[^a-zA-Z0-9\s-]', '', 'g'  -- Remove special characters except spaces and hyphens
        ),
        '\s+', '-', 'g'  -- Replace spaces with hyphens
    )
)
WHERE slug IS NULL;

-- Handle any duplicate artist slugs by appending the ID
UPDATE artists a1
SET slug = a1.slug || '-' || a1.id
WHERE EXISTS (
    SELECT 1 FROM artists a2
    WHERE a2.slug = a1.slug AND a2.id < a1.id
);
