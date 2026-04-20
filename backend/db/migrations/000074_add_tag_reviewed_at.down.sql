DROP INDEX IF EXISTS idx_tags_reviewed_at;
ALTER TABLE tags DROP COLUMN IF EXISTS reviewed_at;
