-- Reverse PSY-2030's case-insensitive uniqueness on users.email.
--
-- Non-destructive: dropping an index neither reads nor rewrites rows, and the
-- byte-exact UNIQUE constraint on users.email is untouched, so the column
-- keeps the uniqueness it had before this migration. Dropping the index also
-- drops its comment.
--
-- Reverting the schema without reverting the application leaves the
-- lower(email) lookups correct but no longer enforced: a concurrent signup
-- could then admit a case-variant duplicate that those lookups would resolve
-- to whichever row sorts first.

DROP INDEX IF EXISTS users_lower_email_key;
