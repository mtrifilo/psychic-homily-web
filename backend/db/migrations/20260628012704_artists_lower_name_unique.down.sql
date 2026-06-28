-- PSY-1256 down: restore the original case-sensitive name uniqueness + non-unique
-- index, then drop the case-insensitive functional index. (Safe: while the
-- functional index was active no case-variant dups could be created, so re-adding
-- the case-sensitive artists_name_key cannot fail on a duplicate.)
DROP INDEX IF EXISTS artists_lower_name_uniq;
ALTER TABLE artists ADD CONSTRAINT artists_name_key UNIQUE (name);
CREATE INDEX IF NOT EXISTS idx_artists_name ON artists (name);
