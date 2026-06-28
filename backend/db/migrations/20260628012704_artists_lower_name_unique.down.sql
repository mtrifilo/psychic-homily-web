-- PSY-1256 down: restore the non-unique name index, drop the unique functional one.
CREATE INDEX IF NOT EXISTS idx_artists_name ON artists (name);
DROP INDEX IF EXISTS artists_lower_name_uniq;
