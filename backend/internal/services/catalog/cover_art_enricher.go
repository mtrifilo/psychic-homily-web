package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Release cover-art enrichment from MusicBrainz/Cover Art Archive + Discogs
// (PSY-1216). Populates release covers for entities that have none, storing only a
// REFERENCE to the externally-hosted image (the URL + provider id + linkback) —
// never the bytes (PSY-1175 architecture D1/D3).
//
// This is the bulk cover-art path that complements the Spotify enricher
// (spotify_image_enricher.go): Spotify's client-credentials rate limit is a poor
// fit for a large pass, so the primary bulk source is purpose-built music metadata
// — the Cover Art Archive (a MusicBrainz project, gentlest + most rule-aligned),
// with Discogs as the secondary searchable provider. Provider order is CAA first,
// then Discogs.
//
// Match policy is the same "strict match only, skip the rest" gate the Spotify
// enricher uses — a candidate is stored only when its normalized title + artist
// match (and year agrees within one when both are known), and when several
// distinct covers qualify the release is skipped + logged rather than guessing.
// Every stored URL is validated before persisting (https image; provider-host
// linkback) per the repo's validate-on-write contract (PSY-525/747/1113).
//
// Artist photos are out of scope here: the Cover Art Archive serves release covers
// only, and MusicBrainz/Discogs are not artist-photo sources. Artist photos stay
// on the Spotify path / a future Phase-3 source.

const (
	coverArtSourceCAA     = "cover_art_archive"
	coverArtSourceDiscogs = "discogs"
	// coverArtYearTolerance mirrors the Spotify enricher: one year absorbs reissue /
	// regional release-date drift without admitting an unrelated re-recording.
	coverArtYearTolerance = 1
	// coverArtSearchLimit is how many candidates each provider search asks for; the
	// strict gate filters them down (we never store the top hit blindly).
	coverArtSearchLimit = 10
)

// MBReleaseGroupCandidate is the catalog-local view of a MusicBrainz release-group
// the matcher works against, so this package need not depend on the pipeline
// package that owns the MusicBrainz transport. The cmd adapts the shared
// pipeline.MusicBrainzClient to the musicBrainzReleaseSearcher interface below.
type MBReleaseGroupCandidate struct {
	MBID             string
	Title            string
	ArtistNames      []string // credited + canonical names; the gate matches any
	FirstReleaseDate string   // "YYYY" | "YYYY-MM" | "YYYY-MM-DD" | "" (year parsed by the matcher)
}

// musicBrainzReleaseSearcher searches MusicBrainz release-groups by artist+title.
// The pipeline MusicBrainz client satisfies this via a thin cmd-side adapter.
type musicBrainzReleaseSearcher interface {
	SearchReleaseGroups(ctx context.Context, artist, title string, limit int) ([]MBReleaseGroupCandidate, error)
}

// coverArtArchiveAPI resolves a front-cover reference for a release-group MBID.
type coverArtArchiveAPI interface {
	FrontCover(ctx context.Context, releaseGroupMBID string) (*CoverArtResult, error)
}

// discogsReleaseSearcher searches Discogs for release covers by artist+title.
type discogsReleaseSearcher interface {
	SearchReleaseCovers(ctx context.Context, artist, title string, limit int) ([]DiscogsRelease, error)
}

// CoverArtEnrichOptions configures a backfill run.
type CoverArtEnrichOptions struct {
	DryRun bool // when true, search + report matches but write nothing
	Limit  int  // max releases to process (0 = no limit)
}

// CoverArtEnrichReport summarizes a backfill run.
//   - Scanned: cover-less releases loaded by the query.
//   - MatchedCAA / MatchedDiscogs: a storable cover passed the strict gate + URL
//     validation, from that provider.
//   - Updated: written (always 0 on a dry run).
//   - Skipped: no usable/unambiguous match from any provider, or a validation fail.
//   - Errors: API/DB failures; the release is left untouched and is safe to retry
//     on a re-run (the run is idempotent).
type CoverArtEnrichReport struct {
	DryRun bool

	ReleasesScanned        int
	ReleasesMatchedCAA     int
	ReleasesMatchedDiscogs int
	ReleasesUpdated        int
	ReleasesSkipped        int
	ReleaseErrors          int
}

