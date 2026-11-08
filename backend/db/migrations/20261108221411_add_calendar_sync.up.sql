-- Persist each user's calendar sync choice.
--
-- calendar_sync is a nullable JSONB document recording what the user decided
-- the first time they were asked to put saved shows on a calendar:
--
--   {"version": 1, "state": "connected",
--    "providers": ["google_subscribe", "apple_subscribe"],
--    "decided_at": "2026-11-08T18:30:00Z"}
--   {"version": 1, "state": "not_now",
--    "decided_at": "2026-11-08T18:30:00Z",
--    "declined_until": "2026-12-08T18:30:00Z", "declines": 1}
--   {"version": 1, "state": "never", "decided_at": "2026-11-08T18:30:00Z"}
--
-- NULL means NOT DECIDED, the only state in which the client asks. A write
-- replaces the whole document. The version, the state and provider
-- vocabularies and the per-state field rules are enforced in Go
-- (models/auth/calendar_sync.go), not by a CHECK constraint, for the same
-- reason as home_layout's: the vocabulary moves with the frontend.
ALTER TABLE user_preferences
    ADD COLUMN calendar_sync JSONB;
