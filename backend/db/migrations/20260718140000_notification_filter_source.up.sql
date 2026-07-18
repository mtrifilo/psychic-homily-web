-- PSY-1467: explicit ownership for quick-created entity alert subscriptions.
-- source='user'     — authored via Notification Filters settings (default)
-- source='managed'  — created/owned by entity-page NotifyMeButton quick toggle
--
-- Existing rows (if any) default to 'user' so the quick toggle never deletes them.
-- Rollback: drop the column (see .down.sql). No data backfill required; zero users.

ALTER TABLE notification_filters
    ADD COLUMN source VARCHAR(16) NOT NULL DEFAULT 'user';

ALTER TABLE notification_filters
    ADD CONSTRAINT notification_filters_source_check
    CHECK (source IN ('user', 'managed'));

CREATE INDEX idx_notification_filters_user_source
    ON notification_filters (user_id, source)
    WHERE is_active = TRUE;
