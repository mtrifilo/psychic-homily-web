-- PSY-296 rollback: drop the reply_permission CHECK constraint and
-- per-user default reply preference.
--
-- Note: any rows with reply_permission = 'followers' remain in the comments
-- table after rollback. They will be silently treated as the default by
-- application code that doesn't recognize the value. Callers rolling back
-- should run a data migration if they want to normalize these to 'anyone'.

ALTER TABLE user_preferences
    DROP CONSTRAINT IF EXISTS user_preferences_default_reply_permission_check;

ALTER TABLE user_preferences
    DROP COLUMN IF EXISTS default_reply_permission;

ALTER TABLE comments
    DROP CONSTRAINT IF EXISTS comments_reply_permission_check;
