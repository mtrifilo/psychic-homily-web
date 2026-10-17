-- auth_email_outbox holds the magic-link and account-recovery emails the
-- request handlers have promised to send. A handler inserts one row for an
-- eligible account and answers at once; a poller in the server sends it. The
-- handlers answer an unknown address and a known one in the same time because
-- neither waits on the email provider.
--
-- A row names the account and the kind of email, nothing else. The poller
-- reads the account at send time and mints the link token then, so no bearer
-- token is ever stored here, and a link's lifetime starts when it is sent.
--
-- One pending or processing row per (user_id, kind): a repeated request while
-- one is still queued is ON CONFLICT DO NOTHING and the person gets one email.
-- Terminal rows ('sent', 'skipped', 'failed') are kept for a bounded audit
-- window and then deleted by the poller, so the table stays small.
--
-- next_attempt_at is the backoff: the claim takes only rows whose time has
-- come, and a failed send pushes it forward.
--
-- New, empty table. The CREATE INDEX statements lock only this table, which
-- nothing reads until this migration commits. The REFERENCES users clause
-- takes a SHARE ROW EXCLUSIVE lock on users, which waits behind any open write
-- to users; lock_timeout bounds that wait so a long transaction on users fails
-- this migration instead of queueing every later users query behind it.
SET LOCAL lock_timeout = '5s';

CREATE TABLE auth_email_outbox (
    id BIGSERIAL PRIMARY KEY,
    kind VARCHAR(32) NOT NULL
        CONSTRAINT auth_email_outbox_kind_check
        CHECK (kind IN ('magic_link', 'account_recovery')),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CONSTRAINT auth_email_outbox_status_check
        CHECK (status IN ('pending', 'processing', 'sent', 'skipped', 'failed')),
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

-- At most one queued email per account and kind.
CREATE UNIQUE INDEX uq_auth_email_outbox_active
    ON auth_email_outbox (user_id, kind)
    WHERE status IN ('pending', 'processing');

-- The claim: WHERE status = 'pending' AND next_attempt_at <= now ORDER BY
-- created_at LIMIT n FOR UPDATE SKIP LOCKED.
CREATE INDEX idx_auth_email_outbox_pending
    ON auth_email_outbox (created_at)
    WHERE status = 'pending';

-- The retention prune: terminal rows older than the audit window.
CREATE INDEX idx_auth_email_outbox_terminal
    ON auth_email_outbox (updated_at)
    WHERE status IN ('sent', 'skipped', 'failed');
