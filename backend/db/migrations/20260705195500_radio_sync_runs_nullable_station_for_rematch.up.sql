-- Global rematch runs (run_type='rematch', no station filter) have no natural
-- station FK. Nullable station_id is allowed only for rematch rows (PSY-1364).
ALTER TABLE radio_sync_runs
    ALTER COLUMN station_id DROP NOT NULL;

ALTER TABLE radio_sync_runs
    ADD CONSTRAINT radio_sync_runs_station_required_except_rematch_check
        CHECK (station_id IS NOT NULL OR run_type = 'rematch');
