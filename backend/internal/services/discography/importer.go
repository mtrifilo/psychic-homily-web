// Package discography imports an artist's primary discography from MusicBrainz
// (PSY-1282). Given an artist with a persisted MBID (PSY-1249), it browses their
// release-GROUPS by MBID (identity-verified — these are THIS artist's, not a
// homonym's), keeps the PRIMARY types (album + EP) only, and creates one release row
// per release-group, deduped on the release-group MBID (the PSY-1281 keystone), with
// cover art fetched directly from the Cover Art Archive by that same MBID.
//
// It lives in its own package because it needs BOTH pipeline (the browse) and catalog
// (FindOrCreateReleaseByReleaseGroupMBID + the Cover Art Archive client) — pipeline
// imports catalog, so neither of those packages can host it. The network clients are
// consumer-defined interfaces so tests can fake them; the DB create/dedup runs against
// a real (testcontainer) database.
package discography

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/pipeline"
)

// discographyPrimaryTypes is the auto-import allowlist (PSY-1252 decision): primary
// types only. Singles + secondary types (compilation/live/remix/dj-mix/…) are the
// highest flood risk, so we import the curated core discography. Keys are lowercased
// to match the browse filter's normalization.
var discographyPrimaryTypes = map[string]bool{"album": true, "ep": true}

// ReleaseGroupBrowser fetches an artist's release-groups by MBID (filtered to the
// given primary types). Satisfied by *pipeline.MusicBrainzClient.
type ReleaseGroupBrowser interface {
	BrowseArtistReleaseGroups(ctx context.Context, mbid string, primaryTypes map[string]bool) ([]pipeline.MBReleaseGroupResult, error)
}

// CoverArtFetcher fetches a release-group's front cover by MBID. Satisfied by
// *catalog.CoverArtArchiveClient (already MBID-keyed — no title search).
type CoverArtFetcher interface {
	FrontCover(ctx context.Context, releaseGroupMBID string) (*catalog.CoverArtResult, error)
}

// Options controls one backfill run.
type Options struct {
	DryRun bool
	Limit  int // 0 = all artists with a stored MBID
	// ReattemptWindow turns on the sync memo for the background sweep (PSY-1291). When > 0,
	// only artists with discography_synced_at NULL or older than (now - window) are selected,
	// ordered stalest-first; the batch is stamped up front (unless DryRun). Leave zero for the
	// manual cmd (memo-agnostic, id-ordered selection).
	ReattemptWindow time.Duration
}

// Plan records one would-import release-group for the dry-run review.
type Plan struct {
	ArtistID   uint
	ArtistName string
	RGMBID     string
	Title      string
	Type       string
	Year       *int
	Action     string // "create" | "dedup"
}

// Report is the structured outcome of a run.
type Report struct {
	ArtistsScanned    int
	ReleaseGroupsSeen int
	Created           int
	Deduped           int // release-group already present (by RG-MBID or title match)
	CoverArtSet       int
	ArtistsNoReleases int // browsed, but no primary-type release-group
	Errors            []string
	Plans             []Plan
}

// BackfillArtistDiscography imports primary-type discography for every artist with a
// stored MBID. Dry-run by default (writes nothing; the Report's Plans show what a live
// run would do). The MB browse + Cover Art Archive calls go through their clients'
// shared ~1 req/s / gentle throttles.
func BackfillArtistDiscography(db *gorm.DB, browser ReleaseGroupBrowser, coverart CoverArtFetcher, opts Options) (*Report, error) {
	return backfillArtistDiscography(context.Background(), db, browser, coverart, opts)
}

