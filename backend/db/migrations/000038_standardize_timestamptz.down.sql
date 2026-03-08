-- Revert TIMESTAMPTZ columns back to TIMESTAMP (without timezone)

-- Table: artists
ALTER TABLE artists ALTER COLUMN created_at TYPE TIMESTAMP;
ALTER TABLE artists ALTER COLUMN updated_at TYPE TIMESTAMP;

-- Table: venues
ALTER TABLE venues ALTER COLUMN created_at TYPE TIMESTAMP;
ALTER TABLE venues ALTER COLUMN updated_at TYPE TIMESTAMP;

-- Table: shows
ALTER TABLE shows ALTER COLUMN created_at TYPE TIMESTAMP;
ALTER TABLE shows ALTER COLUMN updated_at TYPE TIMESTAMP;

-- Table: users
ALTER TABLE users ALTER COLUMN created_at TYPE TIMESTAMP;
ALTER TABLE users ALTER COLUMN updated_at TYPE TIMESTAMP;

-- Table: oauth_accounts
ALTER TABLE oauth_accounts ALTER COLUMN expires_at TYPE TIMESTAMP;
ALTER TABLE oauth_accounts ALTER COLUMN created_at TYPE TIMESTAMP;
ALTER TABLE oauth_accounts ALTER COLUMN updated_at TYPE TIMESTAMP;

-- Table: user_preferences
ALTER TABLE user_preferences ALTER COLUMN created_at TYPE TIMESTAMP;
ALTER TABLE user_preferences ALTER COLUMN updated_at TYPE TIMESTAMP;

-- Table: pending_venue_edits
ALTER TABLE pending_venue_edits ALTER COLUMN reviewed_at TYPE TIMESTAMP;
ALTER TABLE pending_venue_edits ALTER COLUMN created_at TYPE TIMESTAMP;
ALTER TABLE pending_venue_edits ALTER COLUMN updated_at TYPE TIMESTAMP;

-- Table: show_reports
ALTER TABLE show_reports ALTER COLUMN reviewed_at TYPE TIMESTAMP;
ALTER TABLE show_reports ALTER COLUMN created_at TYPE TIMESTAMP;
ALTER TABLE show_reports ALTER COLUMN updated_at TYPE TIMESTAMP;

-- Table: audit_logs
ALTER TABLE audit_logs ALTER COLUMN created_at TYPE TIMESTAMP;
