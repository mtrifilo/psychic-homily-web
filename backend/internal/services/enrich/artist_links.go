// PSY-1279: MBID-keyed artist link enrichment — fill spotify, bandcamp, and website
// fill-when-empty from MusicBrainz url-rels (one lookup per artist, no name re-search).
//
// Design decision (ticket open question, resolved 2026-06-30): AUTO-APPLY via the
// existing ArtistService.UpdateArtist path — NOT the artist_link_suggestions queue.
// The persisted MBID (PSY-1249, exact-name-gated at write sites) is the identity
// signal; url-rels are host-anchored (pipeline.ClassifyPlatformURL + official-homepage
// gate for website). The name-search discovery flow (DiscoverMusicService +
// sweep-link-suggestions cmd) remains the human-reviewed path for MBID-less artists;
// this sweep is the durable backstop for artists that already have an MBID.
package enrich

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/utils"
)

const websiteURLMaxLen = 500

// MBURLRelLookup fetches url-relations for a known MusicBrainz artist MBID.
// Satisfied by *pipeline.MusicBrainzClient.
type MBURLRelLookup interface {
	LookupArtistURLRelations(ctx context.Context, mbid string) ([]pipeline.MBURLRelation, error)
}

// linksArtistStore abstracts candidate load + memo stamp for the links backfill.
type linksArtistStore interface {
	ArtistsNeedingLinks(limit int, reattemptCutoff *time.Time) ([]catalogm.Artist, error)
	StampLinksAttempted(ids []uint, at time.Time) error
}

// linksWriter applies link fills through the validated artist update path (PSY-1190
// Bandcamp profile→embed resolver runs inside UpdateArtist).
type linksWriter interface {
	UpdateArtist(artistID uint, req *contracts.UpdateArtistRequest) (*contracts.ArtistDetailResponse, error)
}

// LinksOptions configures one links backfill run.
type LinksOptions struct {
	DryRun bool
	Limit  int // 0 = all candidates
	// ReattemptWindow turns on the no-result memo for the background sweep (PSY-1279).
	// When > 0, only artists with links_enrich_attempted_at NULL or older than
	// (now - window) are selected; the batch is stamped up front (unless DryRun).
	ReattemptWindow time.Duration
}

// LinksFill records one field that would be / was written.
type LinksFill struct {
	ArtistID uint
	Field    string // "spotify" | "bandcamp" | "website"
	URL      string
}

// LinksReport is the structured outcome of a run.
type LinksReport struct {
	ArtistsScanned    int
	FilledSpotify     int
	FilledBandcamp    int
	FilledWebsite     int
	ArtistsNoLinks    int // browsed, but no fillable url-rel for empty fields
	Errors            []string
	Fills             []LinksFill // populated in dry-run and live runs
}

// BackfillArtistLinks fills missing spotify/bandcamp/website from MB url-rels for
// MBID-bearing artists, dry-run by default.
func BackfillArtistLinks(db *gorm.DB, mb MBURLRelLookup, writer linksWriter, opts LinksOptions) (*LinksReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return backfillArtistLinks(context.Background(), &gormArtistStore{db: db}, mb, writer, opts)
}

