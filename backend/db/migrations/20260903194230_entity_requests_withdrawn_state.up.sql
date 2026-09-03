-- PSY-1992: 'withdrawn' joins the decision_state vocabulary.
--
-- A contributor could not retract a queued request. With replace-on-resubmit
-- (PSY-1948) the only way to undo one was to overwrite it with junk, which
-- leaves the admin a request to reject rather than nothing to review.
--
-- A STATE, not a delete. The row keeps its id, its requester, its payload and
-- its place in the audit trail, so a withdrawal is a fact the queue can read
-- rather than an absence it has to infer. It also keeps the moderation queue's
-- read model intact: every surface already narrows on decision_state, so a
-- withdrawn row is excluded by the same filter that excludes a rejected one, and
-- an admin who wants to see them asks for them.
--
-- It is NOT an admin decision: decided_by names the requester on these rows.
-- EntityRequestService.Withdraw owns that and the freed dedup key.
--
-- ORDERING: run this BEFORE the binary that writes 'withdrawn'. The old binary
-- against the widened constraint is harmless (it writes no such value); the new
-- binary against the old constraint fails every withdrawal with a check
-- violation.

ALTER TABLE entity_requests
    DROP CONSTRAINT IF EXISTS entity_requests_decision_state_check;

ALTER TABLE entity_requests
    ADD CONSTRAINT entity_requests_decision_state_check
    CHECK (decision_state IN ('pending', 'approved', 'rejected', 'withdrawn'));

COMMENT ON COLUMN entity_requests.decision_state IS
    'pending, approved or rejected by an admin, or withdrawn by the requester while it was still pending (PSY-1992).';
