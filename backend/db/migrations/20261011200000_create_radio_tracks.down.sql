-- Reverse 20261011200000_create_radio_tracks.up.sql. Both tables are derived
-- from radio_plays, so dropping them loses nothing the next rebuild cannot
-- reproduce.
DROP TABLE IF EXISTS radio_track_keys;
DROP TABLE IF EXISTS radio_tracks;
DROP FUNCTION IF EXISTS public.radio_track_title_key(text);
DROP FUNCTION IF EXISTS public.radio_track_artist_key(text);
