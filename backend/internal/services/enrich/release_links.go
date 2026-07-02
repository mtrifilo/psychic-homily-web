// PSY-1307: RG-MBID-keyed release link enrichment — fill bandcamp/spotify rows in
// release_external_links from MusicBrainz release-level url-rels, fill-when-empty
// per platform.
//
// Identity chain (no name search anywhere): artist MBID (PSY-1249/1289,
// homonym-guarded) → release-group MBID (PSY-1281/1282, browse-by-artist-MBID) →
// release url-rels (browse-by-RG-MBID here). Auto-apply is therefore consistent
// with the PSY-1279 decision: the persisted MBID chain is the identity signal, and
// URLs are host-anchored (pipeline.ClassifyReleasePlatformURL). Only Official (or
// status-less) MB releases may source a link — bootleg/promo-only release-groups
// are reported, never written (the "curated core only" posture, PSY-1252).
//
// MusicBrainz hangs streaming/purchase links on RELEASES, not release-groups
// (spiked 2026-07-01: RG-level url-rels 0/50, release-level 35/50 ≈ 70% coverage),
// so the lookup browses releases by RG-MBID — one browse (≤10 paginated calls)
// per distinct RG.
//
// Writes go through ReleaseService.AddExternalLink, which also back-fills credited
// artists' NULL bandcamp_embed_url in the same transaction (PSY-1189) — release
// link enrichment compounds into artist embeds without extra code here. Because a
// full run is hours long and release_external_links has NO unique constraint, the
// live path re-checks link absence immediately before each write (the candidate
// snapshot alone would race an admin adding a link mid-run).
package enrich

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/pipeline"
)

// MBReleaseURLRelBrowse fetches url-relations for every release in a known
// release-group. Satisfied by *pipeline.MusicBrainzClient.
type MBReleaseURLRelBrowse interface {
	BrowseReleaseURLRelations(ctx context.Context, rgMBID string) ([]pipeline.MBReleaseResult, error)
}

// releaseLinkWriter applies one link through the validated release path
// (transactional with the PSY-1189 artist-embed fill). Satisfied by
// *catalog.ReleaseService.
type releaseLinkWriter interface {
	AddExternalLink(releaseID uint, platform, url string) (*contracts.ReleaseExternalLinkResponse, error)
}

// ReleaseLinksOptions configures one release-links backfill run.
type ReleaseLinksOptions struct {
	DryRun bool
	Limit  int // 0 = all candidate releases (counts real candidates, post-filter)
}

// ReleaseLinkFill records one link that would be / was written.
type ReleaseLinkFill struct {
	ReleaseID    uint
	ReleaseTitle string
	Platform     string // "bandcamp" | "spotify"
	URL          string
}

// ReleaseLinksReport is the structured outcome of a run. Every scanned release
// lands in at least one bucket: Fills, ReleasesNoLinks, ReleasesSkippedFailedRG,
// LinksRaced, or Errors (invalid MBID / browse or write failure) — the summary
// reconciles against ReleasesScanned.
type ReleaseLinksReport struct {
	ReleasesScanned         int // candidate releases (have RG-MBID, missing ≥1 platform)
	RGsBrowsed              int // distinct release-groups fetched from MusicBrainz
	FilledBandcamp          int
	FilledSpotify           int
	ReleasesNoLinks         int // browsed, but no usable url-rel for the missing platforms
	ReleasesSkippedFailedRG int // siblings of a release-group whose browse failed
	LinksRaced              int // pre-write re-check found the link already present (filled mid-run by someone else)
	Errors                  []string
	Fills                   []ReleaseLinkFill
}

// releaseLinkCandidate is one release needing links, with its per-platform state.
type releaseLinkCandidate struct {
	release     catalogm.Release
	hasBandcamp bool
	hasSpotify  bool
}

// releaseLinkStore abstracts candidate load + the pre-write re-check for the
// backfill (fakeable in unit tests; the gorm implementation is the production
// path).
type releaseLinkStore interface {
	// ReleaseLinkCandidates returns releases with an RG-MBID that are missing a
	// bandcamp or spotify link. limit caps CANDIDATES (the missing-link filter
	// runs in SQL, so a partially-filled catalog cannot exhaust the window with
	// already-complete rows); limit <= 0 = all.
	ReleaseLinkCandidates(limit int) ([]releaseLinkCandidate, error)
	// ReleaseHasPlatformLink re-checks link presence immediately before a write —
	// the candidate snapshot can be hours stale on a full run, and the table has
	// no unique constraint to catch the race.
	ReleaseHasPlatformLink(releaseID uint, platform string) (bool, error)
}

