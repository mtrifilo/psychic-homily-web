-- The entity_former_slugs triggers on artists (see 20261122202828 for what
-- they keep). Creating a trigger takes a SHARE ROW EXCLUSIVE lock on artists,
-- which blocks writes to artists until this file's transaction commits, and it
-- is the only table lock this transaction takes; lock_timeout bounds the wait
-- to acquire it, not how long it is held.
SET LOCAL lock_timeout = '5s';

CREATE TRIGGER artists_track_former_slug_on_update
AFTER UPDATE OF slug ON artists
FOR EACH ROW
WHEN (OLD.slug IS DISTINCT FROM NEW.slug AND NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('artist');

CREATE TRIGGER artists_track_former_slug_on_insert
AFTER INSERT ON artists
FOR EACH ROW
WHEN (NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('artist');