// BackfillCoverArt enriches release covers that are currently empty, trying the
// Cover Art Archive first and Discogs second. It stores only a reference (URL +
// provider id + linkback), never bytes (PSY-1175 D1/D3). Idempotent: only
// cover-less releases are considered, so a re-run after a live pass reports zero
// updates, and an errored release is safe to retry by simply re-running.
//
// discogs may be nil — when no Discogs token is configured the run is CAA-only.
func BackfillCoverArt(
	ctx context.Context,
	db *gorm.DB,
	mb musicBrainzReleaseSearcher,
	caa coverArtArchiveAPI,
	discogs discogsReleaseSearcher,
	opts CoverArtEnrichOptions,
) (*CoverArtEnrichReport, error) {
	report := &CoverArtEnrichReport{DryRun: opts.DryRun}

	var releases []catalogm.Release
	q := db.WithContext(ctx).
		Preload("Artists").
		Where("cover_art_url IS NULL OR cover_art_url = ''").
		Order("id")
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	if err := q.Find(&releases).Error; err != nil {
		return report, fmt.Errorf("loading releases: %w", err)
	}

	for i := range releases {
		rel := &releases[i]
		report.ReleasesScanned++

		// primaryArtistForMatch (spotify_image_enricher.go) returns the first artist
		// with a usable name; the Spotify id it also returns is unused here.
		artistName, _ := primaryArtistForMatch(rel.Artists)
		if artistName == "" || strings.TrimSpace(rel.Title) == "" {
			report.ReleasesSkipped++
			slog.Debug("cover-art-enrich: release missing artist or title; skipping",
				"release_id", rel.ID, "title", rel.Title)
			continue
		}

		ref, err := resolveCover(ctx, mb, caa, discogs, rel, artistName)
		if err != nil {
			report.ReleaseErrors++
			slog.Warn("cover-art-enrich: provider lookup failed",
				"release_id", rel.ID, "title", rel.Title, "artist", artistName, "error", err)
			continue
		}
		if ref == nil {
			report.ReleasesSkipped++
			slog.Info("cover-art-enrich: no cover from any provider; skipping",
				"release_id", rel.ID, "title", rel.Title, "artist", artistName)
			continue
		}

		// Validate-on-write (PSY-525/747/1113): cover_art_url renders in <img src>
		// and cover_art_source_url as an attribution <a href>, so reject a non-https
		// image or an off-host linkback rather than trusting the provider blindly.
		if !isHTTPSURL(ref.ImageURL) || !validCoverSourceURL(ref.Source, ref.SourceURL) {
			report.ReleasesSkipped++
			slog.Warn("cover-art-enrich: matched cover failed URL validation; skipping",
				"release_id", rel.ID, "source", ref.Source, "image_url", ref.ImageURL, "source_url", ref.SourceURL)
			continue
		}

		switch ref.Source {
		case coverArtSourceCAA:
			report.ReleasesMatchedCAA++
		case coverArtSourceDiscogs:
			report.ReleasesMatchedDiscogs++
		}

		slog.Info("cover-art-enrich: release cover match",
			"release_id", rel.ID, "title", rel.Title, "artist", artistName,
			"source", ref.Source, "image_url", ref.ImageURL, "source_url", ref.SourceURL,
			"dry_run", opts.DryRun)

		if opts.DryRun {
			continue
		}

		updates := map[string]any{
			"cover_art_url":        ref.ImageURL,
			"cover_art_source":     ref.Source,
			"cover_art_source_url": ref.SourceURL,
		}
		if err := db.WithContext(ctx).Model(&catalogm.Release{}).Where("id = ?", rel.ID).Updates(updates).Error; err != nil {
			report.ReleaseErrors++
			slog.Warn("cover-art-enrich: failed to update release", "release_id", rel.ID, "error", err)
			continue
		}
		report.ReleasesUpdated++
	}

	return report, nil
}

// coverRef is a resolved, storable cover: the image URL, the attribution linkback,
// and which provider it came from.
type coverRef struct {
	ImageURL  string
	SourceURL string
	Source    string // coverArtSourceCAA | coverArtSourceDiscogs
}

