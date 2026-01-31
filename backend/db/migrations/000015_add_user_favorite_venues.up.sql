-- Add user_favorite_venues table for tracking users' favorite venues
CREATE TABLE user_favorite_venues (
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    venue_id INT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    favorited_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, venue_id)
);

-- Index for efficient user lookups (get all favorites for a user)
CREATE INDEX idx_user_favorite_venues_user_id ON user_favorite_venues(user_id);

-- Index for efficient venue lookups (get all users who favorited a venue)
CREATE INDEX idx_user_favorite_venues_venue_id ON user_favorite_venues(venue_id);
