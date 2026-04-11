-- Add moderation tracking fields to comments for admin hide/restore
ALTER TABLE comments ADD COLUMN hidden_reason VARCHAR(255);
ALTER TABLE comments ADD COLUMN hidden_by_user_id INTEGER REFERENCES users(id);
ALTER TABLE comments ADD COLUMN hidden_at TIMESTAMPTZ;

-- Index for admin pending review queue
CREATE INDEX idx_comments_visibility ON comments(visibility) WHERE visibility = 'pending_review';
