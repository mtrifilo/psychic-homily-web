-- PSY-289: Comment notifications + mention parsing (Wave 2)
--
-- 1. Add last_notified_at to comment_subscriptions for per-hour dedup of
--    subscriber notifications (each send updates this timestamp; queue skips
--    subscribers whose last_notified_at is within the last hour).
-- 2. Add two user preference flags so users can opt out of comment-subscription
--    and mention emails independently of each other and of existing
--    notification_email / show_reminders flags.

ALTER TABLE comment_subscriptions
    ADD COLUMN last_notified_at TIMESTAMPTZ;

ALTER TABLE user_preferences
    ADD COLUMN notify_on_comment_subscription BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN notify_on_mention BOOLEAN NOT NULL DEFAULT TRUE;
