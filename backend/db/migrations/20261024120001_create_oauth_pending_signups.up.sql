-- oauth_pending_signups holds a provider identity between the OAuth callback
-- that found it attached to no account and the "Create your account" step
-- that turns it into one. A row is never a user: it carries what the
-- provider asserted and where the reader was headed, and nothing else.
--
-- token_hash is the SHA-256 (hex) of the random id the pending cookie
-- carries, so a read of this table yields nothing a browser could present. A
-- row is consumed by DELETE ... RETURNING, which hands it to at most one
-- request however many carry its cookie, and is refused after expires_at
-- whether or not it was consumed. Inserting a row deletes every expired one.
--
-- New, empty table with no foreign keys: nothing else is locked, and the
-- index locks only this table. lock_timeout is set all the same, so a manual
-- run outside the bounded boot runner cannot wait without limit.
SET LOCAL lock_timeout = '5s';

CREATE TABLE oauth_pending_signups (
    token_hash CHAR(64) PRIMARY KEY,
    provider VARCHAR(50) NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    name TEXT NOT NULL DEFAULT '',
    first_name TEXT NOT NULL DEFAULT '',
    last_name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    return_to TEXT NOT NULL DEFAULT '/',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The sweep on insert: DELETE ... WHERE expires_at <= now.
CREATE INDEX idx_oauth_pending_signups_expires_at
    ON oauth_pending_signups (expires_at);
