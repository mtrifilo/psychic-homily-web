-- Reverse PSY-2030's case-insensitive uniqueness on users.email.
--
-- No data is lost or rewritten, and the byte-exact UNIQUE constraint on
-- users.email is untouched throughout, so the column keeps the uniqueness it
-- had before this migration. Not free, though: rebuilding idx_users_email is a
-- full scan of users under the same SHARE lock the up migration takes.
--
-- idx_users_email is restored to the shape 000001_create_initial_schema left
-- it in, so a down/up cycle is a round trip rather than a one-way cleanup.
--
-- Order matters: recreate the weaker index before dropping the stronger one,
-- so the raw column is never unindexed.

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

DROP INDEX IF EXISTS users_lower_email_uniq;
