ALTER TABLE user_preferences
    DROP COLUMN IF EXISTS alert_defaults,
    DROP COLUMN IF EXISTS home_metro;