// resolveCover tries the Cover Art Archive first (MusicBrainz search → strict
// release-group match → CAA front cover), then Discogs, returning the first cover
// that passes the strict gate. Returns (nil, nil) when no provider yields a
// confident cover. A provider TRANSPORT error aborts resolution for this release
// (returned as err so the caller counts it + retries on a later run); a provider
// merely having no match is not an error.
func resolveCover(
	ctx context.Context,
	mb musicBrainzReleaseSearcher,
	caa coverArtArchiveAPI,
	discogs discogsReleaseSearcher,
	rel *catalogm.Release,
	artistName string,
) (*coverRef, error) {
	// 1. Cover Art Archive (via a MusicBrainz release-group search).
	cands, err := mb.SearchReleaseGroups(ctx, artistName, rel.Title, coverArtSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz search: %w", err)
	}
	mbid, qualifying := pickStrictReleaseGroup(cands, artistName, rel.Title, rel.ReleaseYear)
	if mbid == "" && qualifying > 1 {
		slog.Info("cover-art-enrich: ambiguous musicbrainz match (multiple distinct release-groups); skipping CAA",
			"release_id", rel.ID, "title", rel.Title, "artist", artistName, "qualifying", qualifying)
	}
	if mbid != "" {
		cover, err := caa.FrontCover(ctx, mbid)
		if err != nil {
			return nil, fmt.Errorf("cover art archive: %w", err)
		}
		if cover != nil {
			return &coverRef{ImageURL: cover.ImageURL, SourceURL: cover.SourceURL, Source: coverArtSourceCAA}, nil
		}
		// Matched a release-group but the Archive has no front cover for it — fall
		// through to Discogs rather than giving up.
	}

	// 2. Discogs (secondary searchable provider; nil when no token configured).
	if discogs == nil {
		return nil, nil
	}
	dcands, err := discogs.SearchReleaseCovers(ctx, artistName, rel.Title, coverArtSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("discogs search: %w", err)
	}
	if d := pickStrictDiscogs(dcands, artistName, rel.Title, rel.ReleaseYear); d != nil {
		return &coverRef{ImageURL: d.imageURL, SourceURL: d.sourceURL, Source: coverArtSourceDiscogs}, nil
	}
	return nil, nil
}

// =============================================================================
// Strict-match policy (pure functions — unit-tested without HTTP or DB)
// =============================================================================

// coverCandidate is a provider-agnostic qualifying candidate. key is the identity
// used to detect "really the same cover" (release-group MBID for CAA; cover-image
// URL for Discogs); year disambiguates same-name distinct releases; imageURL /
// sourceURL are populated for providers that carry the cover directly (Discogs),
// and empty for the CAA path, where the MBID is resolved to a cover afterwards.
type coverCandidate struct {
	key       string
	year      int
	imageURL  string
	sourceURL string
}

// pickStrictReleaseGroup applies the strict-match-skip-ambiguous policy to
// MusicBrainz release-group candidates and returns (mbid, qualifyingCount). mbid
// is "" when nothing qualifies OR the choice is ambiguous; the caller uses the
// count to log which case it was.
//
// A candidate qualifies only when its normalized title equals the release title
// AND one of its artist names matches the (normalized) release artist; catalog
// releases have no MusicBrainz artist id to anchor on, so this is a name match.
// When both years are known they must agree within coverArtYearTolerance.
func pickStrictReleaseGroup(cands []MBReleaseGroupCandidate, artistName, title string, releaseYear *int) (string, int) {
	wantTitle := normalizeForMatch(title)
	wantArtist := normalizeForMatch(artistName)
	if wantTitle == "" || wantArtist == "" {
		return "", 0
	}

	var qualifying []coverCandidate
	for _, c := range cands {
		if strings.TrimSpace(c.MBID) == "" {
			continue
		}
		if normalizeForMatch(c.Title) != wantTitle {
			continue
		}
		if !artistNamesContain(c.ArtistNames, wantArtist) {
			continue
		}
		candYear := parseReleaseYear(c.FirstReleaseDate) // reused from radio provider; 0 when unknown
		if releaseYear != nil && *releaseYear > 0 && candYear > 0 && absInt(candYear-*releaseYear) > coverArtYearTolerance {
			continue
		}
		qualifying = append(qualifying, coverCandidate{key: c.MBID, year: candYear})
	}

	chosen := chooseUnambiguousCover(qualifying, releaseYear)
	if chosen == nil {
		return "", len(qualifying)
	}
	return chosen.key, len(qualifying)
}

