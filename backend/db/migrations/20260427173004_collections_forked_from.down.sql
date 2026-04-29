-- PSY-351: revert collections fork support.
DROP INDEX IF EXISTS idx_collections_forked_from;
ALTER TABLE collections DROP COLUMN IF EXISTS forked_from_collection_id;
