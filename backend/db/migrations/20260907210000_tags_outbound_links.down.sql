-- Reverse PSY-1888's tags outbound-link columns.
--
-- DESTRUCTIVE: dropping these columns discards every stored link. Nothing else
-- holds the values, so a rollback that is later rolled forward starts empty.
--
-- ROLL THE BINARY BACK TOO. The three columns are in requiredSchemaColumns
-- (db/schema_assertion.go), so a server built at or after the migration that
-- added them refuses to BOOT against a database this has been run on. Running
-- this to unblock a deploy, on its own, turns a degraded surface into an
-- outage plus permanent data loss.

ALTER TABLE tags
    DROP COLUMN IF EXISTS website,
    DROP COLUMN IF EXISTS instagram,
    DROP COLUMN IF EXISTS bandcamp;
