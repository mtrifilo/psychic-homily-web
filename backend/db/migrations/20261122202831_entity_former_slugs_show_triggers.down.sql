-- Undoes 20261122202831 only. Dropping a trigger takes a stronger lock on
-- shows than creating it did, one that waits for readers as well as writers;
-- lock_timeout bounds that wait.
SET LOCAL lock_timeout = '5s';

DROP TRIGGER IF EXISTS shows_track_former_slug_on_insert ON shows;
DROP TRIGGER IF EXISTS shows_track_former_slug_on_update ON shows;
