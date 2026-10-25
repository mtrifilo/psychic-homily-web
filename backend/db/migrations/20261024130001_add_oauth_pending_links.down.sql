SET LOCAL lock_timeout = '5s';

DELETE FROM auth_email_outbox WHERE kind = 'oauth_link';

ALTER TABLE auth_email_outbox
    DROP CONSTRAINT IF EXISTS auth_email_outbox_oauth_link_ref_check,
    DROP COLUMN IF EXISTS oauth_link_ref,
    DROP CONSTRAINT IF EXISTS auth_email_outbox_kind_check,
    ADD CONSTRAINT auth_email_outbox_kind_check
        CHECK (kind IN ('magic_link', 'account_recovery', 'verification'));

DELETE FROM oauth_pending_signups WHERE purpose = 'link';

DROP INDEX IF EXISTS idx_oauth_pending_signups_user;

ALTER TABLE oauth_pending_signups
    DROP CONSTRAINT IF EXISTS oauth_pending_signups_link_names_account_check,
    DROP COLUMN IF EXISTS user_id,
    DROP COLUMN IF EXISTS purpose;
