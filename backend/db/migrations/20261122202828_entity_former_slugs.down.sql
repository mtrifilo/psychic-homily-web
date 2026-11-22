-- Dropping a trigger takes the same SHARE ROW EXCLUSIVE lock creating it did.
SET LOCAL lock_timeout = '5s';

DROP TRIGGER IF EXISTS artists_track_former_slug_on_insert ON artists;
DROP TRIGGER IF EXISTS artists_track_former_slug_on_update ON artists;
DROP TRIGGER IF EXISTS venues_track_former_slug_on_insert ON venues;
DROP TRIGGER IF EXISTS venues_track_former_slug_on_update ON venues;
DROP TRIGGER IF EXISTS shows_track_former_slug_on_insert ON shows;
DROP TRIGGER IF EXISTS shows_track_former_slug_on_update ON shows;

DROP FUNCTION IF EXISTS entity_former_slugs_track();

DROP TABLE IF EXISTS entity_former_slugs;
