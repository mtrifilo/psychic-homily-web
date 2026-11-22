-- Undoes 20261122202830 only. Dropping a trigger takes a stronger lock on
-- artists than creating it did, one that waits for readers as well as writers;
-- lock_timeout bounds that wait.
SET LOCAL lock_timeout = '5s';

DROP TRIGGER IF EXISTS artists_track_former_slug_on_insert ON artists;
DROP TRIGGER IF EXISTS artists_track_former_slug_on_update ON artists;
