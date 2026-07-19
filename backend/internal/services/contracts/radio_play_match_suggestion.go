package contracts

import (
	"errors"
	"time"
)

// ──────────────────────────────────────────────
// Radio play match suggestions — PSY-1494
//
// Community "suggest a match" queue for unmatched radio plays (PSY-1052
// Option 2). Community submits pending rows; admins accept (LinkPlay +
// optional BulkLinkPlays) or reject. Community NEVER hits LinkPlay/BulkLink.
// Trusted-tier auto-apply is out of scope.
// ──────────────────────────────────────────────

// CreateRadioPlayMatchSuggestionRequest is the community POST body.
type CreateRadioPlayMatchSuggestionRequest struct {
	ArtistID uint    `json:"artist_id" doc:"Artist ID to suggest as the match"`
	Note     *string `json:"note,omitempty" doc:"Optional free-text rationale"`
}

// RadioPlayMatchSuggestionEntry is one suggestion row for list/detail responses.
type RadioPlayMatchSuggestionEntry struct {
	ID                  uint       `json:"id" doc:"Suggestion row ID"`
	PlayID              uint       `json:"play_id" doc:"Radio play being matched"`
	PlayArtistName      string     `json:"play_artist_name" doc:"Raw artist_name on the play"`
	PlayMatchState      string     `json:"play_match_state" doc:"Current match_state on the play"`
	SuggestedArtistID   uint       `json:"suggested_artist_id" doc:"Suggested artist ID"`
	SuggestedArtistName string     `json:"suggested_artist_name" doc:"Suggested artist name (joined)"`
	SuggestedArtistSlug *string    `json:"suggested_artist_slug,omitempty" doc:"Suggested artist slug"`
	SubmittedBy         uint       `json:"submitted_by" doc:"Submitter user ID"`
	SubmitterUsername   *string    `json:"submitter_username,omitempty" doc:"Submitter username when available"`
	Note                *string    `json:"note,omitempty" doc:"Optional submitter note"`
	Status              string     `json:"status" doc:"pending | accepted | rejected"`
	ReviewedBy          *uint      `json:"reviewed_by,omitempty" doc:"Reviewer user ID"`
	ReviewedAt          *time.Time `json:"reviewed_at,omitempty" doc:"When the review happened"`
	RejectionReason     *string    `json:"rejection_reason,omitempty" doc:"Reason stamped on reject"`
	CreatedAt           time.Time  `json:"created_at" doc:"When the suggestion was submitted"`
}

// RadioPlayMatchSuggestionListResult is the paginated admin pending list.
type RadioPlayMatchSuggestionListResult struct {
	Suggestions []RadioPlayMatchSuggestionEntry `json:"suggestions"`
	Total       int64                           `json:"total"`
}

// AcceptRadioPlayMatchSuggestionRequest is the admin accept body.
type AcceptRadioPlayMatchSuggestionRequest struct {
	// AlsoBulkLinkName, when true, also runs BulkLinkPlays for the play's
	// artist_name → suggested artist (admin-only, audited). Default false.
	AlsoBulkLinkName bool `json:"also_bulk_link_name,omitempty" doc:"Also bulk-link all unmatched plays with the same artist_name"`
}

// RejectRadioPlayMatchSuggestionRequest is the admin reject body.
type RejectRadioPlayMatchSuggestionRequest struct {
	Reason string `json:"reason" doc:"Rejection reason (required)"`
}

// RadioPlayMatchSuggestionReviewResult is the accept/reject response.
type RadioPlayMatchSuggestionReviewResult struct {
	ID                uint       `json:"id"`
	PlayID            uint       `json:"play_id"`
	SuggestedArtistID uint       `json:"suggested_artist_id"`
	SubmittedBy       uint       `json:"submitted_by"`
	Status            string     `json:"status" doc:"accepted or rejected"`
	ReviewedBy        *uint      `json:"reviewed_by,omitempty"`
	ReviewedAt        *time.Time `json:"reviewed_at,omitempty"`
	RejectionReason   *string    `json:"rejection_reason,omitempty"`
	// BulkUpdated is set on accept when also_bulk_link_name was true.
	BulkUpdated *int `json:"bulk_updated,omitempty" doc:"Plays updated by optional BulkLinkPlays"`
	// NewlyReviewed is true when this call performed the pending→terminal
	// transition (vs an idempotent replay). Not serialized — handler uses it
	// to send approval email only once.
	NewlyReviewed bool `json:"-"`
}

// Typed errors for radio play match suggestions. Handlers map these to HTTP codes.
var (
	ErrRadioPlayMatchSuggestionNotFound = errors.New("radio play match suggestion not found")
	// ErrRadioPlayMatchSuggestionAlreadyReviewed: conflicting terminal verdict (409).
	ErrRadioPlayMatchSuggestionAlreadyReviewed = errors.New("radio play match suggestion already reviewed")
	// ErrRadioPlayMatchSuggestionDuplicatePending: user already has a pending row for this play (409).
	ErrRadioPlayMatchSuggestionDuplicatePending = errors.New("pending match suggestion already exists for this play")
	// ErrRadioPlayMatchSuggestionPlayNotSuggestable: play missing, already linked, or wrong match_state (422).
	ErrRadioPlayMatchSuggestionPlayNotSuggestable = errors.New("radio play is not suggestable")
	// ErrRadioPlayMatchSuggestionArtistNotFound: suggested artist_id does not exist (404).
	ErrRadioPlayMatchSuggestionArtistNotFound = errors.New("suggested artist not found")
	// ErrRadioPlayMatchSuggestionRejectReasonRequired: reject without reason (422).
	ErrRadioPlayMatchSuggestionRejectReasonRequired = errors.New("rejection reason is required")
)

// RadioPlayMatchSuggestionServiceInterface is the community submit + admin
// review contract for radio play match suggestions (PSY-1494).
type RadioPlayMatchSuggestionServiceInterface interface {
	CreateSuggestion(playID, submitterID uint, req *CreateRadioPlayMatchSuggestionRequest) (*RadioPlayMatchSuggestionEntry, error)
	GetOwnPendingSuggestion(playID, submitterID uint) (*RadioPlayMatchSuggestionEntry, error)
	ListPendingSuggestions(limit, offset int) (*RadioPlayMatchSuggestionListResult, error)
	AcceptSuggestion(suggestionID, reviewerUserID uint, req *AcceptRadioPlayMatchSuggestionRequest) (*RadioPlayMatchSuggestionReviewResult, error)
	RejectSuggestion(suggestionID, reviewerUserID uint, req *RejectRadioPlayMatchSuggestionRequest) (*RadioPlayMatchSuggestionReviewResult, error)
}
