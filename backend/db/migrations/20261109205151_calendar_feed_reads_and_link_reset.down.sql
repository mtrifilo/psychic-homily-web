SET LOCAL lock_timeout = '5s';

ALTER TABLE user_preferences
    DROP CONSTRAINT IF EXISTS user_preferences_calendar_link_reset_pair_check,
    DROP CONSTRAINT IF EXISTS user_preferences_calendar_link_reset_reason_check,
    DROP COLUMN IF EXISTS calendar_link_reset_reason,
    DROP COLUMN IF EXISTS calendar_link_reset_at;

DROP TABLE IF EXISTS calendar_feed_reads;
