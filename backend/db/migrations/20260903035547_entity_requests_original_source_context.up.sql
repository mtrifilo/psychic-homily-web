-- PSY-1978: record the source_context a queued request was ORIGINALLY filed
-- with, so a replacement cannot silently erase AI provenance.
--
-- PSY-1948 made a resubmission REPLACE a queued request's payload,
-- source_context and source_detail. That was deliberate — the three describe one
-- submission, and a resubmission is the requester's current intent. What it also
-- means is that a request filed as 'ai_extraction' with a source article, then
-- resubmitted as 'manual' with nothing, becomes a plain manual request. Nothing
-- on the row, and nothing an admin reads, says the AI extraction happened. The
-- admin list endpoint's source_context filter narrows on the live value, so a
-- client asking for ai_extraction rows does not get that one back either.
--
-- This column answers ONE question the row otherwise cannot: what was filed
-- first. It is NULL for a row that still holds its first submission, and is
-- written only when it is still NULL, so a row replaced repeatedly keeps naming
-- the ORIGINAL filing rather than the second-to-last thing it said. Each
-- individual replacement is recorded on its own replace_entity_request
-- audit_logs row, which names the source_context that replacement superseded and
-- a digest of the payload it destroyed. Neither is a filter: this column is a
-- column, and it is the one a query can narrow on.
--
-- No CHECK constraint and no backfill:
--
--   * Unconstrained on purpose. source_context carries the CHECK because it is
--     the row's live claim and a bad value there breaks fulfilment. This column
--     is a historical record of a value that already passed that CHECK when it
--     was written, and a vocabulary that later loses a member must not make an
--     old row unwritable.
--   * Existing rows stay NULL, which is the truthful answer for every one of
--     them: whether they were ever replaced was not recorded, so claiming they
--     were not would be an invention. NULL reads as "no recorded revision" on
--     the moderation card, which is what those rows are.

ALTER TABLE entity_requests
    ADD COLUMN original_source_context TEXT;

COMMENT ON COLUMN entity_requests.original_source_context IS
    'source_context of the submission originally filed under this row; NULL until a resubmission replaces it (PSY-1978).';