// pickStrictDiscogs applies the strict-match-skip-ambiguous policy to Discogs
// release candidates, returning the single chosen cover or nil.
//
// Discogs search results carry a combined "Artist - Title" string (the two are not
// split), so the gate is containment: the candidate's normalized title must
// contain BOTH the normalized release title and the normalized artist as
// whole-token phrases — looser than the exact-equality gate the Spotify/MB paths
// use, which is acceptable for a fallback because the server-side artist /
// release_title filters already narrow the search and the ambiguity skip still
// rejects multiple distinct covers.
func pickStrictDiscogs(cands []DiscogsRelease, artistName, title string, releaseYear *int) *coverCandidate {
	wantTitle := normalizeForMatch(title)
	wantArtist := normalizeForMatch(artistName)
	if wantTitle == "" || wantArtist == "" {
		return nil
	}

	var qualifying []coverCandidate
	for _, d := range cands {
		if strings.TrimSpace(d.CoverImage) == "" {
			continue
		}
		if !discogsTitleContains(d.Title, wantTitle, wantArtist) {
			continue
		}
		if releaseYear != nil && *releaseYear > 0 && d.Year > 0 && absInt(d.Year-*releaseYear) > coverArtYearTolerance {
			continue
		}
		qualifying = append(qualifying, coverCandidate{
			key:       d.CoverImage,
			year:      d.Year,
			imageURL:  d.CoverImage,
			sourceURL: d.SourceURL,
		})
	}

	return chooseUnambiguousCover(qualifying, releaseYear)
}

// chooseUnambiguousCover returns the single cover among qualifying candidates, or
// nil when the choice is genuinely ambiguous. One candidate → it. Several that
// share a key (same release-group / same cover image) → that one. Several with
// DIFFERENT keys → narrow to an exact release-year match; if that still doesn't
// resolve to one key, nil (skip + log). Mirrors the Spotify enricher's
// chooseUnambiguous so the "never store the top hit blindly" policy is uniform.
func chooseUnambiguousCover(qualifying []coverCandidate, releaseYear *int) *coverCandidate {
	if len(qualifying) == 0 {
		return nil
	}
	if allSameCoverKey(qualifying) {
		return &qualifying[0]
	}
	if releaseYear != nil && *releaseYear > 0 {
		var exact []coverCandidate
		for i := range qualifying {
			if qualifying[i].year == *releaseYear {
				exact = append(exact, qualifying[i])
			}
		}
		if allSameCoverKey(exact) {
			return &exact[0]
		}
	}
	return nil
}

// allSameCoverKey reports whether every candidate shares the same key — so there
// is really only one cover to choose regardless of how many rows matched. An empty
// slice is not "same key".
func allSameCoverKey(cs []coverCandidate) bool {
	if len(cs) == 0 {
		return false
	}
	for i := 1; i < len(cs); i++ {
		if cs[i].key != cs[0].key {
			return false
		}
	}
	return true
}

// artistNamesContain reports whether any of the names normalizes-equal the wanted
// (already-normalized) artist name.
func artistNamesContain(names []string, wantNormalized string) bool {
	for _, n := range names {
		if normalizeForMatch(n) == wantNormalized {
			return true
		}
	}
	return false
}

// discogsTitleContains reports whether a Discogs "Artist - Title" string, once
// normalized, contains both wanted phrases as whole tokens. Space-padding both
// sides makes the substring check token-boundary-aware, so "war" doesn't match
// inside "warpaint".
func discogsTitleContains(candTitle, wantTitle, wantArtist string) bool {
	padded := " " + normalizeForMatch(candTitle) + " "
	return strings.Contains(padded, " "+wantTitle+" ") && strings.Contains(padded, " "+wantArtist+" ")
}

// validCoverSourceURL gates the *_source_url linkback per provider before
// persisting (it renders as an attribution <a href>): a CAA cover links back to
// MusicBrainz; a Discogs cover links back to the Discogs release page.
func validCoverSourceURL(source, raw string) bool {
	switch source {
	case coverArtSourceCAA:
		return isMusicBrainzWebURL(raw)
	case coverArtSourceDiscogs:
		return isDiscogsWebURL(raw)
	default:
		return false
	}
}

// isMusicBrainzWebURL reports whether raw is an https musicbrainz.org URL.
func isMusicBrainzWebURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "https" && strings.ToLower(u.Hostname()) == "musicbrainz.org"
}

// isDiscogsWebURL reports whether raw is an https discogs.com URL (apex or www).
func isDiscogsWebURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && (host == "www.discogs.com" || host == "discogs.com")
}
