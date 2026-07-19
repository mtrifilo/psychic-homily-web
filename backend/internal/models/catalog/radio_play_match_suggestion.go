package catalog

import "time"

// RadioPlayMatchSuggestion is one community-submitted artist match for an
// unmatched radio play (PSY-1494 / PSY-1052 Option 2). Community inserts
// pending rows; admins accept (LinkPlay + optional BulkLinkPlays) or reject.
// Nothing is auto-applied — trusted-tier auto-apply is explicitly out of scope.
//
// Status values are CHECK-constrained at the DB — keep the const block in sync
// with the PSY-1494 migration.
type RadioPlayMatchSuggestion struct {
	ID                uint `gorm:"primaryKey"`
	PlayID            uint `gorm:"column:play_id;not null"`
	SuggestedArtistID uint `gorm:"column:suggested_artist_id;not null"`
	SubmittedBy       uint `gorm:"column:submitted_by;not null"`
	// Note is an optional free-text rationale from the submitter.
	Note   *string `gorm:"column:note;type:text"`
	Status string  `gorm:"column:status;size:10;not null;default:pending"`

	ReviewedBy *uint      `gorm:"column:reviewed_by"`
	ReviewedAt *time.Time `gorm:"column:reviewed_at"`
	// RejectionReason is stamped on reject; nil while pending/accepted.
	RejectionReason *string `gorm:"column:rejection_reason;type:text"`

	CreatedAt time.Time `gorm:"column:created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (RadioPlayMatchSuggestion) TableName() string {
	return "radio_play_match_suggestions"
}

// Radio play match-suggestion status values. CHECK-constrained at the DB
// (PSY-1494 migration) — keep in sync with the column constraint.
const (
	RadioPlayMatchSuggestionStatusPending  = "pending"
	RadioPlayMatchSuggestionStatusAccepted = "accepted"
	RadioPlayMatchSuggestionStatusRejected = "rejected"
)
