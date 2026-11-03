-- Artist new-release alerts: a weekly roundup, ONE per user per week covering
-- every followed artist, so the batch is keyed on the recipient rather than on
-- an entity.
--
-- artist_release_alert_batch: which releases belong to which week.
--
-- Accrual is per (artist, release), the shape of the observation; followers are
-- resolved once per week by the flush, never at accrual time.
--
-- PRIMARY KEY (artist_id, release_id) with no alert_week in the key: a pair
-- accrues once, ever, so an enrichment re-run, link backfill or edition
-- correction can never produce a second row. alert_week is a column fixed at
-- first observation: the Monday of the ISO week in UTC. UTC because a release
-- has no location, so there is no local clock to honor.
--
-- created_at lets the flush tell a settled week from one still accruing and
-- bounds the poison-pill retry. dispatched_at is per row so a release accrued
-- after its week was delivered can be folded into the existing inbox row; it is
-- bookkeeping, and uq_notification_log_artist_release_digest (below) is what
-- prevents a second email.
--
-- Both foreign keys CASCADE: a deleted release leaves its week, and a delivered
-- roundup names the records that remain.
--
-- NOT PRUNED: the inbox row for a delivered week reads these rows live, so
-- deleting them would blank inbox history. Growth is bounded on the write side
-- instead: accrual records a release only when it is announceable and the
-- credited artist has a follower.

-- The foreign keys lock artists and releases while the table is created, and
-- the index and CHECK below lock notification_log. Bounded so a manual run
-- outside the boot runner cannot queue every write to those tables behind it.
SET LOCAL lock_timeout = '5s';

CREATE TABLE artist_release_alert_batch (
    artist_id     BIGINT      NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
    release_id    BIGINT      NOT NULL REFERENCES releases (id) ON DELETE CASCADE,
    alert_week    DATE        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at TIMESTAMPTZ,
    PRIMARY KEY (artist_id, release_id)
);

-- The flush poller's only query pattern: find the weeks with undispatched work,
-- oldest first. Partial so the index holds only work in flight against a table
-- that is never pruned.
CREATE INDEX ix_artist_release_alert_batch_pending
    ON artist_release_alert_batch (alert_week, created_at)
    WHERE dispatched_at IS NULL;

-- The flush loads a whole week and the inbox enriches by week, so the pending
-- index above does not serve a read of an ALREADY-dispatched week. This one does.
CREATE INDEX ix_artist_release_alert_batch_week
    ON artist_release_alert_batch (alert_week);

-- An artist merge re-points artist_id; artist_id leads the primary key, so that
-- lookup is already covered. release_id is not, and nothing else indexes it.
CREATE INDEX ix_artist_release_alert_batch_release
    ON artist_release_alert_batch (release_id);

-- notification_log: the per-user weekly roundup row.
--
-- entity_type = 'artist_release_digest' and entity_id holds the reader's USER
-- id, because a roundup has no single artist or release to name: a literal 0
-- would look resolvable to every polymorphic join, and an arbitrary artist id
-- would be re-pointed by an artist merge. The polymorphic merge loop matches only
-- the merged entity's own entity_type, so the user id is never rewritten.
-- subject_entity_id stays NULL. This entity_id is NOT a show or artist id: the
-- type must never join showAlertEntityTypes or artistSubjectAlertTypes.

-- Re-applying after a rollback can find digest rows (kept by the down
-- migration as inbox history) with a NULL bucket. Backfilled rather than
-- deleted, matching 20260824030000: truncating sent_at to its UTC Monday
-- recovers the bucket for every row the flush wrote on schedule.
UPDATE notification_log
   SET alert_bucket = (date_trunc('week', sent_at AT TIME ZONE 'UTC'))::date
 WHERE entity_type = 'artist_release_digest' AND alert_bucket IS NULL;

-- The identity key: at most one row per (user, week, channel). entity_id equals
-- user_id here, so it is redundant, but the four columns match
-- uq_notification_log_venue_show_alert so the claim helper has one shape. The
-- in-app row is the bell entry; the email row is what the per-user daily
-- alert-email budget counts.
--
-- This index is the whole exactly-once guarantee. The flush re-resolves an
-- already-delivered week whenever a late release joins it, so the inbox row can
-- grow; ON CONFLICT DO NOTHING turns that re-run, and a concurrent tick, into a
-- no-op instead of a second send.
--
-- Not CONCURRENTLY (illegal inside golang-migrate's transaction), so it blocks
-- notification_log writes while it builds; the partial predicate matches zero
-- rows at deploy time, so the build is trivial.
CREATE UNIQUE INDEX uq_notification_log_artist_release_digest
    ON notification_log (user_id, entity_id, alert_bucket, channel)
    WHERE entity_type = 'artist_release_digest';

-- alert_bucket MUST be present on digest rows: NULLs are distinct inside a
-- UNIQUE index, so a NULL-bucket row would collide with nothing and the flush
-- would re-send. ck_notification_log_venue_alert_bucket does not cover this
-- discriminator. NOT VALID skips a scan the UPDATE above already made empty;
-- every INSERT and UPDATE is still checked.
ALTER TABLE notification_log
    ADD CONSTRAINT ck_notification_log_release_digest_bucket
    CHECK (entity_type <> 'artist_release_digest' OR alert_bucket IS NOT NULL)
    NOT VALID;
