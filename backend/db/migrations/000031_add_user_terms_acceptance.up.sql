-- Track Terms/Privacy acceptance metadata for account creation
ALTER TABLE users
  ADD COLUMN terms_accepted_at TIMESTAMPTZ,
  ADD COLUMN terms_version VARCHAR(64),
  ADD COLUMN privacy_version VARCHAR(64);
