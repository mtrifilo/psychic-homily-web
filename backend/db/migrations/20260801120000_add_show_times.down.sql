-- Rollback. Nothing else references these columns, and event_date is
-- untouched by the up migration, so dropping them restores the prior schema
-- exactly. Any doors/music values entered while they existed are lost.
--
-- ROLL THE APP BACK FIRST. Any server build from this change onward asserts
-- both columns at boot (db/schema_assertion.go, called from cmd/server), so
-- running this against a current binary turns one bad deploy into a refusing
-- to boot loop. Deploy a pre-PSY-1681 build, then migrate down.
ALTER TABLE shows
    DROP COLUMN IF EXISTS music_at,
    DROP COLUMN IF EXISTS doors_at;
