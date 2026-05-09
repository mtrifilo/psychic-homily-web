-- PSY-618: Split entity-edit audit rows into a dedicated table to fix the
-- trusted-user direct-edit dual-render and stats double-count.
--
-- Before: audit_logs carried both moderation events ("approve_show",
-- "verify_venue", "create_artist", ...) AND content-edit events
-- ("edit_artist", "edit_release", ...). The contributor activity feed
-- UNIONed audit_logs with pending_entity_edits, so a trusted-user
-- direct-edit (which writes BOTH a pending_entity_edits row AND an
-- "edit_<type>" audit_log row) rendered twice. The same action also
-- double-counted in stats: ArtistsEdited (audit_log) + RevisionsMade
-- (revisions table).
--
-- After: edit events live in entity_edit_audit_logs. audit_logs holds
-- only moderation + creation events. Stats counters (ArtistsEdited et al)
-- read from the new table. The activity feed query is unchanged in shape;
-- it no longer matches edit rows in audit_logs because they have moved.
--
-- Schema mirrors audit_logs except that entity_type carries the bare type
-- ("artist", "venue", ...) rather than action-prefixed ("edit_artist") —
-- there is only one row class in this table, so the action column is
-- dropped and the entity_type column itself disambiguates.

CREATE TABLE entity_edit_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id INT REFERENCES users(id) ON DELETE SET NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id INT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_entity_edit_audit_logs_created_at ON entity_edit_audit_logs(created_at DESC);
CREATE INDEX idx_entity_edit_audit_logs_entity ON entity_edit_audit_logs(entity_type, entity_id);
CREATE INDEX idx_entity_edit_audit_logs_actor ON entity_edit_audit_logs(actor_id);
