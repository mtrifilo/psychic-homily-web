-- An account row is live (is_active, deleted_at NULL), deactivated (not
-- is_active, deleted_at NULL) or soft-deleted (not is_active, deleted_at set).
-- This constraint refuses the fourth combination, is_active with deleted_at
-- set, so a filter on is_active alone selects exactly the rows
-- User.CanSignIn accepts.
--
-- ADD CONSTRAINT scans every existing row under an ACCESS EXCLUSIVE lock and
-- fails if any row holds the refused combination. lock_timeout bounds the wait
-- for that lock, so a long-running transaction on users fails this migration
-- instead of queueing every later users read and write behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE users
    ADD CONSTRAINT users_live_account_check
    CHECK (NOT (is_active AND deleted_at IS NOT NULL));