func backfillArtistLinks(
	ctx context.Context,
	store linksArtistStore,
	mb MBURLRelLookup,
	writer linksWriter,
	opts LinksOptions,
) (*LinksReport, error) {
	now := time.Now()

	var cutoff *time.Time
	if opts.ReattemptWindow > 0 {
		c := now.Add(-opts.ReattemptWindow)
		cutoff = &c
	}

	artists, err := store.ArtistsNeedingLinks(opts.Limit, cutoff)
	if err != nil {
		return nil, fmt.Errorf("load artists needing links: %w", err)
	}
	report := &LinksReport{ArtistsScanned: len(artists)}

	// Drop invalid stored MBIDs before the memo stamp — a malformed value must not
	// consume a re-attempt window without a MusicBrainz call (poison-row safety
	// applies only to artists we actually browse).
	batch := make([]catalogm.Artist, 0, len(artists))
	for i := range artists {
		a := artists[i]
		if !pipeline.IsValidMBID(trimPtr(a.MusicBrainzArtistID)) {
			report.Errors = append(report.Errors,
				fmt.Sprintf("artist %d %q: invalid stored MBID %q", a.ID, a.Name, trimPtr(a.MusicBrainzArtistID)))
			continue
		}
		batch = append(batch, a)
	}

	if opts.ReattemptWindow > 0 && !opts.DryRun && len(batch) > 0 {
		ids := make([]uint, len(batch))
		for i := range batch {
			ids[i] = batch[i].ID
		}
		if err := store.StampLinksAttempted(ids, now); err != nil {
			return nil, fmt.Errorf("stamp links attempted: %w", err)
		}
	}

	for i := range batch {
		if ctx.Err() != nil {
			break
		}
		a := &batch[i]
		mbid := trimPtr(a.MusicBrainzArtistID)

		rels, err := mb.LookupArtistURLRelations(ctx, mbid)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("artist %d %q url-rels: %v", a.ID, a.Name, err))
			continue
		}

		req := linksUpdateFromRels(a, rels)
		if req == nil {
			report.ArtistsNoLinks++
			continue
		}

		if opts.DryRun {
			recordLinksFills(report, a.ID, req)
			continue
		}

		if _, err := writer.UpdateArtist(a.ID, req); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("artist %d %q update: %v", a.ID, a.Name, err))
			continue
		}
		recordLinksFills(report, a.ID, req)
	}
	return report, nil
}

// linksUpdateFromRels builds an UpdateArtistRequest that fills only empty link
// fields from MB url-rels. Returns nil when nothing would change.
func linksUpdateFromRels(a *catalogm.Artist, rels []pipeline.MBURLRelation) *contracts.UpdateArtistRequest {
	var req contracts.UpdateArtistRequest
	needSpotify := isEmptyPtr(a.Social.Spotify)
	needBandcamp := isEmptyPtr(a.Social.Bandcamp)
	needWebsite := isEmptyPtr(a.Social.Website)

	for _, rel := range rels {
		raw := strings.TrimSpace(rel.URL.Resource)
		if raw == "" {
			continue
		}
		if platform, normalized, ok := pipeline.ClassifyPlatformURL(raw); ok {
			switch platform {
			case contracts.MusicPlatformSpotify:
				if needSpotify {
					req.Spotify = &normalized
					needSpotify = false
				}
			case contracts.MusicPlatformBandcamp:
				if needBandcamp {
					req.Bandcamp = &normalized
					needBandcamp = false
				}
			}
			continue
		}
		if needWebsite {
			if site, ok := classifyOfficialHomepage(rel); ok {
				req.Website = &site
				needWebsite = false
			}
		}
	}

	if req.Spotify == nil && req.Bandcamp == nil && req.Website == nil {
		return nil
	}
	return &req
}

// classifyOfficialHomepage accepts only MB "official homepage" url-rels whose host
// is not a music-platform link (those belong in spotify/bandcamp fields).
func classifyOfficialHomepage(rel pipeline.MBURLRelation) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(rel.Type))
	if !strings.Contains(t, "official") || !strings.Contains(t, "homepage") {
		return "", false
	}
	raw := strings.TrimSpace(rel.URL.Resource)
	if raw == "" || len(raw) > websiteURLMaxLen {
		return "", false
	}
	if err := utils.ValidateHTTPURL(raw, "Website URL"); err != nil {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "open.spotify.com" || utils.IsBandcampArtistHost(host) {
		return "", false
	}
	if _, _, ok := pipeline.ClassifyPlatformURL(raw); ok {
		return "", false
	}
	path := strings.TrimRight(u.Path, "/")
	return "https://" + host + path, true
}

func recordLinksFills(report *LinksReport, artistID uint, req *contracts.UpdateArtistRequest) {
	if req.Spotify != nil {
		report.FilledSpotify++
		report.Fills = append(report.Fills, LinksFill{ArtistID: artistID, Field: "spotify", URL: *req.Spotify})
	}
	if req.Bandcamp != nil {
		report.FilledBandcamp++
		report.Fills = append(report.Fills, LinksFill{ArtistID: artistID, Field: "bandcamp", URL: *req.Bandcamp})
	}
	if req.Website != nil {
		report.FilledWebsite++
		report.Fills = append(report.Fills, LinksFill{ArtistID: artistID, Field: "website", URL: *req.Website})
	}
}
