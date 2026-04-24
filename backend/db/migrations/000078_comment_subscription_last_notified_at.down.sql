-- Revert PSY-289 additions.

ALTER TABLE user_preferences
    DROP COLUMN IF EXISTS notify_on_mention,
    DROP COLUMN IF EXISTS notify_on_comment_subscription;

ALTER TABLE comment_subscriptions
    DROP COLUMN IF EXISTS last_notified_at;
