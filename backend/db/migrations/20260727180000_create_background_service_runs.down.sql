-- PSY-1611 rollback. Dropping the table loses run history; the loops fall back
-- to their bounded start-delay schedule (shared.RunTickerLoop degrades to
-- non-persistent mode when the store is unavailable), so a rollback is safe
-- rather than a boot failure.
DROP TABLE IF EXISTS background_service_runs;
