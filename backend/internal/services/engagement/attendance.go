package engagement

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// AttendanceService handles show attendance (going/interested) operations.
// It wraps the generic user_bookmarks table with attendance-specific logic,
// ensuring a user can only have one status per show (going XOR interested).
type AttendanceService struct {
	db *gorm.DB
}

// NewAttendanceService creates a new attendance service.
func NewAttendanceService(database *gorm.DB) *AttendanceService {
	if database == nil {
		database = db.GetDB()
	}
	return &AttendanceService{db: database}
}

// SetAttendance sets the user's attendance status for a show.
// status must be "going", "interested", or "" (to clear).
// Setting "going" removes any existing "interested" and vice versa (atomic via transaction).
func (s *AttendanceService) SetAttendance(userID, showID uint, status string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if status != string(models.BookmarkActionGoing) && status != string(models.BookmarkActionInterested) && status != "" {
		return fmt.Errorf("invalid attendance status: %s", status)
	}

	// Empty status means clear both
	if status == "" {
		return s.RemoveAttendance(userID, showID)
	}

	// Check that the show exists
	var show models.Show
	if err := s.db.First(&show, showID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("show not found")
		}
		return fmt.Errorf("failed to verify show: %w", err)
	}

	// Determine which status to set and which to remove
	setAction := models.BookmarkAction(status)
	var removeAction models.BookmarkAction
	if setAction == models.BookmarkActionGoing {
		removeAction = models.BookmarkActionInterested
	} else {
		removeAction = models.BookmarkActionGoing
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Remove the opposite status (if any) — ignore "not found"
		tx.Where(
			"user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			userID, models.BookmarkEntityShow, showID, removeAction,
		).Delete(&models.UserBookmark{})

		// Upsert the desired status
		bookmark := models.UserBookmark{
			UserID:     userID,
			EntityType: models.BookmarkEntityShow,
			EntityID:   showID,
			Action:     setAction,
			CreatedAt:  time.Now().UTC(),
		}
		return tx.Where(models.UserBookmark{
			UserID:     userID,
			EntityType: models.BookmarkEntityShow,
			EntityID:   showID,
			Action:     setAction,
		}).Assign(models.UserBookmark{
			CreatedAt: bookmark.CreatedAt,
		}).FirstOrCreate(&bookmark).Error
	})
}

// RemoveAttendance removes both going and interested bookmarks for a user+show.
func (s *AttendanceService) RemoveAttendance(userID, showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ? AND action IN ?",
		userID, models.BookmarkEntityShow, showID,
		[]models.BookmarkAction{models.BookmarkActionGoing, models.BookmarkActionInterested},
	).Delete(&models.UserBookmark{})

	if result.Error != nil {
		return fmt.Errorf("failed to remove attendance: %w", result.Error)
	}

	return nil
}

// GetUserAttendance returns the user's attendance status for a show.
// Returns "going", "interested", or "" if not attending.
func (s *AttendanceService) GetUserAttendance(userID, showID uint) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	var bookmark models.UserBookmark
	err := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ? AND action IN ?",
		userID, models.BookmarkEntityShow, showID,
		[]models.BookmarkAction{models.BookmarkActionGoing, models.BookmarkActionInterested},
	).First(&bookmark).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", fmt.Errorf("failed to get attendance: %w", err)
	}

	return string(bookmark.Action), nil
}

// GetAttendanceCounts returns the going and interested counts for a show.
func (s *AttendanceService) GetAttendanceCounts(showID uint) (*contracts.AttendanceCountsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	type countRow struct {
		Action string
		Count  int
	}
	var rows []countRow

	err := s.db.Model(&models.UserBookmark{}).
		Select("action, COUNT(*) as count").
		Where("entity_type = ? AND entity_id = ? AND action IN ?",
			models.BookmarkEntityShow, showID,
			[]models.BookmarkAction{models.BookmarkActionGoing, models.BookmarkActionInterested},
		).
		Group("action").
		Find(&rows).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get attendance counts: %w", err)
	}

	resp := &contracts.AttendanceCountsResponse{ShowID: showID}
	for _, row := range rows {
		switch models.BookmarkAction(row.Action) {
		case models.BookmarkActionGoing:
			resp.GoingCount = row.Count
		case models.BookmarkActionInterested:
			resp.InterestedCount = row.Count
		}
	}

	return resp, nil
}

