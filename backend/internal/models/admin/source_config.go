package admin

import "time"

// Source entity types for the polymorphic source-config registry.
const (
	SourceEntityVenue = "venue"
	SourceEntityLabel = "label"
)

// SourceConfig is a polymorphic registry row tracking an external source for a
// catalog entity — a venue's calendar page or a label's roster page — so the
// Catalog Refresh loop can refresh the stalest sources first across both types.
//
// Decoupled from the legacy VenueSourceConfig (which is tied to the retiring AI
// pipeline, PSY-1158): the /ingest skill is the executor and stamps
// LastRefreshedAt after each run. Polymorphic like comments/reports — there is
// intentionally no FK on EntityID (a single FK can't span two parent tables).
type SourceConfig struct {
	ID                  uint       `json:"id" gorm:"primaryKey"`
	EntityType          string     `json:"entity_type" gorm:"column:entity_type;not null;uniqueIndex:idx_source_configs_entity"`
	EntityID            uint       `json:"entity_id" gorm:"column:entity_id;not null;uniqueIndex:idx_source_configs_entity"`
	SourceURL           *string    `json:"source_url" gorm:"column:source_url"`
	LastRefreshedAt     *time.Time `json:"last_refreshed_at" gorm:"column:last_refreshed_at"`
	LastContentHash     *string    `json:"last_content_hash" gorm:"column:last_content_hash"`
	ConsecutiveFailures int        `json:"consecutive_failures" gorm:"column:consecutive_failures;not null;default:0"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (SourceConfig) TableName() string { return "source_configs" }
