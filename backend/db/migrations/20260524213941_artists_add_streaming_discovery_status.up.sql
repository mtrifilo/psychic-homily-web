-- Track per-artist streaming-discovery review state so the admin worklist can
-- exclude terminal-state rows (linked, no_links_found, skipped) and remember
-- admin decisions across sessions. Without this, a new artist is
-- indistinguishable from one that's already been reviewed and found empty.
--
-- States:
--   unreviewed         — never touched by the discovery worklist (default)
--   candidates_pending — provider lookups produced candidates awaiting review
--   linked             — admin (or backfill) confirmed at least one streaming link
--   no_links_found     — admin reviewed and the artist has no platform presence
--   skipped            — admin opted to defer (ambiguous match, low priority, etc.)
--
-- VARCHAR + CHECK matches the convention established by
-- collections.display_mode rather than the older CREATE TYPE ... AS ENUM
-- pattern (show_status, etc.) — easier to extend, no separate type to manage.
--
-- streaming_discovery_reason holds the admin's optional note on no_links_found
-- and skipped. NULL is normal for the other states.
ALTER TABLE artists
    ADD COLUMN streaming_discovery_status VARCHAR(32) NOT NULL DEFAULT 'unreviewed'
        CHECK (streaming_discovery_status IN (
            'unreviewed',
            'candidates_pending',
            'linked',
            'no_links_found',
            'skipped'
        )),
    ADD COLUMN streaming_discovery_reason TEXT NULL;

-- Worklist filters on status (e.g. WHERE streaming_discovery_status = 'unreviewed').
-- Added inline; the artists table is small enough today that a non-CONCURRENTLY
-- index is fine, and inlining keeps the schema + index atomic for the
-- round-trip test gate.
CREATE INDEX idx_artists_streaming_discovery_status
    ON artists(streaming_discovery_status);
