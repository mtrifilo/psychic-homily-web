-- Reverse 20261017120000_create_auth_email_outbox.up.sql. Dropping the table
-- drops its indexes and constraints with it. Emails still queued are lost:
-- the people who asked for them get nothing and must ask again, and the audit
-- rows go too. Nothing else references this table.
SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS auth_email_outbox;
