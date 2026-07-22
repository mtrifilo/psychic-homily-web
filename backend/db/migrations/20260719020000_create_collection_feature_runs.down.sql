-- PSY-1500: reverse collection_feature_runs. DROP TABLE removes the partial
-- unique index, the featured_at DESC index, and the FKs with it. The backfill
-- rows are dropped too; collections.is_featured is untouched (this migration
-- never altered it), so the boolean state the backfill read from remains the
-- source of truth after a down.
DROP TABLE IF EXISTS collection_feature_runs;
