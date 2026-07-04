-- PSY-1342: Weekly scene digest for followed scenes (from the PSY-1314 spike).
--
-- 1. scene_digest_sent_at on user_bookmarks — per-(scene-follow) cursor for the
--    weekly digest job. NULL = "never digested"; the first cycle looks back to
--    the follow's created_at so a band that appeared between following and the
--    first digest isn't missed. reminder_sent_at (saved-show reminders) is
--    already taken, hence a dedicated column — same precedent.
--
-- 2. notify_on_scene_digest on user_preferences — opt-IN (default FALSE),
--    matching the collection-digest / bulk-sender anti-spam policy: a new
--    email-channel feature ships opt-out-by-default with a UI toggle + RFC 8058
--    one-click unsubscribe in every send. Users opt in via PATCH
--    /auth/preferences/scene-digest.
ALTER TABLE user_bookmarks
    ADD COLUMN scene_digest_sent_at TIMESTAMPTZ;

ALTER TABLE user_preferences
    ADD COLUMN notify_on_scene_digest BOOLEAN NOT NULL DEFAULT FALSE;
