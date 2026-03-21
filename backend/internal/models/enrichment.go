package models

import (
	"encoding/json"
	"time"
)

// Enrichment type constants
const (
	EnrichmentTypeArtistMatch = "artist_match"
	EnrichmentTypeMusicBrainz = "musicbrainz"
	EnrichmentTypeAPICrossRef = "api_crossref"
	EnrichmentTypeAll         = "all"
)

// Enrichment status constants
const (
	EnrichmentStatusPending    = "pending"
	EnrichmentStatusProcessing = "processing"
	EnrichmentStatusCompleted  = "completed"
	EnrichmentStatusFailed     = "failed"
)

// EnrichmentQueueItem represents a queued enrichment job for a show.
type EnrichmentQueueItem struct {
	ID             uint             `json:"id" gorm:"primaryKey"`
	ShowID         uint             `json:"show_id" gorm:"column:show_id;not null"`
	Status         string           `json:"status" gorm:"column:status;not null;default:'pending'"`
	Attempts       int              `json:"attempts" gorm:"column:attempts;not null;default:0"`
	MaxAttempts    int              `json:"max_attempts" gorm:"column:max_attempts;not null;default:3"`
	LastError      *string          `json:"last_error" gorm:"column:last_error"`
	EnrichmentType string           `json:"enrichment_type" gorm:"column:enrichment_type;not null"`
	Results        *json.RawMessage `json:"results" gorm:"column:results;type:jsonb"`
	CreatedAt      time.Time        `json:"created_at" gorm:"not null"`
	UpdatedAt      time.Time        `json:"updated_at" gorm:"not null"`
	CompletedAt    *time.Time       `json:"completed_at" gorm:"column:completed_at"`

	Show Show `json:"-" gorm:"foreignKey:ShowID"`
}

func (EnrichmentQueueItem) TableName() string { return "enrichment_queue" }
