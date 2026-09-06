-- Reverse PSY-2030's case-insensitive uniqueness on users.email.
--
-- Non-destructive: neither statement reads or rewrites rows, and the
-- byte-exact UNIQUE constraint on users.email is untouched throughout, so the
-- column keeps the uniqueness it had before this migration.
--
-- idx_users_email is restored to the shape 000001_create_initial_schema left
-- it in, so a down/up cycle is a round trip rather than a one-way cleanup.

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

DROP INDEX IF EXISTS users_lower_email_key;
