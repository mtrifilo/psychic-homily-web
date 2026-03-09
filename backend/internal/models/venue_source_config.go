package models

import "time"

// VenueSourceConfig tracks per-venue extraction configuration for the AI pipeline.
type VenueSourceConfig struct {
	ID                  uint       `json:"id" gorm:"primaryKey"`
	VenueID             uint       `json:"venue_id" gorm:"column:venue_id;uniqueIndex;not null"`
	CalendarURL         *string    `json:"calendar_url" gorm:"column:calendar_url"`
	PreferredSource     string     `json:"preferred_source" gorm:"column:preferred_source;not null;default:'ai'"`
	RenderMethod        *string    `json:"render_method" gorm:"column:render_method"`
	FeedURL             *string    `json:"feed_url" gorm:"column:feed_url"`
	LastContentHash     *string    `json:"last_content_hash" gorm:"column:last_content_hash"`
	LastETag            *string    `json:"last_etag" gorm:"column:last_etag"`
	LastExtractedAt     *time.Time `json:"last_extracted_at" gorm:"column:last_extracted_at"`
	EventsExpected      int        `json:"events_expected" gorm:"column:events_expected;not null;default:0"`
	ConsecutiveFailures int        `json:"consecutive_failures" gorm:"column:consecutive_failures;not null;default:0"`
	StrategyLocked      bool       `json:"strategy_locked" gorm:"column:strategy_locked;not null;default:false"`
	AutoApprove         bool       `json:"auto_approve" gorm:"column:auto_approve;not null;default:true"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	Venue Venue `json:"-" gorm:"foreignKey:VenueID"`
}

func (VenueSourceConfig) TableName() string { return "venue_source_configs" }
