-- PSY-1232: per-image CC attribution detail for artist photos sourced from
-- Wikimedia Commons. CC-BY / CC-BY-SA require crediting the author + the
-- specific license, which image_source / image_source_url alone do not capture
-- (those derive a provider label; CC needs the photographer + license string).
--
-- Artist-only: release covers (CAA / Discogs) derive their attribution from
-- image_source, so releases / labels do not need these columns.
ALTER TABLE artists
    ADD COLUMN image_license VARCHAR(64),
    ADD COLUMN image_author  TEXT;
