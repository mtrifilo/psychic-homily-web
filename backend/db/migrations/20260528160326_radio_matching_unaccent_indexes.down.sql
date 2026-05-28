-- PSY-886: Drop the radio-matching unaccent expression indexes.
--
-- We deliberately leave the `unaccent` extension installed. Other tables /
-- features may grow to depend on it, and dropping it on rollback would
-- silently break any future lookups using `unaccent(...)`.

DROP INDEX IF EXISTS idx_labels_name_unaccent_lower;
DROP INDEX IF EXISTS idx_releases_title_unaccent_lower;
DROP INDEX IF EXISTS idx_artist_aliases_alias_unaccent_lower;
DROP INDEX IF EXISTS idx_artists_name_unaccent_lower;

-- Drop the immutable wrapper after the indexes that depend on it.
DROP FUNCTION IF EXISTS immutable_unaccent(text);
