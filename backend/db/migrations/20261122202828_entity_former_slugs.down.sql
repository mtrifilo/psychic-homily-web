-- Dropping a trigger takes a stronger lock on its table than creating it did,
-- one that waits for readers as well as writers; lock_timeout bounds the wait.
SET LOCAL lock_timeout = '5s';

DROP TRIGGER IF EXISTS artists_track_former_slug_on_insert ON artists;
DROP TRIGGER IF EXISTS artists_track_former_slug_on_update ON artists;
DROP TRIGGER IF EXISTS venues_track_former_slug_on_insert ON venues;
DROP TRIGGER IF EXISTS venues_track_former_slug_on_update ON venues;
DROP TRIGGER IF EXISTS shows_track_former_slug_on_insert ON shows;
DROP TRIGGER IF EXISTS shows_track_former_slug_on_update ON shows;

DROP FUNCTION IF EXISTS entity_former_slugs_track();

DROP TABLE IF EXISTS entity_former_slugs;