// BackfillReleaseLinks fills missing bandcamp/spotify external links for releases
// with a release-group MBID, dry-run by default. One MusicBrainz browse (≤10
// paginated calls) per distinct release-group; releases sharing an RG share the
// result.
func BackfillReleaseLinks(db *gorm.DB, mb MBReleaseURLRelBrowse, writer releaseLinkWriter, opts ReleaseLinksOptions) (*ReleaseLinksReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return backfillReleaseLinks(context.Background(), &gormReleaseLinkStore{db: db}, mb, writer, opts)
}

// backfillReleaseLinks checks ctx per candidate (mirroring the artist links
// backfill) so a future sweep caller can interrupt an hours-long MB walk; the
// CLI wrapper passes context.Background().
func backfillReleaseLinks(ctx context.Context, store releaseLinkStore, mb MBReleaseURLRelBrowse, writer releaseLinkWriter, opts ReleaseLinksOptions) (*ReleaseLinksReport, error) {
	if mb == nil || (!opts.DryRun && writer == nil) {
		return nil, fmt.Errorf("musicbrainz browser and (for live runs) link writer are required")
	}

	candidates, err := store.ReleaseLinkCandidates(opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("load candidate releases: %w", err)
	}

	report := &ReleaseLinksReport{ReleasesScanned: len(candidates)}

	// One browse per distinct RG-MBID; failedRG marks a failed fetch so siblings
	// of a failed RG don't re-browse (they're counted as skipped, not "no links").
	relsByRG := make(map[string][]pipeline.MBReleaseResult)
	failedRG := make(map[string]bool)

	for i := range candidates {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		cand := &candidates[i]
		rgMBID := stringValue(cand.release.MusicBrainzReleaseGroupID)

		// The invariant "a malformed stored RG-MBID is never browsed" lives HERE,
		// in the loop that owns the browse — not (only) in a store implementation —
		// and is surfaced as an error: a corrupted key is a data-quality signal,
		// not something to skip silently (mirrors the artist links backfill).
		if !pipeline.IsValidMBID(rgMBID) {
			report.Errors = append(report.Errors,
				fmt.Sprintf("release %d %q: invalid stored RG-MBID %q — skipped", cand.release.ID, cand.release.Title, rgMBID))
			continue
		}

		rels, ok := relsByRG[rgMBID]
		if !ok {
			if failedRG[rgMBID] {
				report.ReleasesSkippedFailedRG++
				continue
			}
			rels, err = mb.BrowseReleaseURLRelations(ctx, rgMBID)
			if err != nil {
				failedRG[rgMBID] = true
				report.Errors = append(report.Errors,
					fmt.Sprintf("release %d %q rg %s browse: %v", cand.release.ID, cand.release.Title, rgMBID, err))
				continue
			}
			relsByRG[rgMBID] = rels
			report.RGsBrowsed++
		}

		// found = a usable url-rel existed for a missing platform (the release
		// does NOT belong in ReleasesNoLinks); whether the write then happened,
		// raced, or errored is accounted separately.
		found := false
		if !cand.hasBandcamp {
			if u, ok := pickReleaseURL(rels, contracts.MusicPlatformBandcamp); ok {
				found = true
				if applyReleaseLink(report, store, writer, cand, contracts.MusicPlatformBandcamp, u, opts.DryRun) {
					report.FilledBandcamp++
				}
			}
		}
		if !cand.hasSpotify {
			if u, ok := pickReleaseURL(rels, contracts.MusicPlatformSpotify); ok {
				found = true
				if applyReleaseLink(report, store, writer, cand, contracts.MusicPlatformSpotify, u, opts.DryRun) {
					report.FilledSpotify++
				}
			}
		}
		if !found {
			report.ReleasesNoLinks++
		}
	}
	return report, nil
}

// applyReleaseLink records the fill (and writes it on live runs). Returns true
// when the fill counts toward the report (dry-run always; live only on success).
// Live writes re-check link absence first: the candidate snapshot can be hours
// stale (an admin adds a link mid-run) and the table has no unique constraint —
// a stale write would create a duplicate platform row. NOTE: this is a
// check-then-act, not atomic with the INSERT — it closes the stale-snapshot
// window, not a fully concurrent second live run (don't run two live instances
// at once; a DB unique index is the PSY-1316 follow-up).
func applyReleaseLink(report *ReleaseLinksReport, store releaseLinkStore, writer releaseLinkWriter, cand *releaseLinkCandidate, platform, u string, dryRun bool) bool {
	if !dryRun {
		exists, err := store.ReleaseHasPlatformLink(cand.release.ID, platform)
		if err != nil {
			report.Errors = append(report.Errors,
				fmt.Sprintf("release %d %q %s pre-write check: %v", cand.release.ID, cand.release.Title, platform, err))
			return false
		}
		if exists {
			report.LinksRaced++
			return false // filled by someone else since the snapshot — not an error
		}
		if _, err := writer.AddExternalLink(cand.release.ID, platform, u); err != nil {
			report.Errors = append(report.Errors,
				fmt.Sprintf("release %d %q %s write: %v", cand.release.ID, cand.release.Title, platform, err))
			return false
		}
	}
	report.Fills = append(report.Fills, ReleaseLinkFill{
		ReleaseID:    cand.release.ID,
		ReleaseTitle: cand.release.Title,
		Platform:     platform,
		URL:          u,
	})
	return true
}

