-- PSY-350: Collection subscription digest notifications.
--
-- 1. Add last_digest_sent_at to collection_subscribers — per-subscriber cursor
--    used by the daily digest job to find collection items added since the
--    user was last notified. Null = "no digest sent yet"; the first cycle
--    treats null as "look back to the subscription's created_at" so users
--    don't miss items added between subscribing and the first cycle.
--
-- 2. Add notify_on_collection_digest to user_preferences so users can opt out
--    of the daily digest independently of show_reminders, mention, and
--    comment-subscription emails. Defaults to TRUE — same convention as the
--    other notification opt-outs (PSY-289).
--
-- last_visited_at on collection_subscribers already exists (migration 000047)
-- and is used to compute "N new since last visit" badges in the library tab.

ALTER TABLE collection_subscribers
    ADD COLUMN last_digest_sent_at TIMESTAMPTZ;

ALTER TABLE user_preferences
    ADD COLUMN notify_on_collection_digest BOOLEAN NOT NULL DEFAULT TRUE;
