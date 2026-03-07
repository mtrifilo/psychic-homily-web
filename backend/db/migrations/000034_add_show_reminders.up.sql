ALTER TABLE user_preferences ADD COLUMN show_reminders BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE user_saved_shows ADD COLUMN reminder_sent_at TIMESTAMPTZ;
