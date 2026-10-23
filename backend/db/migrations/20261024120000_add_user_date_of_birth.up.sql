-- The date of birth an account was created with, and when it was given.
-- date_of_birth is a calendar date with no zone. age_attested_at is the
-- instant the server computed the age from it. Both are NULL for every
-- account created before a sign-up path asked for the date, and for any path
-- that still does not.
--
-- ADD COLUMN with no default is a catalog change, but it still takes an
-- ACCESS EXCLUSIVE lock on users. lock_timeout bounds the wait for it, so a
-- long-running transaction on users fails this migration instead of queueing
-- every later users read and write behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE users
    ADD COLUMN date_of_birth DATE,
    ADD COLUMN age_attested_at TIMESTAMPTZ;
