-- PSY-350: Collection subscription digest notifications.
--
-- 1. Add last_digest_sent_at to collection_subscribers — per-subscriber cursor
--    used by the weekly digest job to find collection items added since the
--    user was last notified. Null = "no digest sent yet"; the first cycle
--    treats null as "look back to the subscription's created_at" so users
--    don't miss items added between subscribing and the first cycle.
--
-- 2. Add notify_on_collection_digest to user_preferences so users can opt in
--    to the weekly digest independently of show_reminders, mention, and
--    comment-subscription emails.
--
--    Defaults to FALSE (opt-IN) — diverges from PSY-289 opt-OUT defaults.
--    The collection digest is the only notification that can deliver many
--    emails to a user without any direct interaction by the contributor
--    (subscribing implicitly subscribes to every future addition by anyone).
--    Defaulting to OFF keeps Gmail/Yahoo bulk-sender complaint rates low and
--    forces the toggle UI to be discoverable. Users opt in via the
--    notification-preferences toggle wired to PATCH
--    /auth/preferences/collection-digest.
--
-- last_visited_at on collection_subscribers already exists (migration 000047)
-- and is used to compute "N new since last visit" badges in the library tab.

ALTER TABLE collection_subscribers
    ADD COLUMN last_digest_sent_at TIMESTAMPTZ;

ALTER TABLE user_preferences
    ADD COLUMN notify_on_collection_digest BOOLEAN NOT NULL DEFAULT FALSE;
