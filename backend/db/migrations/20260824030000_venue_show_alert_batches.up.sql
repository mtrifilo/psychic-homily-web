-- PSY-1895: venue new-show alerts, which are COALESCED where every other alert
-- in this system is per-event.
--
-- A venue publishes its calendar in drops. One ingest run of a season of dates
-- is fifty shows at one venue inside a few minutes, and the per-show shape the
-- artist alerts use (PSY-1896) would send a follower fifty notifications. The
-- product decision (PSY-1892) is ONE alert per user per venue per venue-local
-- calendar day, so this file adds the two things that shape needs: somewhere to
-- record which shows belong to a day's batch, and a uniqueness key on
-- notification_log that is per (user, venue, DAY) rather than per (user, show).
--
-- ─────────────────────────────────────────────────────────────────────────────
-- venue_show_alert_batch: WHICH SHOWS BELONG TO WHICH DAY'S ALERT
-- ─────────────────────────────────────────────────────────────────────────────
--
-- There is no batch ENTITY and deliberately no surrogate id. A batch IS the set
-- of rows sharing (venue_id, alert_day); the primary key is the whole natural
-- key. That choice is what keeps a venue merge and a show dedup ordinary: both
-- are re-points of an id column against a unique key, exactly the shape the
-- merge inventory in services/catalog already knows how to perform. A surrogate
-- id would have added a second identity for the merges to reconcile without
-- making anything else easier.
--
-- alert_day is the day the show was ANNOUNCED, in the VENUE's local zone, not
-- the day it takes place. Announcement day is what "one alert per day" means:
-- a single ingest drop covering nine months of dates is one announcement, and
-- keying on event date would scatter it across nine months of batches. Venue
-- local rather than UTC because the alternative splits an ordinary US evening
-- in half: 5pm in Phoenix is already tomorrow in UTC, so an evening ingest
-- would produce two alerts for what the venue and the reader both experienced
-- as one drop.
--
-- created_at is per ROW because the flush poller waits for a quiet window
-- (no new accrual for N minutes) before dispatching, so it needs to know when
-- the LAST show landed, not when the batch opened.
--
-- dispatched_at is per ROW rather than per batch, so a show announced after the
-- batch already went out can be recorded, folded into the existing alert, and
-- marked without anything having to reopen a closed batch. What stops that
-- second flush from sending a second email is not this column: it is
-- uq_notification_log_venue_show_alert below.
--
-- Both foreign keys CASCADE, matching show_venues. A deleted show simply leaves
-- its batch, and an already-delivered alert degrades to naming the shows that
-- remain rather than disappearing.
--
-- NOT PRUNED. The inbox row for a delivered batch renders its show list by
-- reading these rows live (that is what lets a late show grow the row), so
-- deleting them would blank inbox history. Growth is bounded on the WRITE side
-- instead: accrual only records a show when the venue actually has a follower.

CREATE TABLE venue_show_alert_batch (
    venue_id      BIGINT      NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    alert_day     DATE        NOT NULL,
    show_id       BIGINT      NOT NULL REFERENCES shows (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at TIMESTAMPTZ,
    PRIMARY KEY (venue_id, alert_day, show_id)
);

-- The flush poller's only query pattern: find the undispatched groups. Partial
-- so the index holds only work in flight, which is a handful of rows against a
-- table that is never pruned.
CREATE INDEX ix_venue_show_alert_batch_pending
    ON venue_show_alert_batch (venue_id, alert_day, created_at)
    WHERE dispatched_at IS NULL;

-- Show dedup and venue merge both need to find a show's or a venue's rows to
-- re-point them. venue_id leads the primary key, so a venue lookup is already
-- covered; show_id is not.
CREATE INDEX ix_venue_show_alert_batch_show ON venue_show_alert_batch (show_id);

-- ─────────────────────────────────────────────────────────────────────────────
-- notification_log.alert_bucket: WHICH DAY'S ALERT THIS ROW IS
-- ─────────────────────────────────────────────────────────────────────────────
--
-- A venue show alert's (entity_type, entity_id) is ('venue_show_alert', VENUE
-- id) — the row is about the venue, and the follow that produced it is the
-- follow OF that venue, which is why subject_entity_id stays NULL here (unlike
-- PSY-1896, where the subject is an artist and the entity is a show).
--
-- That leaves no room for the day, and the day is half of this alert's
-- identity: the second alert a user gets for the same venue tomorrow is a
-- different notification, not a duplicate of today's. alert_bucket carries it.
--
-- NULL on every other writer's rows. Per-event alerts have no bucket, and a
-- column that meant "today" by default would make the two shapes indistinguishable.

ALTER TABLE notification_log
    ADD COLUMN alert_bucket DATE;

-- The identity key: at most one row per (user, venue, day, LANE).
--
-- channel is IN the key for the same reason it is in the artist alert's index:
-- the two channels are separate lanes with separate jobs. The in-app row is the
-- bell entry; the email row is what the per-user daily alert-email budget
-- counts. At most one row per lane means at most one bell entry and at most one
-- email per follower per venue per day.
--
-- This index is the WHOLE exactly-once guarantee for this feature, and it is
-- load-bearing in a way the artist alert's is not. Venue alerts are dispatched
-- by a poller that deliberately runs the same batch more than once: a show
-- announced after the batch was delivered puts an undispatched row back into
-- an already-delivered group, and the flush re-resolves that group from
-- scratch. Every re-run therefore reaches the claim, and ON CONFLICT DO NOTHING
-- is what turns the re-run into a no-op instead of a second email.
--
-- Partial on the discriminator so it constrains only these rows and can never
-- reject a write on a pre-existing path.
CREATE UNIQUE INDEX uq_notification_log_venue_show_alert
    ON notification_log (user_id, entity_id, alert_bucket, channel)
    WHERE entity_type = 'venue_show_alert';

-- alert_bucket MUST be present on those rows, and this is not a stylistic
-- assertion — it is what keeps the index above from being inert.
--
-- NULLs compare DISTINCT inside a UNIQUE index, so a venue_show_alert row
-- written with a NULL bucket would collide with nothing, and the poller would
-- re-send on every single flush. That failure mode is not hypothetical in this
-- schema: notification_log's original UNIQUE (user_id, filter_id, entity_type,
-- entity_id, channel) is already inert for every follow-driven row precisely
-- because filter_id is NULL on all of them, and the scene-follow writer carries
-- a racing Count-then-Create to work around it.
--
-- NOT VALID: the constraint is enforced on every INSERT and UPDATE from the
-- moment it lands, and skips the validation scan of the existing table. There
-- is nothing for that scan to find (no row has this entity_type yet), and
-- skipping it means this migration does not take a lock proportional to the
-- size of notification_log.
ALTER TABLE notification_log
    ADD CONSTRAINT ck_notification_log_venue_alert_bucket
    CHECK (entity_type <> 'venue_show_alert' OR alert_bucket IS NOT NULL)
    NOT VALID;
