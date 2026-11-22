-- Undoes 20261122202829 only. Dropping a trigger takes a stronger lock on
-- venues than creating it did, one that waits for readers as well as writers;
-- lock_timeout bounds that wait.
SET LOCAL lock_timeout = '5s';

DROP TRIGGER IF EXISTS venues_track_former_slug_on_insert ON venues;
DROP TRIGGER IF EXISTS venues_track_former_slug_on_update ON venues;
