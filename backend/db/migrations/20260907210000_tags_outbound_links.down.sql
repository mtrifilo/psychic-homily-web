-- Reverse PSY-1888's tags outbound-link columns.
--
-- DESTRUCTIVE: dropping these columns discards every stored link. Nothing else
-- holds the values, so a rollback that is later rolled forward starts empty.

ALTER TABLE tags
    DROP COLUMN IF EXISTS website,
    DROP COLUMN IF EXISTS instagram,
    DROP COLUMN IF EXISTS bandcamp;
