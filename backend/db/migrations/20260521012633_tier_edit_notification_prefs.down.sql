-- Revert PSY-756 additions.

ALTER TABLE user_preferences
    DROP COLUMN IF EXISTS notify_on_edit_notifications,
    DROP COLUMN IF EXISTS notify_on_tier_notifications;
