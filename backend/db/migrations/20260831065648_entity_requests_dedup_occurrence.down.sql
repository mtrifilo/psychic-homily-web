-- PSY-1977: restore the title-only pending-request dedup key.
--
-- This down migration CAN FAIL, and the failure is informative rather than a
-- defect: the narrower key it restores is the one that made same-titled requests
-- collide, so if any requester has two PENDING requests of one entity_type
-- sharing a name and differing only by date, the unique index cannot be built.
-- Those rows exist only because the wide key allowed them.
--
-- The operator's remedy is to resolve the collision EXPLICITLY before rolling
-- back, never to let a rollback pick a winner: decide (approve or reject) all
-- but one of each colliding group, since the index is partial on
-- decision_state = 'pending' and a decided row no longer participates. Find them
-- with:
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
