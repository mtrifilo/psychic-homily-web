ALTER TABLE artists
    DROP COLUMN IF EXISTS image_license,
    DROP COLUMN IF EXISTS image_author;
