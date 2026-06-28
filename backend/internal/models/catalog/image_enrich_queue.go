package catalog

import "time"

// Image-enrich outbox entity types (PSY-1247). The queue is polymorphic; this
// names which catalog table entity_id points at.
const (
	ImageEnrichEntityArtist  = "artist"
	ImageEnrichEntityRelease = "release"
)

// Image-enrich outbox status values. pending → processing (claimed) → done, or
// → pending (retry) → failed once attempts reach max_attempts.
const (
	ImageEnrichStatusPending    = "pending"
	ImageEnrichStatusProcessing = "processing"
	ImageEnrichStatusDone       = "done"
	ImageEnrichStatusFailed     = "failed"
)

// ImageEnrichQueueItem is a transactional-outbox job (PSY-1247): a row enqueued in
// the SAME transaction as a newly-created artist/release via the catalog create
// funnel, then drained by the imageenrich outbox poller, which runs the shipped
// fill-when-empty enrichers for prompt, on-create image coverage.
//
// It deliberately lives in models/catalog (not the imageenrich service package) so
// the catalog funnel can insert it without importing imageenrich — which would
// cycle (imageenrich already imports services/catalog). Mirrors the existing
// admin.EnrichmentQueueItem shape (the per-show enrichment queue).
type ImageEnrichQueueItem struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	EntityType  string     `json:"entity_type" gorm:"column:entity_type;not null"`
	EntityID    uint       `json:"entity_id" gorm:"column:entity_id;not null"`
	Status      string     `json:"status" gorm:"column:status;not null;default:'pending'"`
	Attempts    int        `json:"attempts" gorm:"column:attempts;not null;default:0"`
	MaxAttempts int        `json:"max_attempts" gorm:"column:max_attempts;not null;default:3"`
	LastError   *string    `json:"last_error" gorm:"column:last_error"`
	CreatedAt   time.Time  `json:"created_at" gorm:"not null"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"not null"`
	ProcessedAt *time.Time `json:"processed_at" gorm:"column:processed_at"`
}

func (ImageEnrichQueueItem) TableName() string { return "image_enrich_queue" }
