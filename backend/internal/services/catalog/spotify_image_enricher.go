package catalog

import (
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Image enrichment from Spotify (PSY-1185). Populates release cover art and
// artist photos for entities that have none, storing only a REFERENCE to the
// Spotify-hosted image (the URL + provider id + deep linkback) — never the bytes
// (PSY-1175 architecture D1/D3). Labels stay plain-text in Phase 1 (D6), so this
// enriches releases + artists only.
//
// Match policy = "strict match only, skip the rest" (the PSY-1185 decision):
//   - Release covers come from a SEARCH, so a candidate is stored only when its
//     normalized album name AND a normalized artist name match the release, and
//     (when both years are known) the years agree within one. Ambiguous hits are
//     skipped + logged for the deferred candidate-picker — never blindly stored.
//   - Artist photos come from dereferencing the operator-curated Spotify link
//     (an exact id), so they are stored directly; the resolved Spotify name is
//     logged so a mis-pasted link is auditable.

const (
	spotifyImageSourceID = "spotify"
	// spotifyYearTolerance is how far a candidate album's year may sit from the
	// stored release year and still qualify — one year absorbs reissue / regional
	// release-date drift without admitting an unrelated re-recording.
	spotifyYearTolerance = 1
)

// spotifyImageAPI is the slice of SpotifyClient the enricher depends on, so the
// backfill loop can be tested with a mock instead of a live HTTP server.
type spotifyImageAPI interface {
	SearchAlbums(artist, title string, limit int) ([]SpotifyAlbum, error)
	GetArtist(spotifyID string) (*SpotifyArtist, error)
}

// SpotifyEnrichOptions configures a backfill run.
type SpotifyEnrichOptions struct {
	DryRun bool // when true, search Spotify + report matches but write nothing
	Limit  int  // max entities PER TYPE to process (0 = no limit)
}

// SpotifyEnrichReport summarizes a backfill run. Scanned = considered; Matched =
// passed the strict gate; Updated = written (always 0 on a dry run); Skipped =
// no usable strict match; Errors = API/DB failures (entity left untouched).
type SpotifyEnrichReport struct {
	DryRun bool

	ReleasesScanned int
	ReleasesMatched int
	ReleasesUpdated int
	ReleasesSkipped int
	ReleaseErrors   int

	ArtistsScanned int
	ArtistsMatched int
	ArtistsUpdated int
	ArtistsSkipped int
	ArtistErrors   int
}

// BackfillSpotifyImages enriches release covers and artist photos that are
// currently empty. It is idempotent: only entities missing an image are
// considered, so a re-run after a live pass reports zero updates.
func BackfillSpotifyImages(db *gorm.DB, client spotifyImageAPI, opts SpotifyEnrichOptions) (*SpotifyEnrichReport, error) {
	report := &SpotifyEnrichReport{DryRun: opts.DryRun}

	if err := enrichReleaseCovers(db, client, opts, report); err != nil {
		return report, fmt.Errorf("enriching release covers: %w", err)
	}
	if err := enrichArtistPhotos(db, client, opts, report); err != nil {
		return report, fmt.Errorf("enriching artist photos: %w", err)
	}
	return report, nil
}

// enrichReleaseCovers walks releases with no cover art and stores a Spotify cover
// when a search yields a strict match.
func enrichReleaseCovers(db *gorm.DB, client spotifyImageAPI, opts SpotifyEnrichOptions, report *SpotifyEnrichReport) error {
	var releases []catalogm.Release
	q := db.
		Preload("Artists").
		Where("cover_art_url IS NULL OR cover_art_url = ''").
		Order("id")
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	if err := q.Find(&releases).Error; err != nil {
		return fmt.Errorf("loading releases: %w", err)
	}

	for i := range releases {
		rel := &releases[i]
		report.ReleasesScanned++

		artistName := primaryArtistName(rel.Artists)
		if artistName == "" || strings.TrimSpace(rel.Title) == "" {
			report.ReleasesSkipped++
			slog.Debug("spotify-enrich: release missing artist or title; skipping",
				"release_id", rel.ID, "title", rel.Title)
			continue
		}

		albums, err := client.SearchAlbums(artistName, rel.Title, spotifyDefaultSearchLimit)
		if err != nil {
			report.ReleaseErrors++
			slog.Warn("spotify-enrich: album search failed",
				"release_id", rel.ID, "title", rel.Title, "artist", artistName, "error", err)
			continue
		}

		match := pickStrictAlbumMatch(albums, artistName, rel.Title, rel.ReleaseYear)
		if match == nil {
			report.ReleasesSkipped++
			slog.Info("spotify-enrich: no strict album match; skipping",
				"release_id", rel.ID, "title", rel.Title, "artist", artistName, "candidates", len(albums))
			continue
		}
		report.ReleasesMatched++

		slog.Info("spotify-enrich: release cover match",
			"release_id", rel.ID, "title", rel.Title, "artist", artistName,
			"spotify_album", match.Name, "matched_year", match.Year,
			"image_url", match.ImageURL, "source_url", match.SourceURL, "dry_run", opts.DryRun)

		if opts.DryRun {
			continue
		}

		updates := map[string]any{
			"cover_art_url":        match.ImageURL,
			"cover_art_source":     spotifyImageSourceID,
			"cover_art_source_url": match.SourceURL,
		}
		if err := db.Model(&catalogm.Release{}).Where("id = ?", rel.ID).Updates(updates).Error; err != nil {
			report.ReleaseErrors++
			slog.Warn("spotify-enrich: failed to update release", "release_id", rel.ID, "error", err)
			continue
		}
		report.ReleasesUpdated++
	}
	return nil
}

// enrichArtistPhotos walks artists with no image but a Spotify link, and stores
// the artist photo from the linked Spotify artist.
func enrichArtistPhotos(db *gorm.DB, client spotifyImageAPI, opts SpotifyEnrichOptions, report *SpotifyEnrichReport) error {
	var artists []catalogm.Artist
	q := db.
		Where("(image_url IS NULL OR image_url = '') AND spotify IS NOT NULL AND spotify <> ''").
		Order("id")
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	if err := q.Find(&artists).Error; err != nil {
		return fmt.Errorf("loading artists: %w", err)
	}

	for i := range artists {
		ar := &artists[i]
		report.ArtistsScanned++

		spotifyURL := ""
		if ar.Social.Spotify != nil {
			spotifyURL = *ar.Social.Spotify
		}
		spotifyID := extractSpotifyArtistID(spotifyURL)
		if spotifyID == "" {
			report.ArtistsSkipped++
			slog.Info("spotify-enrich: artist spotify link not parseable; skipping",
				"artist_id", ar.ID, "name", ar.Name, "spotify", spotifyURL)
			continue
		}

		sa, err := client.GetArtist(spotifyID)
		if err != nil {
			report.ArtistErrors++
			slog.Warn("spotify-enrich: artist fetch failed",
				"artist_id", ar.ID, "name", ar.Name, "spotify_id", spotifyID, "error", err)
			continue
		}

		imageURL := bestImageURL(sa.Images)
		if imageURL == "" {
			report.ArtistsSkipped++
			slog.Info("spotify-enrich: spotify artist has no image; skipping",
				"artist_id", ar.ID, "name", ar.Name, "spotify_id", spotifyID)
			continue
		}
		report.ArtistsMatched++

		sourceURL := sa.ExternalURLs.Spotify
		if sourceURL == "" {
			sourceURL = spotifyURL // fall back to the curated link
		}

		// The id comes from an operator-curated link, so the artist match is exact
		// by construction. Log the resolved Spotify name alongside ours so a name
		// divergence (alias, mis-paste) is auditable without blocking the write.
		slog.Info("spotify-enrich: artist photo match",
			"artist_id", ar.ID, "name", ar.Name, "spotify_name", sa.Name,
			"image_url", imageURL, "source_url", sourceURL, "dry_run", opts.DryRun)

		if opts.DryRun {
			continue
		}

		updates := map[string]any{
			"image_url":        imageURL,
			"image_source":     spotifyImageSourceID,
			"image_source_url": sourceURL,
		}
		if err := db.Model(&catalogm.Artist{}).Where("id = ?", ar.ID).Updates(updates).Error; err != nil {
			report.ArtistErrors++
			slog.Warn("spotify-enrich: failed to update artist", "artist_id", ar.ID, "error", err)
			continue
		}
		report.ArtistsUpdated++
	}
	return nil
}

// =============================================================================
// Strict-match policy (pure functions — unit-tested without HTTP or DB)
// =============================================================================

// spotifyAlbumMatch is a release cover that passed the strict gate.
type spotifyAlbumMatch struct {
	Name      string
	ImageURL  string
	SourceURL string
	Year      int // 0 when unknown
}

// pickStrictAlbumMatch returns the first candidate that satisfies the strict
// gate, or nil when none do (caller skips + logs). A candidate qualifies only
// when its normalized album name equals the release title AND one of its artists'
// normalized names equals the release's primary artist. When BOTH the release and
// the candidate have a known year, they must agree within spotifyYearTolerance; a
// candidate with an unknown year is not rejected (we never reject on missing
// provider data). A qualifying candidate with no usable image is skipped.
func pickStrictAlbumMatch(albums []SpotifyAlbum, artist, title string, releaseYear *int) *spotifyAlbumMatch {
	wantTitle := normalizeForMatch(title)
	wantArtist := normalizeForMatch(artist)
	if wantTitle == "" || wantArtist == "" {
		return nil
	}

	for _, alb := range albums {
		if normalizeForMatch(alb.Name) != wantTitle {
			continue
		}
		if !albumArtistsContain(alb.Artists, wantArtist) {
			continue
		}

		candYear := parseReleaseYear(alb.ReleaseDate) // reused from radio provider; 0 when unknown
		if releaseYear != nil && *releaseYear > 0 && candYear > 0 {
			if absInt(candYear-*releaseYear) > spotifyYearTolerance {
				continue
			}
		}

		img := bestImageURL(alb.Images)
		if img == "" {
			continue
		}

		return &spotifyAlbumMatch{
			Name:      alb.Name,
			ImageURL:  img,
			SourceURL: alb.ExternalURLs.Spotify,
			Year:      candYear,
		}
	}
	return nil
}

// albumArtistsContain reports whether any of the album's artists' normalized
// names equals the wanted (already-normalized) artist name.
func albumArtistsContain(artists []SpotifyAlbumArtistRef, wantNormalized string) bool {
	for _, a := range artists {
		if normalizeForMatch(a.Name) == wantNormalized {
			return true
		}
	}
	return false
}

// primaryArtistName returns the artist name to search a release's cover by. The
// many2many preload does not guarantee role/position order, so we take the first
// loaded non-empty name. Picking the "wrong" artist on a multi-artist release is
// safe, not harmful: the strict gate then finds no match and the release is
// skipped (left for the deferred candidate-picker) rather than mis-tagged.
func primaryArtistName(artists []catalogm.Artist) string {
	for _, a := range artists {
		if strings.TrimSpace(a.Name) != "" {
			return a.Name
		}
	}
	return ""
}

// normalizeForMatch produces the comparison key for the strict album/artist
// match: NFKD-fold + strip diacritics ("Café" → "cafe"), lowercase, then replace
// every run of non-alphanumeric runes with a single space ("AC/DC" → "ac dc";
// "The Velvet Underground & Nico" → "the velvet underground nico"). Used only for
// the strict comparison — the value is never persisted.
//
// This DELIBERATELY diverges from this package's normalizeName (radio_matching.go),
// which PRESERVES interior punctuation so "AC/DC" ≠ "ACDC" — radio feed names are
// punctuation-significant, whereas here we want looser title/artist matching
// against Spotify's catalog. Don't "consolidate" the two. It reuses the shared
// markStripper var but builds the transform.Chain per-call: a Chain keeps mutable
// per-call state and is not goroutine-safe to share (see markStripper's comment).
func normalizeForMatch(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}

	// NFKD-decompose + strip combining marks (diacritics) via the shared stripper.
	normalizer := transform.Chain(norm.NFKD, markStripper, norm.NFC)
	if folded, _, err := transform.String(normalizer, s); err == nil {
		s = folded
	}

	// Replace any run of non-alphanumeric runes with a single space.
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteRune(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// spotifyArtistIDPattern matches the bare id segment of a Spotify artist URL.
var spotifyArtistIDPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// extractSpotifyArtistID parses an open.spotify.com/artist/<id> URL and returns
// the id. Host-anchored (mirrors the backend handler's isValidSpotifyURL) so a
// link on a look-alike host yields "". Handles locale prefixes
// (/intl-de/artist/<id>) and trailing sub-tabs (/artist/<id>/about). Returns ""
// for anything that does not parse to a real https Spotify artist URL.
func extractSpotifyArtistID(rawURL string) string {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	// Require an http(s) URL (parity with the handler's isValidSpotifyURL): a
	// scheme-relative "//open.spotify.com/..." or an ftp:// link is rejected.
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	if strings.ToLower(u.Hostname()) != "open.spotify.com" {
		return ""
	}
	segs := strings.Split(u.Path, "/")
	for i := 0; i+1 < len(segs); i++ {
		if segs[i] == "artist" {
			id := segs[i+1]
			if spotifyArtistIDPattern.MatchString(id) {
				return id
			}
			return ""
		}
	}
	return ""
}

// absInt returns the absolute value of n.
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
