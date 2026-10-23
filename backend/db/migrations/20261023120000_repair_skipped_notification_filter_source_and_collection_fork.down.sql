-- Drops both columns the up migration restores; dropping a column drops its
-- CHECK, FK and indexes with it. Each statement is a no-op where its column is
-- absent. They are the same columns
-- 20260718140000_notification_filter_source and
-- 20260427173004_collections_forked_from create, so on a database that had
-- them before the up ran this removes them as well, and the schema at the
-- previous version then lacks both columns.
SET LOCAL lock_timeout = '5s';

ALTER TABLE collections
    DROP COLUMN IF EXISTS forked_from_collection_id;

ALTER TABLE notification_filters
    DROP COLUMN IF EXISTS source;
