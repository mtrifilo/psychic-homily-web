-- PSY-1494: radio_play_match_suggestions — community "suggest a match" queue
-- for unmatched radio plays (Option 2 from PSY-1052). Community submits a
-- pending suggestion; admins accept (LinkPlay, optional BulkLinkPlays) or
-- reject. Nothing is auto-applied; community NEVER hits LinkPlay/BulkLinkPlays.
--
-- Suggestable plays: artist_id IS NULL and match_state in
-- {unmatched, ambiguous, no_match}. Accept overrides matcher outcomes.
--
-- Partial UNIQUE on (submitted_by, play_id) WHERE status='pending' so a user
-- can have at most one pending suggestion per play, but may resubmit after
-- reject (accepted/rejected rows no longer collide).
--
-- ADDITIVE: one brand-new table. Multi-statement => golang-migrate wraps in a
-- transaction => no CREATE INDEX CONCURRENTLY.

CREATE TABLE radio_play_match_suggestions (
    id BIGSERIAL PRIMARY KEY,
    play_id BIGINT NOT NULL REFERENCES radio_plays(id) ON DELETE CASCADE,
    suggested_artist_id BIGINT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    submitted_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note TEXT,
    status VARCHAR(10) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected')),
    reviewed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    rejection_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One pending suggestion per (user, play). Resubmit after reject is allowed.
CREATE UNIQUE INDEX uq_radio_play_match_suggestions_pending_user_play
    ON radio_play_match_suggestions (submitted_by, play_id)
    WHERE status = 'pending';

-- Admin review queue: pending rows newest-first / oldest-first.
CREATE INDEX idx_radio_play_match_suggestions_pending
    ON radio_play_match_suggestions (status, created_at, id)
    WHERE status = 'pending';

-- Lookup "my pending suggestion for this play" (playlist UI state).
CREATE INDEX idx_radio_play_match_suggestions_play_submitter
    ON radio_play_match_suggestions (play_id, submitted_by);