// pickReleaseURL chooses ONE canonical URL for a platform from a release-group's
// browsed releases.
//
// Status FLOOR: only releases MusicBrainz marks "Official" — or with no status
// at all (MB status coverage is spotty; an unset status on an otherwise-normal
// release is common) — may source an auto-applied link. Bootleg / Promotion /
// Withdrawn / Pseudo-Release url-rels are never written, even when they are the
// only option: an unofficial fan-upload page must not become the release's
// canonical embed (and, via the PSY-1189 fill, the artist's site-wide embed).
//
// Within the allowed statuses, deterministic preference:
//
//  1. /album/ URLs before /track/ (the ticket's stated preference — the album
//     page is the embeddable unit; a lone track link is the fallback).
//  2. "Official" before status-less (explicit confirmation beats absence).
//  3. MusicBrainz result order breaks remaining ties (stable across runs).
//
// Ended relations are skipped — MB marks delisted/dead links ended.
func pickReleaseURL(releases []pipeline.MBReleaseResult, platform string) (string, bool) {
	type scored struct {
		url      string
		official bool
		album    bool
	}
	var best *scored
	better := func(a, b *scored) bool {
		if a.album != b.album {
			return a.album
		}
		if a.official != b.official {
			return a.official
		}
		return false // earlier result wins ties
	}
	for i := range releases {
		rel := &releases[i]
		if rel.Status != "Official" && rel.Status != "" {
			continue // status floor: bootleg/promo/withdrawn never source a link
		}
		for j := range rel.Relations {
			r := &rel.Relations[j]
			if r.Ended {
				continue
			}
			p, normalized, ok := pipeline.ClassifyReleasePlatformURL(r.URL.Resource)
			if !ok || p != platform {
				continue
			}
			cand := &scored{
				url:      normalized,
				official: rel.Status == "Official",
				album:    pipeline.IsAlbumUnitURL(normalized),
			}
			if best == nil || better(cand, best) {
				best = cand
			}
		}
	}
	if best == nil {
		return "", false
	}
	return best.url, true
}

// gormReleaseLinkStore is the production releaseLinkStore.
type gormReleaseLinkStore struct {
	db *gorm.DB
}

// ReleaseLinkCandidates selects releases with a release-group MBID that are
// missing a bandcamp or spotify external link; id-ordered. The missing-link
// filter runs in SQL (NOT EXISTS per platform) so a limit caps ACTUAL candidates
// — otherwise a partially-filled catalog fills the limit window with
// already-complete rows and the run reports "nothing to do" while eligible
// releases sit past the window. Existing links are also preloaded so the
// per-platform flags are computed from the same snapshot.
func (s *gormReleaseLinkStore) ReleaseLinkCandidates(limit int) ([]releaseLinkCandidate, error) {
	var releases []catalogm.Release
	q := s.db.
		Preload("ExternalLinks").
		Where("musicbrainz_release_group_id IS NOT NULL AND TRIM(musicbrainz_release_group_id) <> ''").
		Where(`(NOT EXISTS (SELECT 1 FROM release_external_links l WHERE l.release_id = releases.id AND LOWER(l.platform) = ?)
		    OR NOT EXISTS (SELECT 1 FROM release_external_links l WHERE l.release_id = releases.id AND LOWER(l.platform) = ?))`,
			contracts.MusicPlatformBandcamp, contracts.MusicPlatformSpotify).
		Order("id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&releases).Error; err != nil {
		return nil, err
	}

	candidates := make([]releaseLinkCandidate, 0, len(releases))
	for i := range releases {
		cand := releaseLinkCandidate{release: releases[i]}
		for _, l := range releases[i].ExternalLinks {
			// Case-insensitive to match the SQL filter — the API accepts platform
			// strings verbatim, so a "Bandcamp" row must still count as present.
			switch strings.ToLower(l.Platform) {
			case contracts.MusicPlatformBandcamp:
				cand.hasBandcamp = true
			case contracts.MusicPlatformSpotify:
				cand.hasSpotify = true
			}
		}
		candidates = append(candidates, cand)
	}
	return candidates, nil
}

// ReleaseHasPlatformLink reports whether the release currently has a link for
// the platform (case-insensitive) — the pre-write TOCTOU re-check.
func (s *gormReleaseLinkStore) ReleaseHasPlatformLink(releaseID uint, platform string) (bool, error) {
	var count int64
	err := s.db.Model(&catalogm.ReleaseExternalLink{}).
		Where("release_id = ? AND LOWER(platform) = ?", releaseID, strings.ToLower(platform)).
		Count(&count).Error
	return count > 0, err
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
