ALTER TABLE tags ADD COLUMN reviewed_at TIMESTAMPTZ NULL;
CREATE INDEX idx_tags_reviewed_at ON tags(reviewed_at);
