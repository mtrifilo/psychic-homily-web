-- PSY-1316: release-links enrichment becomes ongoing + auditable (Phase A).
--
-- 1) "No-result memo" for the release-LINKS sweep. Without a per-release attempt
--    timestamp, the no-link + single-platform long tail (~30% of RGs carry no
--    streaming url-rel at all) would be re-browsed against MusicBrainz (~1 req/s)
--    every cycle. Mirrors artists.links_enrich_attempted_at (PSY-1279); kept
--    separate per-entity because the sweeps converge independently.
ALTER TABLE releases ADD COLUMN links_enrich_attempted_at TIMESTAMPTZ;

-- Partial index matching the sweep's candidate gate (RG-MBID-bearing releases,
-- ordered by attempt time NULLS FIRST then id). The per-platform NOT EXISTS half
-- of the gate can't live in a partial-index predicate (subqueries are not
-- allowed), so the index narrows on the RG-MBID predicate and the planner
-- filters the rest.
CREATE INDEX idx_releases_links_enrich_pending
    ON releases (links_enrich_attempted_at NULLS FIRST, id)
    WHERE musicbrainz_release_group_id IS NOT NULL
      AND TRIM(musicbrainz_release_group_id) <> '';

-- 2) Provenance for enrichment-written link rows. NULL = manual (admin dialog /
--    create funnel), 'mb_backfill' = written by the MB url-rel backfill/sweep.
--    Without this, a poisoned MusicBrainz edit that gets backfilled is
--    indistinguishable from admin data and can never be mass-reverted
--    (adversarial-review finding on PSY-1307). Mirrors
--    artists.bandcamp_embed_source.
ALTER TABLE release_external_links ADD COLUMN source VARCHAR(50);

-- 3) Close the concurrent-BACKFILL duplicate race at the DB layer — but ONLY for
--    enrichment-sourced rows. PSY-1307's pre-write re-check is check-then-act, so
--    two overlapping live runs could still double-insert. Scoping the unique
--    index to source='mb_backfill' closes the backfill-vs-backfill race (the
--    re-check still narrows the backfill-vs-MANUAL window — keep both) while leaving
--    manual entry unconstrained (an admin may legitimately add two links on the
--    same platform, e.g. a Bandcamp album page and a track page; stage has zero
--    same-platform duplicates today, 2026-07-02).
CREATE UNIQUE INDEX uniq_release_links_backfill_per_platform
    ON release_external_links (release_id, LOWER(platform))
    WHERE source = 'mb_backfill';
