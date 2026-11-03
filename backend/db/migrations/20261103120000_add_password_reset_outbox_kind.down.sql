SET LOCAL lock_timeout = '5s';

DELETE FROM auth_email_outbox WHERE kind = 'password_reset';

ALTER TABLE auth_email_outbox
    DROP CONSTRAINT IF EXISTS auth_email_outbox_kind_check,
    ADD CONSTRAINT auth_email_outbox_kind_check
        CHECK (kind IN ('magic_link', 'account_recovery', 'verification', 'oauth_link'));
