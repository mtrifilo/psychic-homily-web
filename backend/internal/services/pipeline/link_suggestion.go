package pipeline

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// spotifyArtistPathRe matches an /artist/<id> segment anywhere in the path,
// mirroring the per-artist Spotify accept endpoint's gate
// (handlers/catalog/artist.go spotifyArtistPath) so the bulk accept enforces the
// SAME shape: a locale prefix (/intl-de/artist/...) or trailing sub-tab still
// validates; the security win is the open.spotify.com host anchor below.
var spotifyArtistPathRe = regexp.MustCompile(`/artist/[A-Za-z0-9]+(?:/|$)`)

// LinkSuggestionService owns the admin music-link suggestion review queue
// (PSY-1199). The list query returns pending rows joined to their artist,
// high-confidence first. Accept reuses the EXISTING artist update path
// (artistService.UpdateArtist) to write the link — Spotify sets social.spotify;
// Bandcamp sets social.bandcamp, which triggers the PSY-1190 profile→embed
// resolver inside UpdateArtist — so the write/resolve logic is NOT duplicated
// here. Reject just marks the row. Both stamp reviewer + reviewed_at and are
// idempotent on replay of the same verdict.
type LinkSuggestionService struct {
	db *gorm.DB
	// artistService is the existing artist write path. AcceptSuggestion calls
	// UpdateArtist so the link write (and, for Bandcamp, the profile→embed
	// resolver) goes through the SAME validated, provenance-stamping code the
	// per-artist accept endpoint uses — never a parallel writer.
	artistService contracts.ArtistServiceInterface
	// now is injectable so tests can assert the reviewed_at stamp deterministically.
	now func() time.Time
}

// NewLinkSuggestionService creates the service. A nil database resolves to the
// process default. artistService is the existing artist write path the accept
// flow reuses.
func NewLinkSuggestionService(database *gorm.DB, artistService contracts.ArtistServiceInterface) *LinkSuggestionService {
	if database == nil {
		database = db.GetDB()
	}
	return &LinkSuggestionService{
		db:            database,
		artistService: artistService,
		now:           time.Now,
	}
}

// ListPendingSuggestions returns pending suggestions joined to their artist,
// ordered high-confidence first (then by id ASC for stable pagination).
//
// limit is clamped to [1, 200]; offset is clamped to >= 0.
func (s *LinkSuggestionService) ListPendingSuggestions(limit, offset int) (*contracts.LinkSuggestionListResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	// 'high' must sort before 'review', which a plain DESC/ASC on the string
	// won't give ('high' < 'review' alphabetically — ASC would already put high
	// first, but make the intent explicit so a future confidence tier doesn't
	// silently reorder). The id tiebreak is deterministic for pagination.
	query := `
		SELECT
			als.id,
			als.artist_id,
			a.name  AS artist_name,
			a.slug  AS artist_slug,
			als.platform,
			als.url,
			als.source,
			als.mb_artist_id,
			als.mb_artist_name,
			als.confidence,
			als.region_match,
			als.live,
			als.notes,
			als.status,
			als.created_at
		FROM artist_link_suggestions als
		JOIN artists a ON a.id = als.artist_id
		WHERE als.status = ?
		ORDER BY (CASE WHEN als.confidence = 'high' THEN 0 ELSE 1 END) ASC, als.id ASC
		LIMIT ? OFFSET ?
	`

	entries := make([]contracts.LinkSuggestionEntry, 0)
	if err := s.db.Raw(query, catalogm.LinkSuggestionStatusPending, limit, offset).Scan(&entries).Error; err != nil {
		return nil, fmt.Errorf("list link suggestions: %w", err)
	}

	var total int64
	if err := s.db.Model(&catalogm.ArtistLinkSuggestion{}).
		Where("status = ?", catalogm.LinkSuggestionStatusPending).
		Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count link suggestions: %w", err)
	}

	return &contracts.LinkSuggestionListResult{
		Suggestions: entries,
		Total:       total,
	}, nil
}

