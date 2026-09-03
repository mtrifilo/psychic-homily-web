-- PSY-1978: record the source_context a queued request was ORIGINALLY filed
-- with, so a replacement cannot silently erase AI provenance.
--
-- PSY-1948 made a resubmission REPLACE a queued request's payload,
-- source_context and source_detail. That was deliberate — the three describe one
-- submission, and a resubmission is the requester's current intent. What it also
-- means is that a request filed as 'ai_extraction' with a source article, then
-- resubmitted as 'manual' with nothing, becomes a plain manual request. It drops
-- out of the source_context=ai_extraction admin filter and no admin can tell the
-- evidence ever existed.
--
-- This column answers ONE question the row otherwise cannot: what was filed
-- first. It is NULL for a row that still holds its first submission, and is
-- written only when it is still NULL, so a row replaced repeatedly keeps naming
-- the ORIGINAL filing rather than the second-to-last thing it said. The full
-- per-replacement record — including the payload that was overwritten — is the
-- replace_entity_request audit_logs row, which this column does not duplicate.
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
