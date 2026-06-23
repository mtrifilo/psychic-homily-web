-- PSY-1188: provenance for artists.bandcamp_embed_url.
--
-- The artist Bandcamp embed (an album/track URL rendered in an iframe) can be
-- set three ways: a human/admin edit, the AI/community entity-request fulfiller,
-- or — new in PSY-1188 — auto-derived from one of the artist's catalogued
-- release Bandcamp links. A later keep-fresh hook (PSY-1189) needs to tell those
-- apart so it can safely refresh/clean up the auto-derived ones WITHOUT touching
-- a value a person curated. This column records which source set the embed.
--
-- Values (kept in sync with catalog.BandcampEmbedSource* constants):
--   release_derived  — backfilled / auto-derived from a release Bandcamp link.
--   manual           — set by a human/admin/AI write path.
-- NULL = legacy/unknown (set before this column existed). The keep-fresh hook
-- treats NULL conservatively (does not auto-refresh) until it is stamped.
--
-- ADDITIVE: a single nullable VARCHAR with no DEFAULT => Postgres adds it
-- without a table rewrite. Single-statement file => golang-migrate wraps it in
-- one transaction. Internal column only — NOT exposed in any API response.
ALTER TABLE artists
    ADD COLUMN bandcamp_embed_source VARCHAR(32);
