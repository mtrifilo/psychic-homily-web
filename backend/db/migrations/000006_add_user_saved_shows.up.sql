-- Create user_saved_shows junction table
-- Allows users to save shows to their personal list
CREATE TABLE user_saved_shows (
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    show_id INT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    saved_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, show_id)
);

-- Indexes for efficient queries
CREATE INDEX idx_user_saved_shows_user_id ON user_saved_shows(user_id);
CREATE INDEX idx_user_saved_shows_show_id ON user_saved_shows(show_id);
CREATE INDEX idx_user_saved_shows_saved_at ON user_saved_shows(user_id, saved_at DESC);

-- Comments for documentation
COMMENT ON TABLE user_saved_shows IS 'Many-to-many relationship tracking which shows users have saved to their list';
COMMENT ON COLUMN user_saved_shows.saved_at IS 'Timestamp when the user saved the show (for ordering)';
