-- radio_tracks: a derived read model with one row per track as broadcast,
-- rebuilt from radio_plays by the radio_tracks_rebuild loop
-- (catalog.RebuildRadioTracks). radio_plays stays the source of truth: nothing
-- here is written by hand, and a rebuild reproduces every derived column from
-- the plays alone. Every radio_plays row counts: there is no aired-episode
-- gate, no radio_play_exclusions filter and no pseudo-artist filter, so
-- readers that rank or display tracks apply those themselves.
--
-- Track key. A play's key is (radio_track_artist_key(artist_name),
-- radio_track_title_key(track_title)). The title key strips featured-artist
-- credits, trailing bracket groups and " - remix/edit/version/live/mono/stereo"
-- tails before normalizing; a title that strips to nothing keeps its unstripped
-- normalized form. A play whose artist key or title key is empty (or NULL) is
-- UNKEYED: every unkeyed play counts toward the single marker row whose
-- artist_key and title_key are both ''.
--
-- MusicBrainz join. Two keys whose plays carry the same MusicBrainz recording
-- id are one track (transitively). Only ids shaped like a UUID take part. A recording id never splits a key: a key
-- whose plays carry several ids stays one track with mb_recording_id NULL and
-- mb_recording_id_conflict true. radio_track_keys maps every key to its track,
-- so a joined track has one radio_tracks row and several radio_track_keys rows.

-- radio_track_artist_key is the artist half of the track key.
CREATE OR REPLACE FUNCTION public.radio_track_artist_key(text)
  RETURNS text
  AS $$ SELECT public.radio_normalize_name($1) $$
  LANGUAGE SQL IMMUTABLE PARALLEL SAFE STRICT;

-- radio_track_title_key is the title half of the track key. Stripping runs on
-- immutable_unaccent(lower(title)) before radio_normalize_name so the boundary
-- trim cannot eat a closing bracket first. Steps, in order:
--   1. an unbracketed credit: " feat. x", " ft. x", " featuring x" to the end
--   2. trailing bracket groups, repeated: " (live) [2004]"
--   3. a dash tail carrying a keyword: " - live at x", " - radio edit"
--   4. step 2 again, for "x (demo) - live", whose bracket group is exposed
--      only once step 3 removes the dash tail
-- Helpers are schema-qualified so the function also works where an index
-- expression is evaluated under a restricted search_path.
CREATE OR REPLACE FUNCTION public.radio_track_title_key(text)
  RETURNS text
  AS $$
    SELECT coalesce(
      nullif(public.radio_normalize_name(
        regexp_replace(
          regexp_replace(
            regexp_replace(
              regexp_replace(
                public.immutable_unaccent(lower($1)),
                '\s+(featuring|feat\.?|ft\.?)\s.*$', ''),
              '(\s*[\(\[\{][^\(\)\[\]\{\}]*[\)\]\}])+\s*$', ''),
            '\s+[-\u2013\u2014]\s+.*\m(remix|edit|version|live|mono|stereo)\M.*$', ''),
          '(\s*[\(\[\{][^\(\)\[\]\{\}]*[\)\]\}])+\s*$', '')
      ), ''),
      public.radio_normalize_name($1)
    )
  $$
  LANGUAGE SQL IMMUTABLE PARALLEL SAFE STRICT;

CREATE TABLE radio_tracks (
    id BIGSERIAL PRIMARY KEY,
    artist_key TEXT NOT NULL,
    -- The normalized, suffix-stripped title (radio_track_title_key output).
    title_key TEXT NOT NULL,
    unkeyed BOOLEAN GENERATED ALWAYS AS (artist_key = '') STORED,
    -- The most common raw spelling among the track's plays; ties go to the
    -- lexically smallest. Both are '' on the unkeyed marker row.
    display_artist TEXT NOT NULL,
    display_title TEXT NOT NULL,
    play_count INTEGER NOT NULL,
    -- Distinct radio_stations rows, so each WFMU stream counts on its own.
    station_row_count INTEGER NOT NULL,
    -- Distinct broadcasters: a station's network when it has one, else the
    -- station, so the WFMU streams count once.
    station_family_count INTEGER NOT NULL,
    show_count INTEGER NOT NULL,
    -- radio_episodes.air_date of the earliest and latest episode carrying a
    -- play: the station-local broadcast date, present on every play. An
    -- episode published before it airs counts, so last_aired_on can be a
    -- future date.
    first_aired_on DATE NOT NULL,
    last_aired_on DATE NOT NULL,
    -- Set when every MusicBrainz-tagged play of the track carries the same
    -- recording id.
    mb_recording_id TEXT,
    mb_recording_id_conflict BOOLEAN NOT NULL,
    -- Plays whose album_title or label_name has a non-space character.
    album_play_count INTEGER NOT NULL,
    label_play_count INTEGER NOT NULL,
    -- The rebuild that last changed any derived column of this row. A rebuild
    -- leaves unchanged rows untouched, so the loop's run state, not this
    -- column, says when the table was last rebuilt.
    rebuilt_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT radio_tracks_key_unique UNIQUE (artist_key, title_key),
    CONSTRAINT radio_tracks_key_shape_check CHECK ((artist_key = '') = (title_key = '')),
    CONSTRAINT radio_tracks_mb_check CHECK (NOT (mb_recording_id IS NOT NULL AND mb_recording_id_conflict)),
    CONSTRAINT radio_tracks_counts_check CHECK (
        play_count > 0
        AND station_row_count BETWEEN 1 AND play_count
        AND station_family_count BETWEEN 1 AND station_row_count
        AND show_count BETWEEN 1 AND play_count
        AND album_play_count BETWEEN 0 AND play_count
        AND label_play_count BETWEEN 0 AND play_count
        AND first_aired_on <= last_aired_on
    )
);

CREATE INDEX idx_radio_tracks_play_count ON radio_tracks (play_count DESC);
CREATE INDEX idx_radio_tracks_station_row_count ON radio_tracks (station_row_count DESC);
CREATE INDEX idx_radio_tracks_station_family_count ON radio_tracks (station_family_count DESC);
CREATE INDEX idx_radio_tracks_last_aired_on ON radio_tracks (last_aired_on DESC);

-- radio_track_keys maps every track key seen in radio_plays to its track. A
-- key that a MusicBrainz recording id joined to another key points at that
-- key's track.
CREATE TABLE radio_track_keys (
    artist_key TEXT NOT NULL,
    title_key TEXT NOT NULL,
    track_id BIGINT NOT NULL REFERENCES radio_tracks(id) ON DELETE CASCADE,
    PRIMARY KEY (artist_key, title_key)
);

CREATE INDEX idx_radio_track_keys_track_id ON radio_track_keys (track_id);
