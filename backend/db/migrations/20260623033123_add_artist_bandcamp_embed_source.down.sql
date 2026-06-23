-- PSY-1188: reverse the artist Bandcamp embed provenance column. DROP COLUMN
-- IF EXISTS lands the up->down->up CI round-trip back on the pre-PSY-1188 schema
-- exactly.
ALTER TABLE artists
    DROP COLUMN IF EXISTS bandcamp_embed_source;
