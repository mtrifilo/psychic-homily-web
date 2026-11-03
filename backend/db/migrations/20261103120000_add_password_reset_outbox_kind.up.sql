-- auth_email_outbox gains the emailed password reset link's kind. Like the
-- other kinds, the row names only the account; the poller mints the link at
-- send time.
--
-- Replacing a CHECK takes an ACCESS EXCLUSIVE lock on the table and scans
-- it; the table holds at most a week of rows. lock_timeout bounds the wait,
-- so a long-running transaction fails this migration instead of queueing the
-- table's later reads and writes behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE auth_email_outbox
    DROP CONSTRAINT auth_email_outbox_kind_check,
    ADD CONSTRAINT auth_email_outbox_kind_check
        CHECK (kind IN ('magic_link', 'account_recovery', 'verification', 'oauth_link', 'password_reset'));
