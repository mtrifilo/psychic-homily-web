-- PSY-1354: index artists.musicbrainz_artist_id for radio play matching.
-- Radio plays (KEXP etc.) carry musicbrainz_artist_id; the matcher looks up
-- artists BY MBID before falling back to name. Partial index — only non-empty MBIDs.
CREATE INDEX IF NOT EXISTS idx_artists_musicbrainz_artist_id
  ON artists (musicbrainz_artist_id)
  WHERE musicbrainz_artist_id IS NOT NULL AND TRIM(musicbrainz_artist_id) <> '';
