DROP INDEX IF EXISTS idx_notification_filters_user_source;

ALTER TABLE notification_filters
    DROP CONSTRAINT IF EXISTS notification_filters_source_check;

ALTER TABLE notification_filters
    DROP COLUMN IF EXISTS source;
