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
