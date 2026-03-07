ALTER TABLE user_preferences DROP COLUMN IF EXISTS show_reminders;
ALTER TABLE user_saved_shows DROP COLUMN IF EXISTS reminder_sent_at;
