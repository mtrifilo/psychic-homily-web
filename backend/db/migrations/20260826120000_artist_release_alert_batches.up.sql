-- PSY-1897: artist new-release alerts, the second COALESCED alert loop and the
-- first one whose batch is keyed on the RECIPIENT rather than on an entity.
--
-- The product decision (PSY-1892) is releases WEEKLY, and the owner's grain is
-- ONE roundup per USER per week covering every artist they follow — not one
-- message per artist. A listener who follows forty bands and sees six of them
-- put something out in one week is owed one email, not six.
--
-- ─────────────────────────────────────────────────────────────────────────────
-- artist_release_alert_batch: WHICH RELEASES BELONG TO WHICH WEEK
-- ─────────────────────────────────────────────────────────────────────────────
--
-- Accrual is per (artist, release) because that is the shape of the OBSERVATION:
-- a release becomes visible, credited to an artist, at one instant. The USER
-- dimension does not exist yet at that moment and is deliberately not resolved
-- there — a label ingest that mints two hundred releases would otherwise decide
-- "who should hear about this" two hundred times and then suppress most of the
-- answers. The flush resolves followers ONCE per week, across every artist.
--
-- PRIMARY KEY (artist_id, release_id) — NO alert_week in the key, and that is
-- the whole dedup story for this feature. A pair accrues ONCE, EVER. An
-- enrichment re-run, a cover-art pass, a link backfill, an edition or format
-- correction: none of them can produce a second row, because none of them can
-- produce a second (artist, release) pair. The venue sibling needed a separate
-- NOT EXISTS guard for this because its natural key included the day; keying on
-- the pair alone makes the guard structural instead.
--
-- alert_week is therefore a COLUMN, fixed at first observation, not part of the
-- key. It is the Monday of the ISO week in UTC.
--
-- Why UTC and not a local zone, when the venue sibling is emphatically local: a
-- release has no location. There is no venue clock to be faithful to and no
-- reader whose evening would be cut in half — the artist may be in Oslo and the
-- follower in Phoenix. A WEEK is also 168 hours wide, so a zone offset can move
-- at most one of its 168 hours across the boundary. The venue alert's day is 24
-- hours wide, which is why the same offset mattered there and does not here.
--
-- created_at is per ROW so the flush can tell a settled week from one still
-- accruing, and so the poison-pill bound has something to measure.
--
-- dispatched_at is per ROW, not per week, so a release accrued after the week's
-- roundup already went out can be recorded, folded into the existing inbox row,
-- and marked — without anything having to reopen a closed week. What stops that
-- second flush from sending a second email is not this column: it is
-- uq_notification_log_artist_release_digest below.
--
-- Both foreign keys CASCADE. A deleted release simply leaves its week, and an
-- already-delivered roundup degrades to naming the records that remain rather
-- than disappearing.
--
-- NOT PRUNED, for the same reason venue_show_alert_batch is not: the inbox row
-- for a delivered week renders its release list by reading these rows live, so
-- deleting them would blank inbox history. Growth is bounded on the WRITE side
-- instead — accrual only records a release when the credited artist actually has
-- a follower, and only when the release is announceable.

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

-- ─────────────────────────────────────────────────────────────────────────────
-- notification_log: THE PER-USER WEEKLY ROUNDUP ROW
-- ─────────────────────────────────────────────────────────────────────────────
--
-- entity_type = 'artist_release_digest', and entity_id holds the USER id.
--
-- That is unusual enough to justify at length, because every other discriminator
-- in this table puts a catalogue entity there. A weekly roundup is about the
-- reader's whole follow set: there is no single artist and no single release for
-- entity_id to name, and the moment one is chosen the row starts lying about the
-- other four. The alternatives were considered and are worse:
--
--   * A literal 0 is a magic value. entity_id is NOT NULL and every polymorphic
--     join in services/catalog treats it as a real id, so 0 is a value that
--     looks resolvable and never resolves.
--   * An arbitrary "first" artist id makes the row indistinguishable from a
--     per-artist alert, and would be silently re-pointed by an artist merge into
--     naming an artist the roundup never covered.
--
-- The user id is the one value that is genuinely true of the row, cannot be
-- mistaken for a catalogue id under any existing merge (the merges key on
-- entity_type IN ('artist','venue','show'), which this is not), and makes the
-- unique index below read as what it is: one roundup per reader per week per lane.
--
-- subject_entity_id stays NULL: there is no single followed subject either.
--
-- The consequence that matters at every query site: this entity_id is NOT a show
-- id and NOT an artist id. It must never join showAlertEntityTypes or
-- artistSubjectAlertTypes.

-- Re-applying after a rollback is the one case that can find rows of this
-- discriminator already present with a NULL bucket: the down migration keeps the
-- rows (they are real inbox history a reader received) but drops nothing they
-- depend on. Backfilled rather than deleted, matching 20260824030000 —
-- sent_at::date lands inside the week the flush ran, and truncating it to the
-- Monday recovers the exact bucket for every row the flush wrote on schedule.
UPDATE notification_log
   SET alert_bucket = (date_trunc('week', sent_at AT TIME ZONE 'UTC'))::date
 WHERE entity_type = 'artist_release_digest' AND alert_bucket IS NULL;

-- The identity key: at most one row per (user, week, LANE).
--
-- entity_id is in the index because the shared shape is
-- (user_id, entity_id, alert_bucket, channel) and here entity_id IS user_id, so
-- the column is redundant rather than wrong. Keeping the same four columns as
-- uq_notification_log_venue_show_alert is deliberate: the claim helper, the read
-- predicates and the reviewer all get one shape to learn instead of two.
--
-- channel is in the key because the two lanes are separate jobs. The in-app row
-- is the bell entry; the email row is what the per-user daily alert-email budget
-- counts.
--
-- This index is the WHOLE exactly-once guarantee for this feature. The flush
-- deliberately re-runs a week it has already delivered — a release accrued late
-- puts an undispatched row back into a closed week, and the week is re-resolved
-- from scratch so the inbox row can GROW. Every such pass reaches the claim, and
-- ON CONFLICT DO NOTHING is what turns the re-run into a no-op instead of a
-- second email. It is also what makes two concurrent ticks safe: the loser
-- claims nothing and therefore sends nothing.
--
-- LOCK NOTE: a plain CREATE UNIQUE INDEX, not CONCURRENTLY (illegal inside the
-- transaction golang-migrate wraps a multi-statement file in), so it blocks
-- writes to notification_log while it builds. Acceptable because it is partial
-- on a discriminator with zero rows at deploy time, so the build is trivial
-- regardless of the table's size — the same shape 20260824010000 and
-- 20260824030000 already shipped.
CREATE UNIQUE INDEX uq_notification_log_artist_release_digest
    ON notification_log (user_id, entity_id, alert_bucket, channel)
    WHERE entity_type = 'artist_release_digest';

-- alert_bucket MUST be present on those rows, and this is not a stylistic
-- assertion — it is what keeps the index above from being inert.
--
-- NULLs compare DISTINCT inside a UNIQUE index, so a digest row written with a
-- NULL bucket would collide with nothing and the poller would re-send on every
-- flush. The existing ck_notification_log_venue_alert_bucket does NOT cover this
-- discriminator; a separate CHECK is required, not optional.
--
-- NOT VALID: enforced on every INSERT and UPDATE from the moment it lands, and
-- skips a validation scan that the UPDATE above already guarantees is empty.
ALTER TABLE notification_log
    ADD CONSTRAINT ck_notification_log_release_digest_bucket
    CHECK (entity_type <> 'artist_release_digest' OR alert_bucket IS NOT NULL)
    NOT VALID;
