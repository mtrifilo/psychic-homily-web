-- PSY-1896: artist new-show alerts land in notification_log as their own
-- discriminator, and need two things the table did not have.
--
-- ─────────────────────────────────────────────────────────────────────────────
-- subject_entity_id: WHICH SUBSCRIPTION PRODUCED THIS ROW
-- ─────────────────────────────────────────────────────────────────────────────
--
-- (entity_type, entity_id) already answers "what is this notification ABOUT" —
-- for an artist show alert that is the show. It cannot also answer "which thing
-- the user follows caused it", and the row is unreadable without that: the
-- inbox line is "<artist> announced a show", and the email says "you follow
-- <artist>". Deriving the artist at read time instead would mean intersecting
-- the show's bill with the user's LIVE follows, so unfollowing the band would
-- retroactively blank the label on a notification they already received.
--
-- The subject's TYPE is implied by entity_type (artist_show_alert => artist)
-- rather than stored beside it, because a discriminator that can disagree with
-- its own row is a bug waiting to be written. NULL on every pre-existing row and
-- on every row from the filter, scene-follow, comment and request writers, none
-- of which have a followed subject.
--
-- ─────────────────────────────────────────────────────────────────────────────
-- uq_notification_log_artist_show_alert: ONE ALERT PER FOLLOWER PER SHOW
-- ─────────────────────────────────────────────────────────────────────────────
--
-- The table's existing UNIQUE (user_id, filter_id, entity_type, entity_id,
-- channel) is inert for follow-driven rows: filter_id is NULL for them, and
-- NULLs compare DISTINCT in a UNIQUE, so it permits unlimited duplicates. That
-- is a known live hazard — the scene-follow writer works around it with a
-- Count-then-Create that races, and the show-notify outbox's own doc names the
-- missing partial index as the prerequisite for running a second replica.
--
-- This index closes it for the new discriminator, which matters here more than
-- elsewhere because an alert is triggered by an outbox row that CAN be
-- re-processed (a reclaimed `processing` row, a redelivered job). Re-running the
-- match must be a database-level no-op, not a convention the caller remembers.
--
-- channel is in the key, not excluded from it, because the two channels are
-- separate lanes with separate records: the in-app row is what the bell renders,
-- and the email row is what the per-user daily email budget counts. One row
-- could not do both jobs without lying about one of them. At most one row per
-- lane, so at most one bell entry and at most one email, per follower per show.
--
-- Partial rather than whole-table so it constrains ONLY this discriminator and
-- cannot reject a write on any pre-existing path (in particular the duplicate
-- scene-follow rows the race above can already have produced in production).
--
-- ADDITIVE: one nullable column and one partial index. Multi-statement file =>
-- golang-migrate wraps it in a transaction => no CREATE INDEX CONCURRENTLY
-- (illegal in a txn). The index is partial on a discriminator that has zero rows
-- at deploy time, so the build is trivial regardless of table size.

ALTER TABLE notification_log
    ADD COLUMN subject_entity_id BIGINT;

CREATE UNIQUE INDEX uq_notification_log_artist_show_alert
    ON notification_log (user_id, entity_id, channel)
    WHERE entity_type = 'artist_show_alert';
