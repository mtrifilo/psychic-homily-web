-- The entity_former_slugs triggers on venues (see 20261122202828 for what
-- they keep). Creating a trigger takes a SHARE ROW EXCLUSIVE lock on venues,
-- which blocks writes to venues until this file's transaction commits, and it
-- is the only table lock this transaction takes; lock_timeout bounds the wait
-- to acquire it, not how long it is held.
SET LOCAL lock_timeout = '5s';

CREATE TRIGGER venues_track_former_slug_on_update
AFTER UPDATE OF slug ON venues
FOR EACH ROW
WHEN (OLD.slug IS DISTINCT FROM NEW.slug AND NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('venue');

CREATE TRIGGER venues_track_former_slug_on_insert
AFTER INSERT ON venues
FOR EACH ROW
WHEN (NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('venue');
