-- Restore radio_play_match_suggestions exactly as created by
-- 20260719000000_create_radio_play_match_suggestions.up.sql (table held zero
-- rows in all environments, so no data is lost by the drop).

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
