-- PSY-1199: artist_link_suggestions — the pre-computed, human-reviewed queue
-- that clears the ~1859 link-less artist backlog. A sweep cmd (PSY-1206)
-- populates this table from MusicBrainz-sourced candidates; the admin review
-- API (this ticket) lists/accepts/rejects rows; a triage UI (PSY-1207) drives
-- it. The spikes (PSY-1196/1197) ruled out auto-apply — false matches carry
-- real links — so EVERY candidate is reviewed; nothing here is auto-accepted.
--
-- Column shapes mirror contracts.MusicLinkCandidate (the LOCKED discover-music
-- wire shape) so the sweep can insert a discovered candidate directly:
--   platform   ∈ {bandcamp, spotify}    — MusicLinkCandidate.Platform
--   url                                  — MusicLinkCandidate.URL
--   source     = musicbrainz             — MusicLinkCandidate.Source
--   mb_artist_id / mb_artist_name        — MusicLinkCandidate.MB* fields
--   confidence ∈ {high, review}          — MusicLinkCandidate.Confidence (region tier)
--   region_match / live                  — MusicLinkCandidate.RegionMatch / Live
--   notes                                — MusicLinkCandidate.Notes
--
-- Review state:
--   status     ∈ {pending, accepted, rejected} (default pending)
--   reviewed_at / reviewed_by_user_id   — stamped on accept/reject
--
-- CHECK (not Postgres enums) on the small closed sets so adding a platform or
-- confidence tier later is a cheap ALTER, matching the recent radio/source-config
-- schema choice. UNIQUE (artist_id, platform, url) so a re-sweep that re-discovers
-- the same candidate is a no-op insert (ON CONFLICT DO NOTHING in the sweep) and
-- never resurrects an already-reviewed (accepted/rejected) row.
--
-- artist_id has a real FK (single parent, unlike the polymorphic source_configs)
-- with ON DELETE CASCADE: a deleted artist's stale suggestions are meaningless.
--
-- ADDITIVE: one brand-new table; nothing existing is touched. Multi-statement
-- file => golang-migrate wraps it in a transaction => no CREATE INDEX
-- CONCURRENTLY (illegal in a txn, and unnecessary on an empty new table).

CREATE TABLE artist_link_suggestions (
    id BIGSERIAL PRIMARY KEY,
    artist_id BIGINT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    platform VARCHAR(20) NOT NULL CHECK (platform IN ('bandcamp', 'spotify')),
    url TEXT NOT NULL,
    source VARCHAR(20) NOT NULL DEFAULT 'musicbrainz' CHECK (source IN ('musicbrainz')),
    mb_artist_id TEXT,
    mb_artist_name TEXT,
    confidence VARCHAR(10) NOT NULL CHECK (confidence IN ('high', 'review')),
    region_match BOOLEAN NOT NULL DEFAULT FALSE,
    live BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT,
    status VARCHAR(10) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ,
    reviewed_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (artist_id, platform, url)
);

-- The review-queue list query filters status='pending' and orders
-- high-confidence first (then a stable tiebreak). A partial index over only the
-- pending rows keeps it tight as accepted/rejected rows accumulate. The
-- confidence DESC ordering surfaces 'review' before 'high' alphabetically, so
-- the list query orders by an explicit CASE — this index covers the WHERE filter
-- and the id tiebreak; the small per-page CASE sort is cheap on the bounded set.
CREATE INDEX idx_artist_link_suggestions_pending
    ON artist_link_suggestions (status, confidence, id)
    WHERE status = 'pending';
