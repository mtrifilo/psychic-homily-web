-- Granular privacy settings (JSONB) for per-field profile visibility control
-- Each field: "visible", "count_only", or "hidden"
-- Only applies when profile_visibility = 'public' (master switch)
ALTER TABLE users ADD COLUMN privacy_settings JSONB NOT NULL DEFAULT '{"contributions":"visible","saved_shows":"hidden","attendance":"hidden","following":"count_only","collections":"visible","last_active":"visible","profile_sections":"visible"}';

-- User tier for contributor rank progression
-- Values: new_user, contributor, trusted_contributor, local_ambassador
ALTER TABLE users ADD COLUMN user_tier VARCHAR(30) NOT NULL DEFAULT 'new_user';
CREATE INDEX idx_users_user_tier ON users(user_tier);

-- Custom profile sections (max 3 per user, enforced in service layer)
CREATE TABLE user_profile_sections (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    position INT NOT NULL,
    is_visible BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, position)
);

CREATE INDEX idx_user_profile_sections_user_id ON user_profile_sections(user_id);
