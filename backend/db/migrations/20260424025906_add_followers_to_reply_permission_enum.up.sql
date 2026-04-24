-- PSY-296: Add 'followers' as a valid reply permission value.
--
-- The `comments.reply_permission` column is a plain VARCHAR(20) with no
-- DB-level enum or CHECK constraint (see migration 000064). Valid values
-- today are enforced in application code: 'anyone' (default) and
-- 'author_only'. This migration adds 'followers' to the application-valid
-- set by attaching a CHECK constraint that includes all three values.
--
-- Additive only: existing 'anyone' and 'author_only' rows remain valid.

ALTER TABLE comments
    ADD CONSTRAINT comments_reply_permission_check
    CHECK (reply_permission IN ('anyone', 'followers', 'author_only'));

-- Per-user default reply permission applied to new top-level comments.
ALTER TABLE user_preferences
    ADD COLUMN default_reply_permission VARCHAR(20) NOT NULL DEFAULT 'anyone';

ALTER TABLE user_preferences
    ADD CONSTRAINT user_preferences_default_reply_permission_check
    CHECK (default_reply_permission IN ('anyone', 'followers', 'author_only'));
