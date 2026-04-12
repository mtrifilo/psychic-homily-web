ALTER TABLE tags ADD COLUMN created_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_tags_created_by ON tags(created_by_user_id) WHERE created_by_user_id IS NOT NULL;
