-- PSY-1570 rollback. Drops the indexes and restores the function to its
-- pre-migration (unqualified) form. Order matters: the indexes depend on the
-- function resolving, so they go first.
--
-- Restoring the unqualified call re-introduces the "cannot be indexed"
-- limitation, which is correct for a rollback — it is the state this migration
-- was applied against. No data is lost either way.
DROP INDEX IF EXISTS idx_artists_norm_name;
DROP INDEX IF EXISTS idx_releases_norm_title;
DROP INDEX IF EXISTS idx_radio_plays_norm_artist_name;

CREATE OR REPLACE FUNCTION public.radio_normalize_name(text)
    RETURNS text
    LANGUAGE sql
    IMMUTABLE PARALLEL SAFE STRICT
AS $function$
    SELECT regexp_replace(
      regexp_replace(
        regexp_replace(immutable_unaccent(LOWER($1)), '^[^a-z0-9]+', ''),
        '[^a-z0-9]+$',
        ''
      ),
      '\s+',
      ' ',
      'g'
    )
  $function$;
