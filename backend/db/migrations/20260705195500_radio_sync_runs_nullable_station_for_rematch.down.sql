ALTER TABLE radio_sync_runs
    DROP CONSTRAINT IF EXISTS radio_sync_runs_station_required_except_rematch_check;

-- Restore NOT NULL only when no orphaned global rematch rows exist.
ALTER TABLE radio_sync_runs
    ALTER COLUMN station_id SET NOT NULL;
