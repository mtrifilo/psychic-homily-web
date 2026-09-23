-- Restores the schema only: every row gets the column defaults, not the
-- values it held before the up migration ran.
ALTER TABLE user_preferences
    ADD COLUMN IF NOT EXISTS notification_email BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS notification_push BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS theme VARCHAR(50) NOT NULL DEFAULT 'light',
    ADD COLUMN IF NOT EXISTS timezone VARCHAR(50) NOT NULL DEFAULT 'UTC',
    ADD COLUMN IF NOT EXISTS language VARCHAR(10) NOT NULL DEFAULT 'en';
