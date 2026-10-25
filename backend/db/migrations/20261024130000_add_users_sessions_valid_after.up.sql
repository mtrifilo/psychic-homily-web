-- sessions_valid_after revokes every session an account holds. Each
-- revocation writes a whole second later than any before it; a session token
-- carries the value its mint read and is honored only while that is the
-- current one. NULL, the value every existing account keeps, revokes nothing.
--
-- ADD COLUMN with no default is a catalog change, but it still takes an
-- ACCESS EXCLUSIVE lock on users. lock_timeout bounds the wait for it, so a
-- long-running transaction on users fails this migration instead of queueing
-- every later users read and write behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE users
    ADD COLUMN sessions_valid_after TIMESTAMPTZ;
