-- PSY-1989: restore the show-only pending-request dedup key (PSY-1977's).
--
-- This down migration CAN FAIL, and the failure is informative rather than a
-- defect: the narrower key it restores is the one that made two Fillmores and
-- two festival editions collide, so if any requester holds two PENDING venue
-- requests sharing a name in different cities, or two pending festival requests
-- sharing a name in different editions, the unique index cannot be built. Those
-- rows exist only because the per-type key allowed them.
--
-- ORDER: run this migration and the pre-PSY-1989 deploy TOGETHER. Both mismatched
-- pairings are broken, in different ways, and NEITHER destroys a row.
--
-- OLD binary against the NEW (per-type) index: its name-only venue lookup can
-- select a row the INSERT never collided with, and the replacement it attempts
-- then violates that index, so a contributor's correction 500s deterministically
-- until the rows are resolved. The index refuses the write, so the other row
-- survives.
--
-- NEW binary against the NARROW index this file restores: the opposite direction.
-- Its lookup requires MORE terms than the index collides on, so after a unique
-- violation the lookup finds nothing and the create falls through to the same
-- deterministic 500. It cannot pick a wrong row, because under the narrow index
-- at most one pending row exists per (entity_type, requester_id, name). The
-- CreateRequest fall-through comment describes this case.
--
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
--          lower(trim(coalesce(payload->>'name', payload->>'title'))) AS name_key,
--          left(trim(coalesce(payload->>'event_date', '')), 64) AS occurrence,
--          array_agg(id)
--     FROM entity_requests
--    WHERE decision_state = 'pending'
--    GROUP BY 1, 2, 3, 4
--   HAVING count(*) > 1;
--
-- The failure is ATOMIC and leaves the data alone: this is a multi-statement
-- file, so golang-migrate wraps it in a transaction and the DROP rolls back with
-- the failed CREATE — the per-type index is still in place afterwards, and no
-- window exists in which the queue is unindexed. What the failure does leave
-- behind is schema_migrations marked dirty at the PRECEDING version, so the
-- retry after resolving the rows is `migrate force 20260902223753` and then
-- `down 1`.
--
-- CI's down -all → up round-trip runs on a fresh database, where the table is
-- empty and this is unconditional.

DROP INDEX IF EXISTS uq_entity_requests_pending_dedup;

CREATE UNIQUE INDEX uq_entity_requests_pending_dedup
    ON entity_requests (
        entity_type,
        requester_id,
        (lower(trim(coalesce(payload->>'name', payload->>'title')))),
        (left(trim(coalesce(payload->>'event_date', '')), 64))
    )
    WHERE decision_state = 'pending';
