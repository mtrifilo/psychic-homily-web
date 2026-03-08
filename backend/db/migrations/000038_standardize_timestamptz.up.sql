-- Standardize all TIMESTAMP columns to TIMESTAMPTZ
-- PostgreSQL treats this as a metadata-only change: existing values are
-- interpreted in the server's timezone and stored as UTC internally.
-- No data is rewritten; this is fast and non-destructive.

-- Table: artists (from 000001)
ALTER TABLE artists ALTER COLUMN created_at TYPE TIMESTAMPTZ;
ALTER TABLE artists ALTER COLUMN updated_at TYPE TIMESTAMPTZ;

-- Table: venues (from 000001)
ALTER TABLE venues ALTER COLUMN created_at TYPE TIMESTAMPTZ;
ALTER TABLE venues ALTER COLUMN updated_at TYPE TIMESTAMPTZ;

-- Table: shows (from 000001; event_date already converted in 000028, scraped_at created as TIMESTAMPTZ in 000010)
ALTER TABLE shows ALTER COLUMN created_at TYPE TIMESTAMPTZ;
ALTER TABLE shows ALTER COLUMN updated_at TYPE TIMESTAMPTZ;

-- Table: users (from 000001; deleted_at, locked_until, terms_accepted_at already TIMESTAMPTZ)
ALTER TABLE users ALTER COLUMN created_at TYPE TIMESTAMPTZ;
ALTER TABLE users ALTER COLUMN updated_at TYPE TIMESTAMPTZ;

-- Table: oauth_accounts (from 000001)
ALTER TABLE oauth_accounts ALTER COLUMN expires_at TYPE TIMESTAMPTZ;
ALTER TABLE oauth_accounts ALTER COLUMN created_at TYPE TIMESTAMPTZ;
ALTER TABLE oauth_accounts ALTER COLUMN updated_at TYPE TIMESTAMPTZ;

-- Table: user_preferences (from 000001)
ALTER TABLE user_preferences ALTER COLUMN created_at TYPE TIMESTAMPTZ;
ALTER TABLE user_preferences ALTER COLUMN updated_at TYPE TIMESTAMPTZ;

-- Table: pending_venue_edits (from 000008)
ALTER TABLE pending_venue_edits ALTER COLUMN reviewed_at TYPE TIMESTAMPTZ;
ALTER TABLE pending_venue_edits ALTER COLUMN created_at TYPE TIMESTAMPTZ;
ALTER TABLE pending_venue_edits ALTER COLUMN updated_at TYPE TIMESTAMPTZ;

-- Table: show_reports (from 000018)
ALTER TABLE show_reports ALTER COLUMN reviewed_at TYPE TIMESTAMPTZ;
ALTER TABLE show_reports ALTER COLUMN created_at TYPE TIMESTAMPTZ;
ALTER TABLE show_reports ALTER COLUMN updated_at TYPE TIMESTAMPTZ;

-- Table: audit_logs (from 000022)
ALTER TABLE audit_logs ALTER COLUMN created_at TYPE TIMESTAMPTZ;

-- Tables already using TIMESTAMPTZ (no changes needed):
--   webauthn_credentials (000011) — all columns TIMESTAMP WITH TIME ZONE
--   webauthn_challenges (000011)  — all columns TIMESTAMP WITH TIME ZONE
--   api_tokens (000021)           — all columns TIMESTAMP WITH TIME ZONE
--   artist_reports (000030)       — all columns TIMESTAMPTZ
--   calendar_tokens (000033)      — created_at TIMESTAMPTZ
--   releases (000035)             — all columns TIMESTAMPTZ
--   release_external_links (000035) — created_at TIMESTAMPTZ
--   labels (000036)               — all columns TIMESTAMPTZ
--   user_bookmarks (000037)       — all columns TIMESTAMPTZ
-- Tables dropped by 000037:
--   user_saved_shows              — dropped (data migrated to user_bookmarks)
--   user_favorite_venues          — dropped (data migrated to user_bookmarks)
