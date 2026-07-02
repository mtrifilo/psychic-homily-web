// PSY-1307: RG-MBID-keyed release link enrichment — fill bandcamp/spotify rows in
// release_external_links from MusicBrainz release-level url-rels, fill-when-empty
// per platform.
//
// Identity chain (no name search anywhere): artist MBID (PSY-1249/1289,
// homonym-guarded) → release-group MBID (PSY-1281/1282, browse-by-artist-MBID) →
// release url-rels (browse-by-RG-MBID here). Auto-apply is therefore consistent
// with the PSY-1279 decision: the persisted MBID chain is the identity signal, and
// URLs are host-anchored (pipeline.ClassifyReleasePlatformURL).
//
// MusicBrainz hangs streaming/purchase links on RELEASES, not release-groups
// (spiked 2026-07-01: RG-level url-rels 0/50, release-level 35/50 ≈ 70% coverage),
// so the lookup browses releases by RG-MBID — one call per distinct RG.
//
// Writes go through ReleaseService.AddExternalLink, which also back-fills credited
// artists' NULL bandcamp_embed_url in the same transaction (PSY-1189) — release
// link enrichment compounds into artist embeds without extra code here.
package enrich

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/utils"
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
	Limit  int // 0 = all candidate releases
}

// ReleaseLinkFill records one link that would be / was written.
type ReleaseLinkFill struct {
	ReleaseID    uint
	ReleaseTitle string
	Platform     string // "bandcamp" | "spotify"
	URL          string
}

// ReleaseLinksReport is the structured outcome of a run.
type ReleaseLinksReport struct {
	ReleasesScanned int // candidate releases (have RG-MBID, missing ≥1 platform)
	RGsBrowsed      int // distinct release-groups fetched from MusicBrainz
	FilledBandcamp  int
	FilledSpotify   int
	ReleasesNoLinks int // browsed, but no usable url-rel for the missing platforms
	Errors          []string
	Fills           []ReleaseLinkFill
}

// releaseLinkCandidate is one release needing links, with its per-platform state.
type releaseLinkCandidate struct {
	release      catalogm.Release
	hasBandcamp  bool
	hasSpotify   bool
}

// releaseLinkStore abstracts candidate load for the backfill (fakeable in unit
// tests; the gorm implementation is the production path).
type releaseLinkStore interface {
	ReleaseLinkCandidates(limit int) ([]releaseLinkCandidate, error)
}

// BackfillReleaseLinks fills missing bandcamp/spotify external links for releases
// with a release-group MBID, dry-run by default. One MusicBrainz browse per
// distinct release-group; releases sharing an RG share the result.
func BackfillReleaseLinks(db *gorm.DB, mb MBReleaseURLRelBrowse, writer releaseLinkWriter, opts ReleaseLinksOptions) (*ReleaseLinksReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return backfillReleaseLinks(&gormReleaseLinkStore{db: db}, mb, writer, opts)
}

func backfillReleaseLinks(store releaseLinkStore, mb MBReleaseURLRelBrowse, writer releaseLinkWriter, opts ReleaseLinksOptions) (*ReleaseLinksReport, error) {
	if mb == nil || (!opts.DryRun && writer == nil) {
		return nil, fmt.Errorf("musicbrainz browser and (for live runs) link writer are required")
	}

	candidates, err := store.ReleaseLinkCandidates(opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("load candidate releases: %w", err)
	}

	report := &ReleaseLinksReport{ReleasesScanned: len(candidates)}
	ctx := context.Background()

	// One browse per distinct RG-MBID; nil marks a failed fetch so siblings of a
	// failed RG don't re-browse (and don't count as "no links").
	relsByRG := make(map[string][]pipeline.MBReleaseResult)
	failedRG := make(map[string]bool)

	for i := range candidates {
		cand := &candidates[i]
		rgMBID := stringValue(cand.release.MusicBrainzReleaseGroupID)

		rels, ok := relsByRG[rgMBID]
		if !ok {
			if failedRG[rgMBID] {
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

		filled := false
		if !cand.hasBandcamp {
			if u, found := pickReleaseURL(rels, contracts.MusicPlatformBandcamp); found {
				filled = true
				if applyReleaseLink(report, writer, cand, contracts.MusicPlatformBandcamp, u, opts.DryRun) {
					report.FilledBandcamp++
				}
			}
		}
		if !cand.hasSpotify {
			if u, found := pickReleaseURL(rels, contracts.MusicPlatformSpotify); found {
				filled = true
				if applyReleaseLink(report, writer, cand, contracts.MusicPlatformSpotify, u, opts.DryRun) {
					report.FilledSpotify++
				}
			}
		}
		if !filled {
			report.ReleasesNoLinks++
		}
	}
	return report, nil
}

// applyReleaseLink records the fill (and writes it on live runs). Returns true
// when the fill counts toward the report (dry-run always; live only on success).
func applyReleaseLink(report *ReleaseLinksReport, writer releaseLinkWriter, cand *releaseLinkCandidate, platform, u string, dryRun bool) bool {
	if !dryRun {
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
// browsed releases. Deterministic preference order:
//
//  1. Official releases before any other status (bootlegs/promos carry junk).
//  2. Within a status tier, /album/ URLs before /track/ (the album page is the
//     embeddable unit; a lone track link is the fallback).
//  3. MusicBrainz result order breaks remaining ties (stable across runs for the
//     same data).
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
		if a.official != b.official {
			return a.official
		}
		if a.album != b.album {
			return a.album
		}
		return false // earlier result wins ties
	}
	for i := range releases {
		rel := &releases[i]
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
				album:    strings.Contains(normalized, "/album/"),
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
// missing a bandcamp or spotify external link; id-ordered, limit <= 0 = all.
// Existing links are preloaded so the fill-when-empty check is per-platform
// (release_external_links has NO unique constraint — application-level dedup is
// the only dedup).
func (s *gormReleaseLinkStore) ReleaseLinkCandidates(limit int) ([]releaseLinkCandidate, error) {
	var releases []catalogm.Release
	q := s.db.
		Preload("ExternalLinks").
		Where("musicbrainz_release_group_id IS NOT NULL AND TRIM(musicbrainz_release_group_id) <> ''").
		Order("id")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&releases).Error; err != nil {
		return nil, err
	}

	candidates := make([]releaseLinkCandidate, 0, len(releases))
	for i := range releases {
		r := releases[i]
		if !utils.IsValidMBID(stringValue(r.MusicBrainzReleaseGroupID)) {
			continue // malformed stored key — never browse on it
		}
		cand := releaseLinkCandidate{release: r}
		for _, l := range r.ExternalLinks {
			switch l.Platform {
			case contracts.MusicPlatformBandcamp:
				cand.hasBandcamp = true
			case contracts.MusicPlatformSpotify:
				cand.hasSpotify = true
			}
		}
		if cand.hasBandcamp && cand.hasSpotify {
			continue // nothing to fill
		}
		candidates = append(candidates, cand)
	}
	return candidates, nil
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
