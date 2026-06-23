-- PSY-1179: reverse the venues.capacity column. DROP COLUMN IF EXISTS lands the
-- up->down->up CI round-trip back on the pre-PSY-1179 schema exactly.
ALTER TABLE venues
    DROP COLUMN IF EXISTS capacity;
