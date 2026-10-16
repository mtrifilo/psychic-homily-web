-- Restores the radio_track_title_key body of 20261011200000_create_radio_tracks
-- (any trailing bracket group stripped, bare "ft" always a credit) and drops
-- the marker-group helpers, which that body does not call.
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

DROP FUNCTION IF EXISTS public.radio_track_strip_marker_groups(text);
DROP FUNCTION IF EXISTS public.radio_track_title_marker_words();
