package contracts

import (
	"errors"
	"time"
)

// ──────────────────────────────────────────────
// Music-link suggestion review queue — PSY-1199
//
// The bulk-backfill counterpart to the per-artist discover-music endpoint
// (PSY-1191). A sweep cmd (PSY-1206) pre-computes MusicBrainz-sourced
// Bandcamp/Spotify candidates into artist_link_suggestions; the admin review
// API (this ticket) lists pending rows and accepts/rejects them; a triage UI
// (PSY-1207) drives it. Shapes here are the LOCKED contract PSY-1206/1207 build
// against.
// ──────────────────────────────────────────────

// LinkSuggestionEntry is one pending suggestion in the review queue, joined to
// its artist (name/slug) for direct rendering. Shape is LOCKED.
type LinkSuggestionEntry struct {
	ID           uint       `json:"id" doc:"Suggestion row ID — pass to accept/reject"`
	ArtistID     uint       `json:"artist_id" doc:"The artist this link is suggested for"`
	ArtistName   string     `json:"artist_name" doc:"Artist name (joined)"`
	ArtistSlug   *string    `json:"artist_slug,omitempty" doc:"Artist slug (joined; may be null)"`
	Platform     string     `json:"platform" doc:"Streaming platform: 'bandcamp' or 'spotify'"`
	URL          string     `json:"url" doc:"Candidate profile/artist URL"`
	Source       string     `json:"source" doc:"Discovery source (always 'musicbrainz')"`
	MBArtistID   *string    `json:"mb_artist_id,omitempty" doc:"MusicBrainz artist UUID this link came from"`
	MBArtistName *string    `json:"mb_artist_name,omitempty" doc:"MusicBrainz artist name"`
	Confidence   string     `json:"confidence" doc:"Region confidence tier: 'high' or 'review'"`
	RegionMatch  bool       `json:"region_match" doc:"True if the MB region aligned with a PH show region"`
	Live         bool       `json:"live" doc:"True if the URL passed an SSRF-guarded liveness probe at sweep time"`
	Notes        *string    `json:"notes,omitempty" doc:"Optional reviewer note"`
	Status       string     `json:"status" doc:"Review state: always 'pending' in the list"`
	CreatedAt    time.Time  `json:"created_at" doc:"When the sweep discovered this candidate"`
}

// LinkSuggestionListResult is the paginated review-queue response. Shape is
// LOCKED (PSY-1207 builds against it).
type LinkSuggestionListResult struct {
	Suggestions []LinkSuggestionEntry `json:"suggestions" doc:"Pending suggestions, high-confidence first"`
	Total       int64                 `json:"total" doc:"Total pending suggestions matching the query"`
}

// LinkSuggestionReviewResult is the response from accept/reject: the resulting
// status + reviewer stamp. Shape is LOCKED.
type LinkSuggestionReviewResult struct {
	ID               uint       `json:"id" doc:"The reviewed suggestion ID"`
	ArtistID         uint       `json:"artist_id" doc:"The artist the suggestion was for"`
	Status           string     `json:"status" doc:"Resulting status: 'accepted' or 'rejected'"`
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty" doc:"When the review happened"`
	ReviewedByUserID *uint      `json:"reviewed_by_user_id,omitempty" doc:"Reviewer user ID"`
}

// ErrLinkSuggestionNotFound is returned when the suggestion row does not exist.
// Handler maps this to a 404.
var ErrLinkSuggestionNotFound = errors.New("link suggestion not found")

// ErrLinkSuggestionAlreadyReviewed is returned when accept/reject targets a row
// that is already accepted or rejected and the requested terminal state DIFFERS
// from the stored one (a re-review with a conflicting verdict). Replaying the
// SAME verdict is idempotent (no error). Handler maps this to a 409.
var ErrLinkSuggestionAlreadyReviewed = errors.New("link suggestion already reviewed")

// LinkSuggestionServiceInterface defines the contract for the admin music-link
// suggestion review queue. AcceptSuggestion writes the link via the existing
// artist update path (Spotify → social.spotify; Bandcamp → social.bandcamp +
// the PSY-1190 profile→embed resolver) and marks the row accepted; both
// accept and reject stamp reviewer + reviewed_at and are idempotent on replay.
type LinkSuggestionServiceInterface interface {
	ListPendingSuggestions(limit, offset int) (*LinkSuggestionListResult, error)
	AcceptSuggestion(suggestionID, reviewerUserID uint) (*LinkSuggestionReviewResult, error)
	RejectSuggestion(suggestionID, reviewerUserID uint) (*LinkSuggestionReviewResult, error)
}
