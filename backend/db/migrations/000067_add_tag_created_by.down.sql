DROP INDEX IF EXISTS idx_tags_created_by;
ALTER TABLE tags DROP COLUMN IF EXISTS created_by_user_id;