// AcceptSuggestion writes the suggested link to the artist via the existing
// artist update path, then marks the row accepted and stamps the reviewer.
//
// Idempotency + replay safety:
//   - A row already 'accepted' returns the existing stamp WITHOUT re-writing the
//     link (the write is gated on the pending→accepted transition), so a double
//     POST never double-writes.
//   - A row already 'rejected' returns ErrLinkSuggestionAlreadyReviewed (a
//     conflicting verdict — the handler maps it to 409).
//
// The link write reuses artistService.UpdateArtist: Spotify sets the spotify
// social URL; Bandcamp sets the bandcamp social URL, which triggers the PSY-1190
// profile→embed resolver inside UpdateArtist (fill-when-empty). No write/resolve
// logic is duplicated here.
func (s *LinkSuggestionService) AcceptSuggestion(suggestionID, reviewerUserID uint) (*contracts.LinkSuggestionReviewResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if s.artistService == nil {
		return nil, fmt.Errorf("artist service not configured")
	}

	suggestion, err := s.loadSuggestion(suggestionID)
	if err != nil {
		return nil, err
	}

	// Idempotent replay: already accepted → return the stored stamp, no re-write.
	if suggestion.Status == catalogm.LinkSuggestionStatusAccepted {
		return reviewResultFromModel(suggestion), nil
	}
	// Conflicting verdict: already rejected → 409.
	if suggestion.Status == catalogm.LinkSuggestionStatusRejected {
		return nil, contracts.ErrLinkSuggestionAlreadyReviewed
	}

	// Write the link via the existing artist update path. UpdateArtist surfaces
	// ErrArtistNotFound when the artist is gone; the FK ON DELETE CASCADE means a
	// pending suggestion for a deleted artist shouldn't exist, but treat a missing
	// artist as a hard failure rather than silently marking accepted.
	if err := s.applyLink(suggestion); err != nil {
		return nil, fmt.Errorf("apply suggested link: %w", err)
	}

	// Mark accepted + stamp the reviewer. The WHERE re-asserts status='pending'
	// so two concurrent accepts can't both write the link AND both stamp — the
	// second sees RowsAffected==0 and re-reads the already-accepted row below.
	reviewedAt := s.now().UTC()
	res := s.db.Model(&catalogm.ArtistLinkSuggestion{}).
		Where("id = ? AND status = ?", suggestionID, catalogm.LinkSuggestionStatusPending).
		Updates(map[string]interface{}{
			"status":              catalogm.LinkSuggestionStatusAccepted,
			"reviewed_at":         reviewedAt,
			"reviewed_by_user_id": reviewerUserID,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("mark suggestion accepted: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Lost the race — another accept already stamped it. Re-read and return.
		reloaded, err := s.loadSuggestion(suggestionID)
		if err != nil {
			return nil, err
		}
		return reviewResultFromModel(reloaded), nil
	}

	return &contracts.LinkSuggestionReviewResult{
		ID:               suggestionID,
		ArtistID:         suggestion.ArtistID,
		Status:           catalogm.LinkSuggestionStatusAccepted,
		ReviewedAt:       &reviewedAt,
		ReviewedByUserID: &reviewerUserID,
	}, nil
}

// RejectSuggestion marks the row rejected and stamps the reviewer.
//
// Idempotency + replay safety:
//   - A row already 'rejected' returns the existing stamp (no-op).
//   - A row already 'accepted' returns ErrLinkSuggestionAlreadyReviewed (409).
func (s *LinkSuggestionService) RejectSuggestion(suggestionID, reviewerUserID uint) (*contracts.LinkSuggestionReviewResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	suggestion, err := s.loadSuggestion(suggestionID)
	if err != nil {
		return nil, err
	}

	if suggestion.Status == catalogm.LinkSuggestionStatusRejected {
		return reviewResultFromModel(suggestion), nil
	}
	if suggestion.Status == catalogm.LinkSuggestionStatusAccepted {
		return nil, contracts.ErrLinkSuggestionAlreadyReviewed
	}

	reviewedAt := s.now().UTC()
	res := s.db.Model(&catalogm.ArtistLinkSuggestion{}).
		Where("id = ? AND status = ?", suggestionID, catalogm.LinkSuggestionStatusPending).
		Updates(map[string]interface{}{
			"status":              catalogm.LinkSuggestionStatusRejected,
			"reviewed_at":         reviewedAt,
			"reviewed_by_user_id": reviewerUserID,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("mark suggestion rejected: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		reloaded, err := s.loadSuggestion(suggestionID)
		if err != nil {
			return nil, err
		}
		return reviewResultFromModel(reloaded), nil
	}

	return &contracts.LinkSuggestionReviewResult{
		ID:               suggestionID,
		ArtistID:         suggestion.ArtistID,
		Status:           catalogm.LinkSuggestionStatusRejected,
		ReviewedAt:       &reviewedAt,
		ReviewedByUserID: &reviewerUserID,
	}, nil
}

// applyLink writes the suggestion's URL to the artist via the existing artist
// update path, routing on platform. ArtistService.UpdateArtist does NOT validate
// social URLs (that gate lives in the per-artist HTTP handlers), and the PSY-1190
// resolver only re-anchors the FETCH host — neither stops a hostile/foreign value
// being STORED in the rendered social link. So this boundary re-applies the SAME
// host-anchored gate the per-artist accept endpoints enforce before writing
// (defense-in-depth against a sweep bug or a hand-inserted row), then delegates
// the write (and, for Bandcamp, the profile→embed resolution) to UpdateArtist.
func (s *LinkSuggestionService) applyLink(suggestion *catalogm.ArtistLinkSuggestion) error {
	linkURL := suggestion.URL
	var req contracts.UpdateArtistRequest
	switch suggestion.Platform {
	case contracts.MusicPlatformSpotify:
		if !isValidSpotifyArtistURL(linkURL) {
			return contracts.ErrLinkSuggestionInvalidURL
		}
		req.Spotify = &linkURL
	case contracts.MusicPlatformBandcamp:
		if !isBandcampProfileURL(linkURL) {
			return contracts.ErrLinkSuggestionInvalidURL
		}
		// Setting the bandcamp social URL triggers the PSY-1190 resolver inside
		// UpdateArtist, which fills bandcamp_embed_url from the profile root.
		req.Bandcamp = &linkURL
	default:
		return fmt.Errorf("unsupported suggestion platform %q", suggestion.Platform)
	}

	_, err := s.artistService.UpdateArtist(suggestion.ArtistID, &req)
	return err
}

// isValidSpotifyArtistURL mirrors the per-artist endpoint's isValidSpotifyURL:
// an http/https URL anchored on the open.spotify.com host with an /artist/<id>
// path segment. The host anchor (not a substring of the whole URL) is the
// security win — it rejects "https://evil.test/artist/x".
func isValidSpotifyArtistURL(rawURL string) bool {
	u, ok := parseHTTPURL(rawURL)
	if !ok {
		return false
	}
	if strings.ToLower(u.Hostname()) != "open.spotify.com" {
		return false
	}
	return spotifyArtistPathRe.MatchString(u.Path)
}

// isBandcampProfileURL accepts an http/https *.bandcamp.com artist subdomain
// (a profile root — the only Bandcamp shape this queue stores and the only one
// the PSY-1190 resolver acts on). Anchors on the parsed host via
// utils.IsBandcampArtistHost, so a hostile value like
// "https://169.254.169.254/?x=bandcamp.com" is rejected before it is stored.
func isBandcampProfileURL(rawURL string) bool {
	u, ok := parseHTTPURL(rawURL)
	if !ok {
		return false
	}
	return utils.IsBandcampArtistHost(u.Hostname())
}

// parseHTTPURL parses rawURL and confirms an http/https scheme and a non-empty
// host. Mirrors the per-artist handler's parseHTTPURL so the two accept paths
// reject the same malformed inputs.
func parseHTTPURL(rawURL string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}
	if u.Hostname() == "" {
		return nil, false
	}
	return u, true
}

// loadSuggestion fetches the suggestion row, translating a missing row into the
// typed ErrLinkSuggestionNotFound the handler maps to a 404.
func (s *LinkSuggestionService) loadSuggestion(suggestionID uint) (*catalogm.ArtistLinkSuggestion, error) {
	var suggestion catalogm.ArtistLinkSuggestion
	if err := s.db.First(&suggestion, suggestionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contracts.ErrLinkSuggestionNotFound
		}
		return nil, fmt.Errorf("load link suggestion: %w", err)
	}
	return &suggestion, nil
}

// UpsertSuggestions inserts each discovered candidate as a PENDING
// artist_link_suggestions row, skipping any (artist_id, platform, url) that
// already exists. It is the WRITE counterpart to the list/accept/reject methods
// above — the batch sweep cmd (PSY-1206) is its only caller today — so the
// store's persistence mechanics (the ON CONFLICT clause, the candidate→row
// mapping) stay encapsulated in the service that owns the table, not in the cmd.
//
// Returns the number of rows ACTUALLY inserted (RowsAffected), which the caller
// uses to report idempotency: a re-discovered candidate contributes 0.
//
// ON CONFLICT DO NOTHING (not DO UPDATE) is deliberate: the unique key IS the
// row identity, so a conflict means this exact candidate was already queued —
// possibly already accepted or rejected by a human. DO NOTHING leaves that
// reviewed row untouched (never flips it back to pending), which is what makes a
// re-sweep safe/resumable. An empty candidate list is a no-op (0, nil).
func (s *LinkSuggestionService) UpsertSuggestions(artistID uint, candidates []contracts.MusicLinkCandidate) (int, error) {
	if len(candidates) == 0 {
		return 0, nil
	}
	rows := make([]catalogm.ArtistLinkSuggestion, 0, len(candidates))
	for _, c := range candidates {
		rows = append(rows, catalogm.ArtistLinkSuggestion{
			ArtistID:     artistID,
			Platform:     c.Platform,
			URL:          c.URL,
			Source:       c.Source,
			MBArtistID:   nilIfEmpty(c.MBArtistID),
			MBArtistName: nilIfEmpty(c.MBArtistName),
			Confidence:   c.Confidence,
			RegionMatch:  c.RegionMatch,
			Live:         c.Live,
			Notes:        nilIfEmpty(c.Notes),
			Status:       catalogm.LinkSuggestionStatusPending,
		})
	}

	res := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "artist_id"}, {Name: "platform"}, {Name: "url"}},
		DoNothing: true,
	}).Create(&rows)
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// nilIfEmpty maps the contract's value-type "" (its zero value for an absent
// optional field) to a nil *string so the nullable mb_artist_id / mb_artist_name
// / notes columns store SQL NULL rather than an empty string.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// reviewResultFromModel builds the review response from a stored row, used on an
// idempotent replay (the row already carries the stamp).
func reviewResultFromModel(suggestion *catalogm.ArtistLinkSuggestion) *contracts.LinkSuggestionReviewResult {
	return &contracts.LinkSuggestionReviewResult{
		ID:               suggestion.ID,
		ArtistID:         suggestion.ArtistID,
		Status:           suggestion.Status,
		ReviewedAt:       suggestion.ReviewedAt,
		ReviewedByUserID: suggestion.ReviewedByUserID,
	}
}
