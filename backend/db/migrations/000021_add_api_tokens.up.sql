-- Create api_tokens table for long-lived API authentication
-- Used by the local scraper app and other admin tools
CREATE TABLE api_tokens (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA-256 hash of the token
    description VARCHAR(255),                 -- User-provided description (e.g., "Mike's laptop scraper")
    scope VARCHAR(50) NOT NULL DEFAULT 'admin',  -- Token scope (admin, read-only, etc.)
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_used_at TIMESTAMP WITH TIME ZONE,
    revoked_at TIMESTAMP WITH TIME ZONE
);

-- Index for token lookup (used on every API request with token auth)
CREATE INDEX idx_api_tokens_token_hash ON api_tokens(token_hash);

-- Index for listing user's tokens
CREATE INDEX idx_api_tokens_user_id ON api_tokens(user_id);

-- Index for cleanup of expired/revoked tokens
CREATE INDEX idx_api_tokens_expires_revoked ON api_tokens(expires_at, revoked_at);
