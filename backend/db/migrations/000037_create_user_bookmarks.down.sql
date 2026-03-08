-- Recreate user_saved_shows table
CREATE TABLE user_saved_shows (
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    show_id INT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    saved_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reminder_sent_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, show_id)
);

CREATE INDEX idx_user_saved_shows_user_id ON user_saved_shows(user_id);
CREATE INDEX idx_user_saved_shows_show_id ON user_saved_shows(show_id);
CREATE INDEX idx_user_saved_shows_saved_at ON user_saved_shows(user_id, saved_at DESC);

-- Migrate data back from user_bookmarks
INSERT INTO user_saved_shows (user_id, show_id, saved_at, reminder_sent_at)
SELECT user_id, entity_id, created_at, reminder_sent_at
FROM user_bookmarks
WHERE entity_type = 'show' AND action = 'save';

-- Recreate user_favorite_venues table
CREATE TABLE user_favorite_venues (
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    venue_id INT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    favorited_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, venue_id)
);

CREATE INDEX idx_user_favorite_venues_user_id ON user_favorite_venues(user_id);
CREATE INDEX idx_user_favorite_venues_venue_id ON user_favorite_venues(venue_id);

-- Migrate data back from user_bookmarks
INSERT INTO user_favorite_venues (user_id, venue_id, favorited_at)
SELECT user_id, entity_id, created_at
FROM user_bookmarks
WHERE entity_type = 'venue' AND action = 'follow';

-- Drop user_bookmarks table
DROP TABLE IF EXISTS user_bookmarks;
