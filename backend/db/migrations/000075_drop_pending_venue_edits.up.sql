-- PSY-503: Drop legacy pending_venue_edits table.
-- Superseded by pending_entity_edits (migration 000061), which uses a JSONB
-- field_changes column and covers every entity type (artist, venue, festival,
-- release, label) through a single moderation queue.
--
-- Pending rows in this table are dropped. Any in-flight venue edits not
-- approved/rejected before this migration runs are abandoned — the user can
-- resubmit via PUT /venues/{id}/suggest-edit. Production impact checked
-- before merge (SELECT count(*) FROM pending_venue_edits WHERE status='pending').
--
-- The venues.submitted_by column introduced in migration 000008 is kept —
-- it is still used by the contributor profile activity feed and non-admin
-- ownership checks on venue deletion.

DROP TABLE IF EXISTS pending_venue_edits;
DROP TYPE IF EXISTS venue_edit_status;
