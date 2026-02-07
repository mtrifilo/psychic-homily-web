-- Backfill slugs for venues with NULL slugs
-- Uses the same name-city-state pattern as the Go code: GenerateVenueSlug(name, city, state)

UPDATE venues
SET slug = LOWER(
    TRIM(BOTH '-' FROM
        REGEXP_REPLACE(
            REGEXP_REPLACE(
                REGEXP_REPLACE(
                    CONCAT(name, '-', city, '-', state),
                    '\s+', '-', 'g'
                ),
                '[^a-z0-9-]', '', 'g'
            ),
            '-+', '-', 'g'
        )
    )
)
WHERE slug IS NULL;

-- Deduplicate venue slugs by appending ID
UPDATE venues v1
SET slug = v1.slug || '-' || v1.id
WHERE EXISTS (
    SELECT 1 FROM venues v2
    WHERE v2.slug = v1.slug AND v2.id < v1.id
);