func backfillArtistDiscography(ctx context.Context, db *gorm.DB, browser ReleaseGroupBrowser, coverart CoverArtFetcher, opts Options) (*Report, error) {
	now := time.Now()

	var cutoff *time.Time
	if opts.ReattemptWindow > 0 {
		c := now.Add(-opts.ReattemptWindow)
		cutoff = &c
	}

	artists, err := loadArtistsWithMBID(db, opts.Limit, cutoff)
	if err != nil {
		return nil, fmt.Errorf("load artists with MBID: %w", err)
	}
	report := &Report{ArtistsScanned: len(artists)}

	// Stamp-before-resolve (sweep mode only, never in a dry run): mark the whole batch synced
	// up front so a poison row that errors still waits one re-attempt window before a retry and
	// can't wedge the sweep (mirrors the location + image sweeps, PSY-1250/1246).
	if opts.ReattemptWindow > 0 && !opts.DryRun && len(artists) > 0 {
		ids := make([]uint, len(artists))
		for i := range artists {
			ids[i] = artists[i].ID
		}
		if err := stampDiscographySynced(db, ids, now); err != nil {
			return nil, fmt.Errorf("stamp discography synced: %w", err)
		}
	}

	for i := range artists {
		// Honor cancellation (server shutdown) between artists — a browse is ~1s under
		// the MB throttle. The manual cmd uses a Background ctx.
		if ctx.Err() != nil {
			break
		}
		a := &artists[i]
		mbid := derefString(a.MusicBrainzArtistID)

		rgs, err := browser.BrowseArtistReleaseGroups(ctx, mbid, discographyPrimaryTypes)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("artist %d %q browse: %v", a.ID, a.Name, err))
			continue
		}
		if len(rgs) == 0 {
			report.ArtistsNoReleases++
			continue
		}

		for j := range rgs {
			rg := rgs[j]
			// Primary CORE only (PSY-1252). The browse already filtered to primary type
			// album/EP, but that is NOT sufficient: MusicBrainz tags live albums /
			// compilations / soundtracks / remix albums / DJ-mixes with primary type
			// "Album" (or "EP") PLUS a secondary type. Skip any release-group carrying a
			// secondary type — exactly the flood-risk content this curation gate exists
			// to exclude.
			if len(rg.SecondaryTypes) > 0 {
				continue
			}
			// Trust boundary: the RG-MBID is the release dedup key + the cover-art key,
			// so reject a malformed one rather than poison either (mirrors the artist
			// MBID gate). A browse result without a valid id is an upstream anomaly.
			if !pipeline.IsValidMBID(rg.ID) {
				continue
			}
			report.ReleaseGroupsSeen++

			year := yearFromDate(rg.FirstReleaseDate)
			req := &contracts.CreateReleaseRequest{
				Title:       rg.Title,
				ReleaseType: string(releaseTypeFor(rg.PrimaryType)),
				ReleaseYear: year,
				Artists:     []contracts.CreateReleaseArtistEntry{{ArtistID: a.ID, Role: string(catalogm.ArtistReleaseRoleMain)}},
			}

			if opts.DryRun {
				action := "create"
				if exists, _ := releaseGroupExists(db, rg.ID); exists {
					action = "dedup"
				}
				report.Plans = append(report.Plans, planFor(a, rg, year, action))
				continue
			}

			rel, created, err := catalog.FindOrCreateReleaseByReleaseGroupMBID(db, rg.ID, req)
			if err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("artist %d %q rg %s create: %v", a.ID, a.Name, rg.ID, err))
				continue
			}
			if !created {
				report.Deduped++
				report.Plans = append(report.Plans, planFor(a, rg, year, "dedup"))
				continue
			}
			report.Created++
			report.Plans = append(report.Plans, planFor(a, rg, year, "create"))

			// Cover art on the freshly-created release only, keyed on the RG-MBID we
			// already hold (no title search). Written through catalog's shared,
			// host-anchored validate-on-write boundary (one gate + one provenance value
			// for every cover-art writer). Best-effort: a CAA miss / failed gate / DB
			// error never fails the import — the release stands, and the on-create
			// image-enrich outbox (PSY-1247, enqueued by the create) is the backstop.
			if cover, cerr := coverart.FrontCover(ctx, rg.ID); cerr == nil {
				if set, uerr := catalog.SetReleaseCoverArtFromCAA(db, rel.ID, cover); uerr != nil {
					report.Errors = append(report.Errors, fmt.Sprintf("release %d cover-art write: %v", rel.ID, uerr))
				} else if set {
					report.CoverArtSet++
				}
			}
		}
	}
	return report, nil
}

// loadArtistsWithMBID selects artists that have a stored MBID (the importer's input).
// reattemptCutoff (sweep mode, PSY-1291): when non-nil, also skip artists whose
// discography_synced_at is at-or-after the cutoff and order by sync time (NULLs — never
// synced — first) so brand-new and stalest rows come before recently re-checked ones.
// nil (the manual cmd) keeps the original id-ordered, memo-agnostic selection. limit <= 0 = all.
func loadArtistsWithMBID(db *gorm.DB, limit int, reattemptCutoff *time.Time) ([]catalogm.Artist, error) {
	var artists []catalogm.Artist
	q := db.Where("musicbrainz_artist_id IS NOT NULL AND TRIM(musicbrainz_artist_id) <> ''")
	if reattemptCutoff != nil {
		q = q.
			Where("discography_synced_at IS NULL OR discography_synced_at < ?", *reattemptCutoff).
			Order("discography_synced_at ASC NULLS FIRST").
			Order("id ASC")
	} else {
		q = q.Order("id")
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&artists).Error; err != nil {
		return nil, err
	}
	return artists, nil
}

// stampDiscographySynced sets discography_synced_at = at for the given ids (the sweep's sync
// memo). A no-op on an empty slice. Uses Table (not Model) so this bookkeeping write does NOT
// bump artists.updated_at — the memo marks "we tried", not a content change.
func stampDiscographySynced(db *gorm.DB, ids []uint, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	return db.Table("artists").
		Where("id IN ?", ids).
		Update("discography_synced_at", at).Error
}

// releaseGroupExists reports whether a release already carries this RG-MBID (the
// dry-run preview's create-vs-dedup signal). It only sees the RG-MBID dedup, not the
// importer's artist-anchored title-match fill, so dry-run "create" is an UPPER bound.
func releaseGroupExists(db *gorm.DB, rgMBID string) (bool, error) {
	var n int64
	err := db.Model(&catalogm.Release{}).Where("musicbrainz_release_group_id = ?", rgMBID).Count(&n).Error
	return n > 0, err
}

// releaseTypeFor maps a MusicBrainz primary type to our ReleaseType. Only album/EP
// reach here (the browse filters to those); the default is defensive.
func releaseTypeFor(primaryType string) catalogm.ReleaseType {
	switch strings.ToLower(primaryType) {
	case "ep":
		return catalogm.ReleaseTypeEP
	default: // "album"
		return catalogm.ReleaseTypeLP
	}
}

// yearFromDate extracts a 4-digit year from a MusicBrainz first-release-date
// ("YYYY" | "YYYY-MM" | "YYYY-MM-DD" | ""), or nil when absent/implausible.
func yearFromDate(date string) *int {
	if len(date) < 4 {
		return nil
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil || y < 1900 || y > 2100 {
		return nil
	}
	return &y
}

func planFor(a *catalogm.Artist, rg pipeline.MBReleaseGroupResult, year *int, action string) Plan {
	return Plan{
		ArtistID:   a.ID,
		ArtistName: a.Name,
		RGMBID:     rg.ID,
		Title:      rg.Title,
		Type:       string(releaseTypeFor(rg.PrimaryType)),
		Year:       year,
		Action:     action,
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
