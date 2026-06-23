-- PSY-1186: reverse schedule_locked. DROP COLUMN IF EXISTS lands the up->down->up CI
-- round-trip back on the pre-PSY-1186 schema exactly.
ALTER TABLE radio_shows
    DROP COLUMN IF EXISTS schedule_locked;
