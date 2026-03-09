-- Add profile_visibility to users for contributor profile privacy controls
ALTER TABLE users ADD COLUMN profile_visibility VARCHAR(20) NOT NULL DEFAULT 'public';

CREATE INDEX idx_users_profile_visibility ON users(profile_visibility);
