SET LOCAL lock_timeout = '5s';

ALTER TABLE auth_email_outbox
    DROP CONSTRAINT IF EXISTS auth_email_outbox_return_to_check,
    DROP COLUMN IF EXISTS return_to;
