-- Create generic user_bookmarks table replacing user_saved_shows and user_favorite_venues
-- Supports all entity types and action types for future extensibility

CREATE TABLE user_bookmarks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type VARCHAR(50) NOT NULL,
    entity_id BIGINT NOT NULL,
    action VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reminder_sent_at TIMESTAMPTZ,
    UNIQUE(user_id, entity_type, entity_id, action)
);

-- Indexes for efficient queries
CREATE INDEX idx_user_bookmarks_user_entity ON user_bookmarks(user_id, entity_type);
CREATE INDEX idx_user_bookmarks_user_entity_action ON user_bookmarks(user_id, entity_type, action);
CREATE INDEX idx_user_bookmarks_entity ON user_bookmarks(entity_type, entity_id);
CREATE INDEX idx_user_bookmarks_created_at ON user_bookmarks(user_id, created_at DESC);
CREATE INDEX idx_user_bookmarks_reminder ON user_bookmarks(entity_type, action, reminder_sent_at)
    WHERE reminder_sent_at IS NULL;

COMMENT ON TABLE user_bookmarks IS 'Generic user-entity relationship table supporting saves, follows, bookmarks, going, interested actions across all entity types';
COMMENT ON COLUMN user_bookmarks.entity_type IS 'Entity type: show, venue, artist, release, label, festival';
COMMENT ON COLUMN user_bookmarks.action IS 'Action type: save, follow, bookmark, going, interested';
COMMENT ON COLUMN user_bookmarks.reminder_sent_at IS 'Timestamp when a reminder was sent (for deduplication)';

-- Migrate data from user_saved_shows
INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at, reminder_sent_at)
SELECT user_id, 'show', show_id, 'save', saved_at, reminder_sent_at
FROM user_saved_shows;

-- Migrate data from user_favorite_venues
INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
SELECT user_id, 'venue', venue_id, 'follow', favorited_at
FROM user_favorite_venues;

-- Drop old tables
DROP TABLE IF EXISTS user_saved_shows;
DROP TABLE IF EXISTS user_favorite_venues;
