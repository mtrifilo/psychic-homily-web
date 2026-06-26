package catalog

import (
	"errors"
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
//   - Release covers come from a SEARCH. A candidate is stored only when its
//     normalized album name matches AND its artist matches — by Spotify artist ID
//     when the release's artist has a curated link (immune to two distinct artists
//     sharing a name), otherwise by normalized name. When both years are known
//     they must agree within one. When several candidates carry DIFFERENT covers,
//     the release is skipped + logged as ambiguous for the deferred
//     candidate-picker — Spotify's top hit is never blindly stored.
//   - Artist photos dereference the operator-curated Spotify link (an exact id),
//     so they are stored directly; the resolved Spotify name is logged so a
//     mis-pasted link is auditable.
//
// Every stored URL is validated before persisting (https for image urls;
// open.spotify.com for the *_source_url linkbacks) — these fields render in
// <img src> / <a href>, so this honors the repo's validate-on-write contract
// (PSY-525/747/1113) that the backfill would otherwise bypass.

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

// SpotifyEnrichReport summarizes a backfill run.
//   - Scanned: entities considered (missing an image, loaded by the query).
//   - Matched: a STORABLE match was found — for releases, passed the strict gate +
//     URL validation; for artists, the curated link resolved to a usable photo
//     (artist photos have no strict gate: the operator-curated id is exact).
//   - Updated: written (always 0 on a dry run).
//   - Skipped: no usable/unambiguous match, or a stored-URL validation failure.
//   - Errors: API/DB failures; the entity is left untouched and is safe to retry
//     on a re-run (the run is idempotent).
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

		artistName, artistSpotifyID := primaryArtistForMatch(rel.Artists)
		if artistName == "" || strings.TrimSpace(rel.Title) == "" {
			report.ReleasesSkipped++
			slog.Debug("spotify-enrich: release missing artist or title; skipping",
				"release_id", rel.ID, "title", rel.Title)
			continue
		}

		albums, err := client.SearchAlbums(artistName, rel.Title, spotifyDefaultSearchLimit)
		if err != nil {
			if errors.Is(err, ErrSpotifyRateLimited) {
				return fmt.Errorf("aborting backfill — %w; wait for the throttle to clear and re-run (idempotent)", err)
			}
			report.ReleaseErrors++
			slog.Warn("spotify-enrich: album search failed",
				"release_id", rel.ID, "title", rel.Title, "artist", artistName, "error", err)
			continue
		}

		match, qualifying := pickStrictAlbumMatch(albums, artistName, artistSpotifyID, rel.Title, rel.ReleaseYear)
		if match == nil {
			report.ReleasesSkipped++
			if qualifying > 1 {
				slog.Info("spotify-enrich: ambiguous album match (multiple distinct covers); skipping for candidate-picker",
					"release_id", rel.ID, "title", rel.Title, "artist", artistName, "qualifying", qualifying)
			} else {
				slog.Info("spotify-enrich: no strict album match; skipping",
					"release_id", rel.ID, "title", rel.Title, "artist", artistName, "candidates", len(albums))
			}
			continue
		}

		// Validate-on-write (PSY-525/747/1113): cover_art_url renders in <img src>
		// and cover_art_source_url as an attribution <a href>, so reject a non-https
		// / non-Spotify URL rather than trusting the provider response blindly.
		if !isHTTPSURL(match.ImageURL) || !isSpotifyWebURL(match.SourceURL) {
			report.ReleasesSkipped++
			slog.Warn("spotify-enrich: matched cover failed URL validation; skipping",
				"release_id", rel.ID, "image_url", match.ImageURL, "source_url", match.SourceURL)
			continue
		}
		report.ReleasesMatched++

		slog.Info("spotify-enrich: release cover match",
			"release_id", rel.ID, "title", rel.Title, "artist", artistName,
			"anchored_by_id", artistSpotifyID != "",
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
			if errors.Is(err, ErrSpotifyRateLimited) {
				return fmt.Errorf("aborting backfill — %w; wait for the throttle to clear and re-run (idempotent)", err)
			}
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

		sourceURL := sa.ExternalURLs.Spotify
		if sourceURL == "" {
			sourceURL = spotifyURL // fall back to the curated link
		}

		// Validate-on-write (PSY-525/747/1113): image_url / image_source_url render
		// in <img src> / <a href>, so reject a non-https / non-Spotify URL.
		if !isHTTPSURL(imageURL) || !isSpotifyWebURL(sourceURL) {
			report.ArtistsSkipped++
			slog.Warn("spotify-enrich: artist photo failed URL validation; skipping",
				"artist_id", ar.ID, "image_url", imageURL, "source_url", sourceURL)
			continue
		}
		report.ArtistsMatched++

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

// pickStrictAlbumMatch applies the strict-match-skip-ambiguous policy and returns
// (match, qualifyingCount). match is nil when no candidate qualifies OR when the
// choice is ambiguous; the caller uses qualifyingCount to log which case it was.
//
// A candidate qualifies only when its normalized album name equals the release
// title AND its artist matches: by Spotify artist ID when artistSpotifyID != ""
// (the release's artist has a curated link — immune to two distinct artists
// sharing a name), otherwise by normalized artist name. When both the release and
// the candidate have a known year they must agree within spotifyYearTolerance; a
// candidate with an unknown year is not rejected (never reject on missing provider
// data). A qualifying candidate with no usable image is dropped.
//
// chooseUnambiguous then selects among the qualifying candidates, returning nil
// when multiple carry DIFFERENT covers the year can't disambiguate — so Spotify's
// top hit is never blindly stored.
func pickStrictAlbumMatch(albums []SpotifyAlbum, artistName, artistSpotifyID, title string, releaseYear *int) (*spotifyAlbumMatch, int) {
	wantTitle := normalizeForMatch(title)
	if wantTitle == "" {
		return nil, 0
	}
	wantArtist := normalizeForMatch(artistName)
	anchorByID := artistSpotifyID != ""
	if !anchorByID && wantArtist == "" {
		return nil, 0
	}

	var qualifying []spotifyAlbumMatch
	for _, alb := range albums {
		if normalizeForMatch(alb.Name) != wantTitle {
			continue
		}
		if anchorByID {
			if !albumArtistsContainID(alb.Artists, artistSpotifyID) {
				continue
			}
		} else if !albumArtistsContainName(alb.Artists, wantArtist) {
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

		qualifying = append(qualifying, spotifyAlbumMatch{
			Name:      alb.Name,
			ImageURL:  img,
			SourceURL: alb.ExternalURLs.Spotify,
			Year:      candYear,
		})
	}

	return chooseUnambiguous(qualifying, releaseYear), len(qualifying)
}

// chooseUnambiguous returns the single cover among qualifying candidates, or nil
// when the choice is genuinely ambiguous. One candidate → it. Many candidates that
// all carry the SAME cover image → that cover (different pressings, identical art —
// not ambiguous). Many with DIFFERENT covers → narrow to an exact release-year
// match; if that still doesn't resolve to one cover, return nil (skip + log).
func chooseUnambiguous(qualifying []spotifyAlbumMatch, releaseYear *int) *spotifyAlbumMatch {
	if len(qualifying) == 0 {
		return nil
	}
	if allSameCover(qualifying) {
		return &qualifying[0]
	}
	if releaseYear != nil && *releaseYear > 0 {
		var exact []spotifyAlbumMatch
		for _, q := range qualifying {
			if q.Year == *releaseYear {
				exact = append(exact, q)
			}
		}
		if allSameCover(exact) {
			return &exact[0]
		}
	}
	return nil
}

// allSameCover reports whether every match carries the same image URL — so there
// is really only one cover to choose regardless of how many album rows matched.
// An empty slice is not "same cover".
func allSameCover(matches []spotifyAlbumMatch) bool {
	if len(matches) == 0 {
		return false
	}
	for i := 1; i < len(matches); i++ {
		if matches[i].ImageURL != matches[0].ImageURL {
			return false
		}
	}
	return true
}

// albumArtistsContainID reports whether any album artist has the given Spotify id.
func albumArtistsContainID(artists []SpotifyAlbumArtistRef, wantID string) bool {
	for _, a := range artists {
		if a.ID == wantID {
			return true
		}
	}
	return false
}

// albumArtistsContainName reports whether any album artist's normalized name
// equals the wanted (already-normalized) artist name.
func albumArtistsContainName(artists []SpotifyAlbumArtistRef, wantNormalized string) bool {
	for _, a := range artists {
		if normalizeForMatch(a.Name) == wantNormalized {
			return true
		}
	}
	return false
}

// primaryArtistForMatch chooses the artist to search + anchor a release's cover
// by, returning (name, spotifyID). It prefers an artist with a parseable curated
// Spotify link — enabling ID-anchored matching that is immune to same-name
// distinct artists — and falls back to the first non-empty name (matched by name,
// with the ambiguity gate as the backstop). name and spotifyID always come from
// the SAME artist, so the search term and the ID anchor never disagree.
func primaryArtistForMatch(artists []catalogm.Artist) (name, spotifyID string) {
	fallback := ""
	for i := range artists {
		nm := strings.TrimSpace(artists[i].Name)
		if nm == "" {
			continue
		}
		if artists[i].Social.Spotify != nil {
			if id := extractSpotifyArtistID(*artists[i].Social.Spotify); id != "" {
				return nm, id
			}
		}
		if fallback == "" {
			fallback = nm
		}
	}
	return fallback, ""
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
	// On the (UTF-8-impossible) transform error we keep the lowercased input rather
	// than failing, so a partial fold never silently corrupts the match key —
	// mirrors normalizeName's documented fallback (radio_matching.go).
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
// link on a look-alike host yields "". "artist" is accepted ONLY as the first
// path segment, or the second after an intl-* locale prefix or an /embed prefix
// — so /user/<name>/... (e.g. a Spotify user literally named "artist") can't be
// misread as an artist link. Returns "" for anything that does not parse to a
// real https Spotify artist URL.
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

	// Path forms: /artist/<id>, /intl-de/artist/<id>, /embed/artist/<id>.
	// segs[0] is the leading "" before the first slash.
	segs := strings.Split(u.Path, "/")
	artistIdx := -1
	switch {
	case len(segs) >= 2 && segs[1] == "artist":
		artistIdx = 1
	case len(segs) >= 3 && (strings.HasPrefix(segs[1], "intl-") || segs[1] == "embed") && segs[2] == "artist":
		artistIdx = 2
	}
	if artistIdx == -1 || artistIdx+1 >= len(segs) {
		return ""
	}
	if id := segs[artistIdx+1]; spotifyArtistIDPattern.MatchString(id) {
		return id
	}
	return ""
}

// isHTTPSURL reports whether raw is a parseable https URL with a host. Gates
// cover_art_url / image_url before persisting (they render in <img src>); the
// image CDN host varies (i.scdn.co / mosaic.scdn.co / *.spotifycdn.com), so we
// require https + a host rather than pinning the host.
func isHTTPSURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.Host != ""
}

// isSpotifyWebURL reports whether raw is an https open.spotify.com URL. Gates the
// *_source_url linkback (rendered as an attribution <a href>) before persisting —
// the repo's host-anchored validate-on-write contract (PSY-1113).
func isSpotifyWebURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "https" && strings.ToLower(u.Hostname()) == "open.spotify.com"
}

// absInt returns the absolute value of n.
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
