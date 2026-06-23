-- PSY-1175: reverse the per-image attribution columns. DROP COLUMN IF EXISTS lands
-- the up->down->up CI round-trip back on the pre-PSY-1175 schema exactly.
ALTER TABLE releases
    DROP COLUMN IF EXISTS cover_art_source,
    DROP COLUMN IF EXISTS cover_art_source_url;

ALTER TABLE artists
    DROP COLUMN IF EXISTS image_source,
    DROP COLUMN IF EXISTS image_source_url;

ALTER TABLE labels
    DROP COLUMN IF EXISTS image_source,
    DROP COLUMN IF EXISTS image_source_url;
