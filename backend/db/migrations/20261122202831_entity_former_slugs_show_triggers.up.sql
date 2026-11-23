-- The entity_former_slugs triggers on shows (see 20261122202828 for what
-- they keep). They only read and write entity_former_slugs, so they sit beside
-- shows_sync_dedup_keys, which fires on event_date and writes only
-- show_dedup_keys, safely in either firing order. Creating a trigger takes a
-- SHARE ROW EXCLUSIVE lock on shows, which blocks writes to shows until this
-- file's transaction commits, and it is the only table lock this transaction
-- takes; lock_timeout bounds the wait to acquire it, not how long it is held.
SET LOCAL lock_timeout = '5s';

CREATE TRIGGER shows_track_former_slug_on_update
AFTER UPDATE OF slug ON shows
FOR EACH ROW
WHEN (OLD.slug IS DISTINCT FROM NEW.slug AND NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('show');

CREATE TRIGGER shows_track_former_slug_on_insert
AFTER INSERT ON shows
FOR EACH ROW
WHEN (NEW.slug IS NOT NULL AND NEW.slug <> '')
EXECUTE FUNCTION entity_former_slugs_track('show');
