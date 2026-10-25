-- A pending link is a provider identity whose verified address belongs to an
-- account it is not attached to, held between the sign-in callback and a
-- proof that the person holds that account: its password, or its mailbox by
-- an emailed link. It lives in oauth_pending_signups beside the pending
-- sign-ups, told apart by purpose, and names the account it would join in
-- user_id. A sign-up row names no account.
--
-- auth_email_outbox gains the emailed link's kind. Its row names the pending
-- link by oauth_link_ref, that row's token_hash, which the emailed token
-- carries; no bearer token is stored.
--
-- ADD COLUMN with a constant default is a catalog change. Each ALTER takes an
-- ACCESS EXCLUSIVE lock on its table, and the REFERENCES users clause a SHARE
-- ROW EXCLUSIVE lock on users; lock_timeout bounds every wait, so a
-- long-running transaction fails this migration instead of queueing the
-- tables' later reads and writes behind it. The CHECK constraints scan two
-- small tables.
SET LOCAL lock_timeout = '5s';

ALTER TABLE oauth_pending_signups
    ADD COLUMN purpose VARCHAR(16) NOT NULL DEFAULT 'signup'
        CONSTRAINT oauth_pending_signups_purpose_check
        CHECK (purpose IN ('signup', 'link')),
    ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT oauth_pending_signups_link_names_account_check
        CHECK ((purpose = 'link') = (user_id IS NOT NULL));

-- The users FK's ON DELETE CASCADE.
CREATE INDEX idx_oauth_pending_signups_user
    ON oauth_pending_signups (user_id)
    WHERE user_id IS NOT NULL;

ALTER TABLE auth_email_outbox
    DROP CONSTRAINT auth_email_outbox_kind_check,
    ADD CONSTRAINT auth_email_outbox_kind_check
        CHECK (kind IN ('magic_link', 'account_recovery', 'verification', 'oauth_link')),
    ADD COLUMN oauth_link_ref CHAR(64),
    ADD CONSTRAINT auth_email_outbox_oauth_link_ref_check
        CHECK ((kind = 'oauth_link') = (oauth_link_ref IS NOT NULL));
