package admin

import (
	"time"

	"psychic-homily-backend/internal/models/catalog"
)

// VenueExtractionRun records a single extraction attempt for a venue.
type VenueExtractionRun struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	VenueID         uint      `json:"venue_id" gorm:"column:venue_id;not null"`
	RunAt           time.Time `json:"run_at" gorm:"column:run_at;not null;default:NOW()"`
	RenderMethod    *string   `json:"render_method" gorm:"column:render_method"`
	PreferredSource *string   `json:"preferred_source" gorm:"column:preferred_source"`
	EventsExtracted int       `json:"events_extracted" gorm:"column:events_extracted;not null;default:0"`
	EventsImported  int       `json:"events_imported" gorm:"column:events_imported;not null;default:0"`
	ContentHash     *string   `json:"content_hash" gorm:"column:content_hash"`
	HTTPStatus      *int      `json:"http_status" gorm:"column:http_status"`
	Error           *string   `json:"error" gorm:"column:error"`
	DurationMs      int       `json:"duration_ms" gorm:"column:duration_ms;not null;default:0"`
	CreatedAt       time.Time `json:"created_at"`

	Venue catalog.Venue `json:"-" gorm:"foreignKey:VenueID"`
}

func (VenueExtractionRun) TableName() string { return "venue_extraction_runs" }
