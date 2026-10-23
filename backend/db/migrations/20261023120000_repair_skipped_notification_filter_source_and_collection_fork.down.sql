-- Drops every object the up migration restores; each statement is a no-op
-- where its object is absent. They are the same objects
-- 20260718140000_notification_filter_source and
-- 20260427173004_collections_forked_from create, so on a database that had
-- them before the up ran this removes them as well, and the schema at the
-- previous version then lacks both columns.
SET LOCAL lock_timeout = '5s';

DROP INDEX IF EXISTS idx_collections_forked_from;

ALTER TABLE collections
    DROP CONSTRAINT IF EXISTS collections_forked_from_collection_id_fkey;

ALTER TABLE collections
    DROP COLUMN IF EXISTS forked_from_collection_id;

DROP INDEX IF EXISTS idx_notification_filters_user_source;

ALTER TABLE notification_filters
    DROP CONSTRAINT IF EXISTS notification_filters_source_check;

ALTER TABLE notification_filters
    DROP COLUMN IF EXISTS source;
