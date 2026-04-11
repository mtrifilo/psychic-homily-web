DROP INDEX IF EXISTS idx_comments_visibility;
ALTER TABLE comments DROP COLUMN IF EXISTS hidden_at;
ALTER TABLE comments DROP COLUMN IF EXISTS hidden_by_user_id;
ALTER TABLE comments DROP COLUMN IF EXISTS hidden_reason;
