-- calendar_feed_reads records which calendar clients have fetched a feed
-- link: one row per (token, client family), with the first and the most
-- recent fetch. The family is the server's classification of the request's
-- User-Agent; the User-Agent itself is never stored. A row belongs to one
-- token, so deleting or replacing the token deletes its reads.
--
-- user_preferences.calendar_link_reset_at and calendar_link_reset_reason say
-- when and why a security revocation deleted the account's feed link, so the
-- site can explain why a subscribed calendar stopped updating. Both are NULL
-- when no such reset is outstanding, and they are set and cleared together.
--
-- CREATE TABLE ... REFERENCES calendar_tokens and ALTER TABLE
-- user_preferences each take a table lock; lock_timeout bounds the wait for
-- either, so a long-running transaction fails this migration instead of
-- queueing feed lookups and preference reads behind it.
SET LOCAL lock_timeout = '5s';

CREATE TABLE calendar_feed_reads (
    token_id      BIGINT      NOT NULL REFERENCES calendar_tokens(id) ON DELETE CASCADE,
    client        TEXT        NOT NULL,
    first_read_at TIMESTAMPTZ NOT NULL,
    last_read_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (token_id, client),
    CONSTRAINT calendar_feed_reads_client_check
        CHECK (client IN ('google', 'apple_ios', 'apple_mac', 'other'))
);

ALTER TABLE user_preferences
    ADD COLUMN calendar_link_reset_at TIMESTAMPTZ,
    ADD COLUMN calendar_link_reset_reason TEXT,
    ADD CONSTRAINT user_preferences_calendar_link_reset_reason_check
        CHECK (calendar_link_reset_reason IN ('sign_out_everywhere', 'password_reset')),
    ADD CONSTRAINT user_preferences_calendar_link_reset_pair_check
        CHECK ((calendar_link_reset_at IS NULL) = (calendar_link_reset_reason IS NULL));
