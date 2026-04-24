-- Add editor attribution to comment edit history.
-- PSY-297: admin edit history viewer needs to know *who* edited a comment,
-- not just when and what the prior body was.
--
-- Nullable because the comment_edits table has been live since migration 000064
-- and may contain rows written before this column existed. All new edits
-- written via CommentService.UpdateComment will populate this column.
ALTER TABLE comment_edits
    ADD COLUMN editor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;

-- Index to support admin queries by editor (future-proofing; cheap to add now).
CREATE INDEX idx_comment_edits_editor ON comment_edits(editor_user_id) WHERE editor_user_id IS NOT NULL;