// GetBatchAttendanceCounts returns attendance counts for multiple shows efficiently.
func (s *AttendanceService) GetBatchAttendanceCounts(showIDs []uint) (map[uint]*contracts.AttendanceCountsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[uint]*contracts.AttendanceCountsResponse)
	if len(showIDs) == 0 {
		return result, nil
	}

	// Initialize all requested show IDs in the result map
	for _, id := range showIDs {
		result[id] = &contracts.AttendanceCountsResponse{ShowID: id}
	}

	type countRow struct {
		EntityID uint
		Action   string
		Count    int
	}
	var rows []countRow

	err := s.db.Model(&models.UserBookmark{}).
		Select("entity_id, action, COUNT(*) as count").
		Where("entity_type = ? AND entity_id IN ? AND action IN ?",
			models.BookmarkEntityShow, showIDs,
			[]models.BookmarkAction{models.BookmarkActionGoing, models.BookmarkActionInterested},
		).
		Group("entity_id, action").
		Find(&rows).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get batch attendance counts: %w", err)
	}

	for _, row := range rows {
		resp, ok := result[row.EntityID]
		if !ok {
			continue
		}
		switch models.BookmarkAction(row.Action) {
		case models.BookmarkActionGoing:
			resp.GoingCount = row.Count
		case models.BookmarkActionInterested:
			resp.InterestedCount = row.Count
		}
	}

	return result, nil
}

// GetBatchUserAttendance returns the user's attendance status for multiple shows.
// The returned map contains show_id -> status ("going", "interested", or absent if none).
func (s *AttendanceService) GetBatchUserAttendance(userID uint, showIDs []uint) (map[uint]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[uint]string)
	if len(showIDs) == 0 {
		return result, nil
	}

	var bookmarks []models.UserBookmark
	err := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id IN ? AND action IN ?",
		userID, models.BookmarkEntityShow, showIDs,
		[]models.BookmarkAction{models.BookmarkActionGoing, models.BookmarkActionInterested},
	).Find(&bookmarks).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get batch user attendance: %w", err)
	}

	for _, b := range bookmarks {
		result[b.EntityID] = string(b.Action)
	}

	return result, nil
}

// GetUserAttendingShows returns shows a user is going to or interested in.
// status filter: "going", "interested", or "all" (default).
// Only returns upcoming approved shows, ordered by event_date ASC.
func (s *AttendanceService) GetUserAttendingShows(userID uint, status string, limit, offset int) ([]*contracts.AttendingShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Build the bookmark filter
	actions := []models.BookmarkAction{models.BookmarkActionGoing, models.BookmarkActionInterested}
	if status == string(models.BookmarkActionGoing) {
		actions = []models.BookmarkAction{models.BookmarkActionGoing}
	} else if status == string(models.BookmarkActionInterested) {
		actions = []models.BookmarkAction{models.BookmarkActionInterested}
	}

	now := time.Now().UTC()

	// Count total matching shows
	var total int64
	err := s.db.Model(&models.UserBookmark{}).
		Joins("JOIN shows ON shows.id = user_bookmarks.entity_id").
		Where("user_bookmarks.user_id = ? AND user_bookmarks.entity_type = ? AND user_bookmarks.action IN ?",
			userID, models.BookmarkEntityShow, actions).
		Where("shows.status = ? AND shows.event_date >= ?", models.ShowStatusApproved, now).
		Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count attending shows: %w", err)
	}

	if total == 0 {
		return []*contracts.AttendingShowResponse{}, 0, nil
	}

	// Query bookmarks joined with shows and first venue
	type attendingRow struct {
		ShowID    uint
		Title     string
		Slug      *string
		EventDate time.Time
		Action    string
		City      *string
		State     *string
		VenueName *string
		VenueSlug *string
	}

	var rows []attendingRow
	err = s.db.
		Table("user_bookmarks").
		Select(`
			shows.id as show_id,
			shows.title,
			shows.slug,
			shows.event_date,
			user_bookmarks.action,
			shows.city,
			shows.state,
			venues.name as venue_name,
			venues.slug as venue_slug
		`).
		Joins("JOIN shows ON shows.id = user_bookmarks.entity_id").
		Joins("LEFT JOIN show_venues ON show_venues.show_id = shows.id").
		Joins("LEFT JOIN venues ON venues.id = show_venues.venue_id").
		Where("user_bookmarks.user_id = ? AND user_bookmarks.entity_type = ? AND user_bookmarks.action IN ?",
			userID, models.BookmarkEntityShow, actions).
		Where("shows.status = ? AND shows.event_date >= ?", models.ShowStatusApproved, now).
		Order("shows.event_date ASC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get attending shows: %w", err)
	}

	// Deduplicate rows by show ID (a show may have multiple venues)
	// Keep the first venue encountered for each show
	seen := make(map[uint]bool)
	responses := make([]*contracts.AttendingShowResponse, 0, len(rows))
	for _, row := range rows {
		if seen[row.ShowID] {
			continue
		}
		seen[row.ShowID] = true

		slug := ""
		if row.Slug != nil {
			slug = *row.Slug
		}

		responses = append(responses, &contracts.AttendingShowResponse{
			ShowID:    row.ShowID,
			Title:     row.Title,
			Slug:      slug,
			EventDate: row.EventDate,
			Status:    row.Action,
			VenueName: row.VenueName,
			VenueSlug: row.VenueSlug,
			City:      row.City,
			State:     row.State,
		})
	}

	return responses, total, nil
}
