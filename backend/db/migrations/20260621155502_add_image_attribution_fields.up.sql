-- PSY-1175: per-image attribution metadata for cover art + entity photos.
-- Each provider (Spotify/Discogs/Cover Art Archive) requires attribution + a
-- linkback when we display its image. We store only a REFERENCE to the externally
-- hosted image (existing *_url columns) plus, here, the provider id and the deep
-- linkback so the UI can render "Cover via Spotify ↗" / "Data provided by Discogs".
-- source ∈ spotify | discogs | cover_art_archive | user | commons | public_domain.
--
-- ADDITIVE: nullable columns, no DEFAULT => Postgres adds them without a table
-- rewrite. Existing rows keep NULL source (legacy/unknown => rendered without
-- attribution). Multi-statement file => golang-migrate wraps it in one transaction.
ALTER TABLE releases
    ADD COLUMN cover_art_source     VARCHAR(32),
    ADD COLUMN cover_art_source_url TEXT;

ALTER TABLE artists
    ADD COLUMN image_source     VARCHAR(32),
    ADD COLUMN image_source_url TEXT;

ALTER TABLE labels
    ADD COLUMN image_source     VARCHAR(32),
    ADD COLUMN image_source_url TEXT;
