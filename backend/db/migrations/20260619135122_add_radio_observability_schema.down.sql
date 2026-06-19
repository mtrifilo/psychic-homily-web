-- PSY-1132: Reverse the radio observability schema. Drop the three new tables in
-- reverse dependency order — radio_sync_run_errors (child of radio_sync_runs)
-- first, then radio_sync_runs, then the independent radio_station_health.
-- DROP TABLE removes the tables' indexes and CHECK/FK constraints with them, so
-- the up->down->up CI round-trip lands back on the pre-PSY-1132 schema exactly.

DROP TABLE IF EXISTS radio_sync_run_errors;
DROP TABLE IF EXISTS radio_sync_runs;
DROP TABLE IF EXISTS radio_station_health;
