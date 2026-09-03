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
-- It is NOT an admin decision, and nothing here makes it one. decided_by and
-- decided_at record who ended the row's pending life and when, which for a
-- withdrawal is the requester themselves; decision_note stays for the
-- moderator's words. Every admin write path is scoped to pending or approved
-- rows, so none of them can act on a withdrawn one.
--
-- The dedup index is partial on decision_state = 'pending', so withdrawing a
-- request frees its key: the contributor can file the same name again, and that
-- filing is a new row rather than a replacement. That is the intended shape of
-- withdraw-then-refile.
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
