package admin

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/utils"
)

// ErrFeaturedSlotNotFound is returned when no active slot exists for the
// requested slot_type, or no row matches a more specific lookup. Allows
// the handler to distinguish "no active pick" (which is a 404 — the
// slot simply isn't curated yet) from real I/O errors.
var ErrFeaturedSlotNotFound = errors.New("featured slot not found")

// FeaturedSlotService manages the admin-curated /explore editorial slots.
// One row per slot_type is active at any time (active_until IS NULL);
// activation atomically retires the previous active row inside a
// transaction so the partial unique index always holds.
//
// The curator_note column is markdown rendered with the shared
// utils.MarkdownRenderer (goldmark + bluemonday) on read. Sanitization
// happens at the rendering boundary, mirroring how CollectionService
// treats Collection.Description.
type FeaturedSlotService struct {
	db *gorm.DB
	md *utils.MarkdownRenderer
}

// NewFeaturedSlotService creates a new featured-slot service. The DB
// handle falls back to the package singleton so older test paths using
// the bare-struct literal still resolve a connection.
func NewFeaturedSlotService(database *gorm.DB) *FeaturedSlotService {
	if database == nil {
		database = db.GetDB()
	}
	return &FeaturedSlotService{
		db: database,
		md: utils.NewMarkdownRenderer(),
	}
}

// GetActiveSlot returns the currently active row for slot_type (the one
// with active_until IS NULL), preloading the curator. Returns
// (nil, ErrFeaturedSlotNotFound) when the slot has never been set or is
// currently retired without replacement; callers map that to HTTP 404.
func (s *FeaturedSlotService) GetActiveSlot(slotType string) (*adminm.FeaturedSlot, error) {
	if !adminm.IsValidFeaturedSlotType(slotType) {
		return nil, fmt.Errorf("invalid slot type: %s", slotType)
	}

	var slot adminm.FeaturedSlot
	err := s.db.Preload("Creator").
		Where("slot_type = ? AND active_until IS NULL", slotType).
		First(&slot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFeaturedSlotNotFound
		}
		return nil, fmt.Errorf("failed to get active featured slot: %w", err)
	}
	return &slot, nil
}

// ListRecent returns the last `limit` rows for slot_type ordered by
// created_at DESC, including retired and currently-active entries. Used
// by the admin "recent picks" listing so curators can see what they've
// shipped without trawling the audit log.
func (s *FeaturedSlotService) ListRecent(slotType string, limit int) ([]adminm.FeaturedSlot, error) {
	if !adminm.IsValidFeaturedSlotType(slotType) {
		return nil, fmt.Errorf("invalid slot type: %s", slotType)
	}
	if limit <= 0 {
		limit = 10
	}

	var slots []adminm.FeaturedSlot
	err := s.db.Preload("Creator").
		Where("slot_type = ?", slotType).
		Order("created_at DESC").
		Limit(limit).
		Find(&slots).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list featured slots: %w", err)
	}
	return slots, nil
}

// SetActiveSlot atomically retires the currently active row (if any)
// for slot_type by setting its active_until to NOW(), then inserts a
// new active row. Wrapped in a transaction so the partial unique index
// (slot_type WHERE active_until IS NULL) never sees two active rows in
// the same window — a concurrent set on the same slot_type fails fast
// with a 23505 unique violation rather than corrupting state.
//
// curatorNote is stored verbatim (markdown source); rendering happens at
// the handler / response boundary via RenderCuratorNote.
func (s *FeaturedSlotService) SetActiveSlot(slotType string, entityID uint, curatorNote *string, userID uint) (*adminm.FeaturedSlot, error) {
	if !adminm.IsValidFeaturedSlotType(slotType) {
		return nil, fmt.Errorf("invalid slot type: %s", slotType)
	}
	if entityID == 0 {
		return nil, fmt.Errorf("entity_id is required")
	}
	if userID == 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	if curatorNote != nil && len(*curatorNote) > adminm.MaxFeaturedSlotCuratorNoteLength {
		return nil, fmt.Errorf("curator_note exceeds maximum length of %d characters", adminm.MaxFeaturedSlotCuratorNoteLength)
	}

	var created adminm.FeaturedSlot
	err := s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		// Retire the currently active row, if any. Update returns 0
		// rows affected on first-ever set, which is fine.
		if err := tx.Model(&adminm.FeaturedSlot{}).
			Where("slot_type = ? AND active_until IS NULL", slotType).
			Update("active_until", now).Error; err != nil {
			return fmt.Errorf("failed to retire active featured slot: %w", err)
		}

		created = adminm.FeaturedSlot{
			SlotType:    slotType,
			EntityID:    entityID,
			CuratorNote: curatorNote,
			ActiveFrom:  now,
			CreatedBy:   userID,
		}
		if err := tx.Create(&created).Error; err != nil {
			return fmt.Errorf("failed to create featured slot: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Re-fetch with Creator preloaded so the response carries the same
	// shape as GetActiveSlot.
	return s.GetActiveSlot(slotType)
}

// RetireActiveSlot retires the currently active row for slot_type
// without replacement. Returns ErrFeaturedSlotNotFound if no row is
// currently active. The slot then has no active row until a curator
// sets a new one via SetActiveSlot.
func (s *FeaturedSlotService) RetireActiveSlot(slotType string, userID uint) error {
	if !adminm.IsValidFeaturedSlotType(slotType) {
		return fmt.Errorf("invalid slot type: %s", slotType)
	}
	if userID == 0 {
		return fmt.Errorf("user_id is required")
	}

	now := time.Now().UTC()
	res := s.db.Model(&adminm.FeaturedSlot{}).
		Where("slot_type = ? AND active_until IS NULL", slotType).
		Update("active_until", now)
	if res.Error != nil {
		return fmt.Errorf("failed to retire featured slot: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrFeaturedSlotNotFound
	}
	return nil
}

// RenderCuratorNote returns the sanitized HTML for a slot's curator
// note, or "" when the note is empty or nil. Centralised here so the
// admin handler and any future read-side consumer share one rendering
// path — the markdown source on disk is always the source of truth and
// sanitization is applied on every render.
func (s *FeaturedSlotService) RenderCuratorNote(note *string) string {
	if note == nil || *note == "" {
		return ""
	}
	if s.md == nil {
		s.md = utils.NewMarkdownRenderer()
	}
	return s.md.Render(*note)
}
