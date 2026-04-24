DROP INDEX IF EXISTS idx_comment_edits_editor;
ALTER TABLE comment_edits DROP COLUMN IF EXISTS editor_user_id;
