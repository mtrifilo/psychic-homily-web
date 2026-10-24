-- Restores two columns, with their constraints and indexes, exactly as
-- 20260718140000_notification_filter_source and
-- 20260427173004_collections_forked_from define them, on a database whose
-- schema_migrations pointer is past those versions but whose DDL is absent.
-- Every statement is a no-op where its object already exists, so this applies
-- cleanly both there and on a database migrated from scratch.
--
-- The constraint names are the ones those migrations produce: the CHECK is
-- named explicitly there, and the FK takes the name Postgres gives an inline
-- column REFERENCES on that column.
--
-- Adding a column takes an ACCESS EXCLUSIVE lock on its table, and a
-- non-concurrent index build a SHARE lock, held until this file commits.
-- lock_timeout bounds the wait for them, so a long-running read fails this
-- migration instead of queueing every later query on either table behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE notification_filters
    ADD COLUMN IF NOT EXISTS source VARCHAR(16) NOT NULL DEFAULT 'user';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'notification_filters'::regclass
          AND conname = 'notification_filters_source_check'
    ) THEN
        ALTER TABLE notification_filters
            ADD CONSTRAINT notification_filters_source_check
            CHECK (source IN ('user', 'managed'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_notification_filters_user_source
    ON notification_filters (user_id, source)
    WHERE is_active = TRUE;

-- The column is added bare and the FK separately: ADD COLUMN IF NOT EXISTS
-- skips an inline REFERENCES along with the column when the column exists.
ALTER TABLE collections
    ADD COLUMN IF NOT EXISTS forked_from_collection_id BIGINT;

-- ON DELETE SET NULL: deleting a collection keeps its forks.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'collections'::regclass
          AND conname = 'collections_forked_from_collection_id_fkey'
    ) THEN
        ALTER TABLE collections
            ADD CONSTRAINT collections_forked_from_collection_id_fkey
            FOREIGN KEY (forked_from_collection_id)
            REFERENCES collections(id) ON DELETE SET NULL;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_collections_forked_from
    ON collections(forked_from_collection_id)
    WHERE forked_from_collection_id IS NOT NULL;
