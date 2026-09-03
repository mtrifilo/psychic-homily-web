-- Reverse PSY-1992's 'withdrawn' decision_state.
--
-- DESTRUCTIVE where any request has been withdrawn: the narrowed CHECK cannot be
-- added while such a row exists, so those rows are rejected first with a note
-- saying what they were. Rejected is the nearest surviving state — the request
-- will not be fulfilled — and it misstates WHO ended the row, which is why the
-- note is written rather than left to be inferred. Nothing recovers the
-- distinction afterwards, and decided_by still names the requester.
--
-- Roll the BINARY back FIRST, mirroring the up migration's ordering: a binary
-- that writes 'withdrawn' against the narrowed constraint fails every withdrawal
-- with a check violation, and the requester sees a 500 on a request that is
-- still queued.

UPDATE entity_requests
SET decision_state = 'rejected',
    decision_note = COALESCE(decision_note, 'Withdrawn by the requester.')
WHERE decision_state = 'withdrawn';

ALTER TABLE entity_requests
    DROP CONSTRAINT IF EXISTS entity_requests_decision_state_check;

ALTER TABLE entity_requests
    ADD CONSTRAINT entity_requests_decision_state_check
    CHECK (decision_state IN ('pending', 'approved', 'rejected'));

COMMENT ON COLUMN entity_requests.decision_state IS NULL;
