-- PSY-1977: restore the title-only pending-request dedup key.
--
-- This down migration CAN FAIL, and the failure is informative rather than a
-- defect: the narrower key it restores is the one that made same-titled requests
-- collide, so if any requester has two PENDING requests of one entity_type
-- sharing a name and differing only by date, the unique index cannot be built.
-- Those rows exist only because the wide key allowed them.
--
-- ORDER: run this migration and the pre-PSY-1977 deploy TOGETHER. Unlike the
-- usual convention in this directory (roll the app back first), the old binary
-- is the half that is incompatible with the NEW schema: against the wide index
-- its name-only lookup can select a row the INSERT never collided with, and the
-- replacement it attempts then violates the wide index, so a contributor's
-- correction 500s deterministically. It does not destroy the other row (verified
-- against a live Postgres), but the endpoint is unusable for that name until the
-- rows are resolved. The new binary, by contrast, works correctly against the
-- narrow index this file restores — its extra key term only narrows the lookup.
-- So: resolve the rows, then migrate down, then deploy the old build.
--
-- The operator's remedy is to resolve the collision EXPLICITLY before rolling
-- back, never to let a rollback pick a winner: decide (approve or reject) all
-- but one of each colliding group, since the index is partial on
-- decision_state = 'pending' and a decided row no longer participates. Be honest
-- about what that costs — these rows are NOT duplicates. They are the distinct
-- requests the up migration exists to preserve, so rejecting one discards a real
-- contributor submission. Tell the requester, or approve it, rather than
-- treating it as queue cleanup. Find the groups with:
--
--   SELECT entity_type, requester_id,
--          lower(trim(coalesce(payload->>'name', payload->>'title'))) AS key,
--          array_agg(id)
--     FROM entity_requests
--    WHERE decision_state = 'pending'
--    GROUP BY 1, 2, 3
--   HAVING count(*) > 1;
--
-- The failure is ATOMIC and leaves the data alone: this is a multi-statement
-- file, so golang-migrate wraps it in a transaction and the DROP rolls back with
-- the failed CREATE — the WIDE index is still in place afterwards, and no window
-- exists in which the queue is unindexed. What the failure does leave behind is
-- schema_migrations marked dirty at the PRECEDING version, so the retry after
-- resolving the rows is `migrate force 20260831065648` and then `down 1`.
--
-- CI's down -all → up round-trip runs on a fresh database, where the table is
-- empty and this is unconditional.

DROP INDEX IF EXISTS uq_entity_requests_pending_dedup;

CREATE UNIQUE INDEX uq_entity_requests_pending_dedup
    ON entity_requests (
        entity_type,
        requester_id,
        (lower(trim(coalesce(payload->>'name', payload->>'title'))))
    )
    WHERE decision_state = 'pending';
