-- auth_email_outbox gains the destination a magic-link request names: the
-- frontend path the sign-in page was going to return to, already checked by
-- the same rule as the Google sign-in's destination. The poller signs it into
-- the link's token at send time, and verifying that token returns it, so it is
-- never taken from the emailed URL. NULL is no destination (the site root);
-- only a magic_link row may carry one.
--
-- ADD COLUMN without a default is a catalog change. The ALTER takes an ACCESS
-- EXCLUSIVE lock on the table and the CHECK scans it; the table holds at most
-- a week of rows. lock_timeout bounds the wait, so a long-running transaction
-- fails this migration instead of queueing the table's later reads and writes
-- behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE auth_email_outbox
    ADD COLUMN return_to VARCHAR(2048),
    ADD CONSTRAINT auth_email_outbox_return_to_check
        CHECK (return_to IS NULL OR kind = 'magic_link');
