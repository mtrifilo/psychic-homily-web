-- PSY-2030: one mailbox is one account.
--
-- users.email already carries a UNIQUE constraint, but it compares raw bytes,
-- so Bob@x.com and bob@x.com are two rows for one mailbox. Every application
-- lookup now compares lower(email) = lower(?) (authm.EmailIdentityWhere), and
-- this index is what makes that comparison an identity rather than a
-- convention: without it a check-then-insert can still admit a second row for
-- the same mailbox under concurrency.
--
-- lower() here and lower() in the lookup are the same Postgres function, so
-- the index and the query agree on case by construction. Folding in the
-- application instead would let Go's case rules disagree with the index's.
--
-- NULL email stays legal and unconstrained: lower(NULL) is NULL, and a unique
-- index permits any number of NULLs, so OAuth-only accounts with no address
-- are unaffected.
--
-- Not CONCURRENTLY: golang-migrate wraps each migration in a transaction, and
-- CREATE INDEX CONCURRENTLY cannot run inside one. A plain CREATE INDEX holds
-- a SHARE lock on users, which blocks concurrent writes for the duration of
-- the build; that window is acceptable at this table's size.
--
-- The build FAILS the deploy if any two live rows collide on lower(email)
-- rather than silently keeping one. That is the intended behaviour: a
-- collision is a pair of accounts a human has to merge or rename, not
-- something a migration may decide.

CREATE UNIQUE INDEX users_lower_email_key ON users (lower(email));

COMMENT ON INDEX users_lower_email_key IS
    'Case-insensitive uniqueness for users.email. Pairs with authm.EmailIdentityWhere, the lower(email) = lower(?) clause every lookup by address uses. Stored bytes keep the casing the owner typed.';
