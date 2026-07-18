-- PSY-1423: Persist chart window + scene defaults per user.
-- Nullable JSONB object: {"window":"month"|"quarter"|"all_time","scene":"<metro>"|null}.
-- NULL column = no saved defaults (anonymous quarter / all-scenes behavior).
ALTER TABLE user_preferences
    ADD COLUMN chart_defaults JSONB;
