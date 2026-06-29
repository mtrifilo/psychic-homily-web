-- PSY-1281: releases.musicbrainz_release_group_id — the release subsystem's FIRST
-- exact dedup key, the keystone for the discography importer (PSY-1252 decision:
-- GO on MusicBrainz browse-by-MBID). Mirrors the artist MBID keystone (PSY-1249).
--
-- Why the release-GROUP MBID (not the release MBID): the importer keys on the album
-- ABSTRACTION (one row per album, not per pressing/edition), which is also the grain
-- the Cover Art Archive is keyed on (PSY-1216). VARCHAR(36) = a canonical MBID UUID,
-- matching artists.musicbrainz_artist_id.
--
-- PARTIAL-unique index: two releases may not share a release-group MBID (the
-- importer's dedup guard, and it makes a concurrent double-import a clean conflict
-- rather than a duplicate), while the entire legacy backlog — every existing release
-- has a NULL MBID — stays unconstrained, since NULLs are excluded from the index.
--
-- Multi-statement file => golang-migrate wraps it in a transaction => no CREATE
-- INDEX CONCURRENTLY (illegal in a txn, and unnecessary here: the partial index
-- covers zero rows at creation, every existing release's MBID being NULL).
--
-- Backfilling existing releases' RG-MBID is out of scope (they have no MBID); the
-- importer fills it forward-only on match (fill-when-empty), and a future pass can
-- reconcile.
ALTER TABLE releases ADD COLUMN musicbrainz_release_group_id VARCHAR(36);

CREATE UNIQUE INDEX uq_releases_musicbrainz_release_group_id
    ON releases (musicbrainz_release_group_id)
    WHERE musicbrainz_release_group_id IS NOT NULL;
