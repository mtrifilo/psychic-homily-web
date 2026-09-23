-- Drop the five user_preferences columns that no product surface reads or
-- writes: notification_email, notification_push, theme, timezone, language.
-- Theme is a device-local setting held by the browser, not an account column.
ALTER TABLE user_preferences
    DROP COLUMN IF EXISTS notification_email,
    DROP COLUMN IF EXISTS notification_push,
    DROP COLUMN IF EXISTS theme,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS language;
