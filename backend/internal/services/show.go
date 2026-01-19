package services

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// ShowService handles show-related business logic
type ShowService struct {
	db *gorm.DB
}

// NewShowService creates a new show service
func NewShowService() *ShowService {
	return &ShowService{
		db: db.GetDB(),
	}
}

// CreateShowVenue represents a venue in a show creation request.
type CreateShowVenue struct {
	ID      *uint  `json:"id"`
	Name    string `json:"name"`
	City    string `json:"city"`
	State   string `json:"state"`
	Address string `json:"address,omitempty"`
}

// CreateShowArtist represents an artist in a show creation request.
// IsHeadliner is used for duplicate prevention (headliners can't perform at same venue on same date).
type CreateShowArtist struct {
	ID          *uint  `json:"id"`
	Name        string `json:"name"`
	IsHeadliner *bool  `json:"is_headliner"`
}

// CreateShowRequest represents the data needed to create a new show.
// The service will prevent duplicate headliners at the same venue on the same date/time
// and reuse existing venues by name and city (venues are unique by name within a city).
type CreateShowRequest struct {
	Title          string             `json:"title" validate:"required"`
	EventDate      time.Time          `json:"event_date" validate:"required"`
	City           string             `json:"city"`
	State          string             `json:"state"`
	Price          *float64           `json:"price"`
	AgeRequirement string             `json:"age_requirement"`
	Description    string             `json:"description"`
	Venues         []CreateShowVenue  `json:"venues" validate:"required,min=1"`
	Artists        []CreateShowArtist `json:"artists" validate:"required,min=1"`

	// User context for determining show status
	SubmittedByUserID *uint `json:"-"` // User ID of submitter (set by handler)
	SubmitterIsAdmin  bool  `json:"-"` // Whether submitter is admin (set by handler)
	IsPrivate         bool  `json:"-"` // Whether show should be private (user's list only)
}

// ShowResponse represents the show data returned to clients
type ShowResponse struct {
	ID              uint             `json:"id"`
	Title           string           `json:"title"`
	EventDate       time.Time        `json:"event_date"`
	City            *string          `json:"city"`
	State           *string          `json:"state"`
	Price           *float64         `json:"price"`
	AgeRequirement  *string          `json:"age_requirement"`
	Description     *string          `json:"description"`
	Status          string           `json:"status"`
	SubmittedBy     *uint            `json:"submitted_by,omitempty"`
	RejectionReason *string          `json:"rejection_reason,omitempty"`
	Venues          []VenueResponse  `json:"venues"`
	Artists         []ArtistResponse `json:"artists"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// VenueResponse represents venue data in show responses
type VenueResponse struct {
	ID       uint    `json:"id"`
	Name     string  `json:"name"`
	Address  *string `json:"address"`
	City     string  `json:"city"`
	State    string  `json:"state"`
	Verified bool    `json:"verified"` // Admin-verified as legitimate venue
}

// ShowArtistSocials represents social media links for artists in show responses
type ShowArtistSocials struct {
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// ArtistResponse represents artist data in show responses
type ArtistResponse struct {
	ID          uint              `json:"id"`
	Name        string            `json:"name"`
	State       *string           `json:"state"`
	City        *string           `json:"city"`
	IsHeadliner *bool             `json:"is_headliner"`
	IsNewArtist *bool             `json:"is_new_artist"`
	Socials     ShowArtistSocials `json:"socials"`
}

// CreateShow creates a new show with associated venues and artists.
// Prevents duplicate headliners at the same venue on the same date/time.
// Prevents duplicate venues with the same name in the same city.
// Status is determined based on venue verification and submitter admin status.
func (s *ShowService) CreateShow(req *CreateShowRequest) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Use transaction for data consistency
	var response *ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Check for duplicate headliner-venue-date conflicts
		if err := s.checkDuplicateHeadlinerConflicts(tx, req); err != nil {
			return err
		}

		// Determine show status based on venue verification and privacy preference
		status := s.determineShowStatus(tx, req.Venues, req.SubmitterIsAdmin, req.IsPrivate)

		// Create the show
		show := &models.Show{
			Title:          req.Title,
			EventDate:      req.EventDate.UTC(), // Ensure UTC storage
			City:           &req.City,
			State:          &req.State,
			Price:          req.Price,
			AgeRequirement: &req.AgeRequirement,
			Description:    &req.Description,
			Status:         status,
			SubmittedBy:    req.SubmittedByUserID,
		}

		if err := tx.Create(show).Error; err != nil {
			return fmt.Errorf("failed to create show: %w", err)
		}

		// Associate venues (pass admin status for venue verification)
		venues, err := s.associateVenues(tx, show.ID, req.Venues, req.SubmitterIsAdmin)
		if err != nil {
			return fmt.Errorf("failed to associate venues: %w", err)
		}

		// Associate artists
		artists, err := s.associateArtists(tx, show.ID, req.Artists)
		if err != nil {
			return fmt.Errorf("failed to associate artists: %w", err)
		}

		// Build response
		response = &ShowResponse{
			ID:              show.ID,
			Title:           show.Title,
			EventDate:       show.EventDate,
			City:            show.City,
			State:           show.State,
			Price:           show.Price,
			AgeRequirement:  show.AgeRequirement,
			Description:     show.Description,
			Status:          string(show.Status),
			SubmittedBy:     show.SubmittedBy,
			RejectionReason: show.RejectionReason,
			Venues:          venues,
			Artists:         artists,
			CreatedAt:       show.CreatedAt,
			UpdatedAt:       show.UpdatedAt,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// determineShowStatus determines whether a show should be pending, approved, or private.
// Admins always get approved status. Non-admins get pending/private if any venue is unverified.
// If isPrivate is true and venue is unverified, the show is private (user's list only).
func (s *ShowService) determineShowStatus(tx *gorm.DB, venues []CreateShowVenue, isAdmin bool, isPrivate bool) models.ShowStatus {
	// Admins bypass the pending workflow - their shows are always approved
	if isAdmin {
		return models.ShowStatusApproved
	}

	// Check if any venue is unverified
	hasUnverifiedVenue := false
	for _, v := range venues {
		if v.ID == nil {
			// New venue (no ID) = will be created as unverified
			hasUnverifiedVenue = true
			break
		}

		// Check if existing venue is verified
		var venue models.Venue
		if err := tx.First(&venue, *v.ID).Error; err == nil {
			if !venue.Verified {
				hasUnverifiedVenue = true
				break
			}
		}
	}

	// If there's an unverified venue, determine if private or pending
	if hasUnverifiedVenue {
		if isPrivate {
			return models.ShowStatusPrivate
		}
		return models.ShowStatusPending
	}

	// All venues are verified - show is approved
	return models.ShowStatusApproved
}

// checkDuplicateHeadlinerConflicts checks if any headliners are already performing
// at the same venue on the same date/time
func (s *ShowService) checkDuplicateHeadlinerConflicts(tx *gorm.DB, req *CreateShowRequest) error {
	// Get all headliners from the request
	var headlinerNames []string
	for _, artist := range req.Artists {
		if artist.IsHeadliner != nil && *artist.IsHeadliner {
			headlinerNames = append(headlinerNames, artist.Name)
		}
	}

	// If no headliners, no conflicts possible
	if len(headlinerNames) == 0 {
		return nil
	}

	// Get all venue names from the request
	var venueNames []string
	for _, venue := range req.Venues {
		venueNames = append(venueNames, venue.Name)
	}

	// Check for conflicts: same headliner + same venue + same date
	for _, headlinerName := range headlinerNames {
		for _, venueName := range venueNames {
			var existingShows []models.Show

			// Query for shows on the same date with the same headliner and venue
			err := tx.Table("shows").
				Joins("JOIN show_artists ON shows.id = show_artists.show_id").
				Joins("JOIN artists ON show_artists.artist_id = artists.id").
				Joins("JOIN show_venues ON shows.id = show_venues.show_id").
				Joins("JOIN venues ON show_venues.venue_id = venues.id").
				Where("artists.name = ? AND venues.name = ? AND shows.event_date = ? AND show_artists.set_type = ?",
					headlinerName, venueName, req.EventDate.UTC(), "headliner").
				Find(&existingShows).Error

			if err != nil {
				return fmt.Errorf("failed to check for duplicate headliner conflicts: %w", err)
			}

			if len(existingShows) > 0 {
				return fmt.Errorf("headliner '%s' is already performing at venue '%s' on %s",
					headlinerName, venueName, req.EventDate.Format("2006-01-02 15:04:05 UTC"))
			}
		}
	}

	return nil
}

// GetShow retrieves a show by ID with all associations
func (s *ShowService) GetShow(showID uint) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var show models.Show
	err := s.db.Preload("Venues").Preload("Artists").First(&show, showID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("show not found")
		}
		return nil, fmt.Errorf("failed to get show: %w", err)
	}

	return s.buildShowResponse(&show), nil
}

// GetShows retrieves shows with optional filtering
func (s *ShowService) GetShows(filters map[string]interface{}) ([]*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Preload("Venues").Preload("Artists")

	// Apply filters
	if city, ok := filters["city"].(string); ok && city != "" {
		query = query.Where("city = ?", city)
	}
	if state, ok := filters["state"].(string); ok && state != "" {
		query = query.Where("state = ?", state)
	}
	if fromDate, ok := filters["from_date"].(time.Time); ok {
		query = query.Where("event_date >= ?", fromDate.UTC())
	}
	if toDate, ok := filters["to_date"].(time.Time); ok {
		query = query.Where("event_date <= ?", toDate.UTC())
	}

	// Default ordering by event date
	query = query.Order("event_date ASC")

	var shows []models.Show
	err := query.Find(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get shows: %w", err)
	}

	// Build responses
	responses := make([]*ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, nil
}

// GetUserSubmissions returns all shows submitted by a specific user
func (s *ShowService) GetUserSubmissions(userID uint, limit, offset int) ([]ShowResponse, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count first
	var total int64
	if err := s.db.Model(&models.Show{}).Where("submitted_by = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user submissions: %w", err)
	}

	// Query shows with pagination
	var shows []models.Show
	err := s.db.Preload("Venues").Preload("Artists").
		Where("submitted_by = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user submissions: %w", err)
	}

	// Build responses
	responses := make([]ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = *s.buildShowResponse(&show)
	}

	return responses, int(total), nil
}

// UpdateShow updates an existing show (basic fields only)
func (s *ShowService) UpdateShow(showID uint, updates map[string]interface{}) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Handle event_date conversion to UTC if present
	if eventDate, ok := updates["event_date"].(time.Time); ok {
		updates["event_date"] = eventDate.UTC()
	}

	err := s.db.Model(&models.Show{}).Where("id = ?", showID).Updates(updates).Error
	if err != nil {
		return nil, fmt.Errorf("failed to update show: %w", err)
	}

	return s.GetShow(showID)
}

// UpdateShowWithRelations updates a show including its artist and venue associations.
// If venues or artists slices are provided (non-nil), they replace the existing associations.
// If nil, the existing associations are preserved.
// If isAdmin is true, new venues created during update are automatically verified.
func (s *ShowService) UpdateShowWithRelations(
	showID uint,
	updates map[string]interface{},
	venues []CreateShowVenue,
	artists []CreateShowArtist,
	isAdmin bool,
) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Handle event_date conversion to UTC if present
	if eventDate, ok := updates["event_date"].(time.Time); ok {
		updates["event_date"] = eventDate.UTC()
	}

	var response *ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// First, verify the show exists
		var show models.Show
		if err := tx.First(&show, showID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("show not found")
			}
			return fmt.Errorf("failed to find show: %w", err)
		}

		// Update basic show fields if any updates provided
		if len(updates) > 0 {
			if err := tx.Model(&show).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update show: %w", err)
			}
			// Reload show to get updated values
			if err := tx.First(&show, showID).Error; err != nil {
				return fmt.Errorf("failed to reload show: %w", err)
			}
		}

		// Update venue associations if venues are provided
		var venueResponses []VenueResponse
		if venues != nil {
			// Delete existing show-venue associations
			if err := tx.Where("show_id = ?", showID).Delete(&models.ShowVenue{}).Error; err != nil {
				return fmt.Errorf("failed to delete existing show venues: %w", err)
			}

			// Create new associations (pass admin status for venue verification)
			var err error
			venueResponses, err = s.associateVenues(tx, showID, venues, isAdmin)
			if err != nil {
				return fmt.Errorf("failed to associate venues: %w", err)
			}
		}

		// Update artist associations if artists are provided
		var artistResponses []ArtistResponse
		if artists != nil {
			// Delete existing show-artist associations
			if err := tx.Where("show_id = ?", showID).Delete(&models.ShowArtist{}).Error; err != nil {
				return fmt.Errorf("failed to delete existing show artists: %w", err)
			}

			// Create new associations
			var err error
			artistResponses, err = s.associateArtists(tx, showID, artists)
			if err != nil {
				return fmt.Errorf("failed to associate artists: %w", err)
			}
		}

		// Build response - need to fetch associations if not updated
		if venues == nil {
			// Fetch existing venues
			var showVenues []models.ShowVenue
			if err := tx.Where("show_id = ?", showID).Find(&showVenues).Error; err != nil {
				return fmt.Errorf("failed to fetch show venues: %w", err)
			}
			for _, sv := range showVenues {
				var venue models.Venue
				if err := tx.First(&venue, sv.VenueID).Error; err == nil {
					venueResponses = append(venueResponses, VenueResponse{
						ID:       venue.ID,
						Name:     venue.Name,
						Address:  venue.Address,
						City:     venue.City,
						State:    venue.State,
						Verified: venue.Verified,
					})
				}
			}
		}

		if artists == nil {
			// Fetch existing artists in order
			var showArtists []models.ShowArtist
			if err := tx.Where("show_id = ?", showID).Order("position ASC").Find(&showArtists).Error; err != nil {
				return fmt.Errorf("failed to fetch show artists: %w", err)
			}
			for _, sa := range showArtists {
				var artist models.Artist
				if err := tx.First(&artist, sa.ArtistID).Error; err == nil {
					isHeadliner := sa.SetType == "headliner"
					isNewArtist := false
					socials := ShowArtistSocials{
						Instagram:  artist.Social.Instagram,
						Facebook:   artist.Social.Facebook,
						Twitter:    artist.Social.Twitter,
						YouTube:    artist.Social.YouTube,
						Spotify:    artist.Social.Spotify,
						SoundCloud: artist.Social.SoundCloud,
						Bandcamp:   artist.Social.Bandcamp,
						Website:    artist.Social.Website,
					}
					artistResponses = append(artistResponses, ArtistResponse{
						ID:          artist.ID,
						Name:        artist.Name,
						State:       artist.State,
						City:        artist.City,
						IsHeadliner: &isHeadliner,
						IsNewArtist: &isNewArtist,
						Socials:     socials,
					})
				}
			}
		}

		response = &ShowResponse{
			ID:              show.ID,
			Title:           show.Title,
			EventDate:       show.EventDate,
			City:            show.City,
			State:           show.State,
			Price:           show.Price,
			AgeRequirement:  show.AgeRequirement,
			Description:     show.Description,
			Status:          string(show.Status),
			SubmittedBy:     show.SubmittedBy,
			RejectionReason: show.RejectionReason,
			Venues:          venueResponses,
			Artists:         artistResponses,
			CreatedAt:       show.CreatedAt,
			UpdatedAt:       show.UpdatedAt,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// encodeCursor creates a cursor from event_date and show ID
func encodeCursor(eventDate time.Time, id uint) string {
	// Format: base64(timestamp_unix_nano:id)
	cursor := fmt.Sprintf("%d:%d", eventDate.UnixNano(), id)
	return base64.URLEncoding.EncodeToString([]byte(cursor))
}

// decodeCursor parses a cursor into event_date and show ID
func decodeCursor(cursor string) (time.Time, uint, error) {
	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor encoding: %w", err)
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid cursor format")
	}

	unixNano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor timestamp: %w", err)
	}

	id, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor id: %w", err)
	}

	return time.Unix(0, unixNano), uint(id), nil
}

// GetUpcomingShows retrieves shows from today onwards in the specified timezone with cursor pagination.
// Includes tonight's shows by filtering from the start of today in the user's timezone.
// If includeNonApproved is false, only approved shows are returned (public view).
// If includeNonApproved is true, all shows are returned including pending/rejected (admin view).
// Returns shows, next cursor (nil if no more), and error.
func (s *ShowService) GetUpcomingShows(timezone string, cursor string, limit int, includeNonApproved bool) ([]*ShowResponse, *string, error) {
	if s.db == nil {
		return nil, nil, fmt.Errorf("database not initialized")
	}

	// Load timezone, default to UTC
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		// Invalid timezone, fall back to UTC
		loc = time.UTC
	}

	// Get start of today in the user's timezone, then convert to UTC for query
	now := time.Now().In(loc)
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	startOfTodayUTC := startOfToday.UTC()

	// Build query
	query := s.db.Preload("Venues").Preload("Artists")

	// Filter by status for non-admin users (public view shows only approved)
	if !includeNonApproved {
		query = query.Where("status = ?", models.ShowStatusApproved)
	} else {
		// For admin view, still exclude private shows (those are personal to the submitter)
		query = query.Where("status != ?", models.ShowStatusPrivate)
	}

	// Apply cursor filter if provided
	if cursor != "" {
		cursorDate, cursorID, err := decodeCursor(cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		// Get shows after the cursor position (same date but higher ID, or later date)
		query = query.Where(
			"(event_date = ? AND id > ?) OR (event_date > ?)",
			cursorDate, cursorID, cursorDate,
		)
	} else {
		// No cursor, start from today
		query = query.Where("event_date >= ?", startOfTodayUTC)
	}

	// Order by event_date ASC, then by ID ASC for stable ordering
	// Fetch one extra to check if there are more results
	query = query.Order("event_date ASC, id ASC").Limit(limit + 1)

	var shows []models.Show
	if err := query.Find(&shows).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to get upcoming shows: %w", err)
	}

	// Check if there are more results
	var nextCursor *string
	if len(shows) > limit {
		// There are more results, create cursor from the last item we'll return
		shows = shows[:limit] // Trim to requested limit
		lastShow := shows[len(shows)-1]
		encoded := encodeCursor(lastShow.EventDate, lastShow.ID)
		nextCursor = &encoded
	}

	// Build responses
	responses := make([]*ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, nextCursor, nil
}

// DeleteShow deletes a show and its associations
func (s *ShowService) DeleteShow(showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete show associations first (cascade will handle this, but being explicit)
		if err := tx.Where("show_id = ?", showID).Delete(&models.ShowVenue{}).Error; err != nil {
			return fmt.Errorf("failed to delete show venues: %w", err)
		}
		if err := tx.Where("show_id = ?", showID).Delete(&models.ShowArtist{}).Error; err != nil {
			return fmt.Errorf("failed to delete show artists: %w", err)
		}

		// Delete the show
		if err := tx.Delete(&models.Show{}, showID).Error; err != nil {
			return fmt.Errorf("failed to delete show: %w", err)
		}

		return nil
	})
}

// GetPendingShows retrieves shows with pending status for admin review.
// Returns shows, total count, and error.
func (s *ShowService) GetPendingShows(limit, offset int) ([]*ShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count
	var total int64
	if err := s.db.Model(&models.Show{}).Where("status = ?", models.ShowStatusPending).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count pending shows: %w", err)
	}

	// Get pending shows with pagination
	var shows []models.Show
	err := s.db.Preload("Venues").Preload("Artists").
		Where("status = ?", models.ShowStatusPending).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get pending shows: %w", err)
	}

	// Build responses
	responses := make([]*ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, total, nil
}

// GetRejectedShows retrieves shows with rejected status for admin reference.
// Supports optional search by title or rejection reason.
// Returns shows, total count, and error.
func (s *ShowService) GetRejectedShows(limit, offset int, search string) ([]*ShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Build base query
	baseQuery := s.db.Model(&models.Show{}).Where("status = ?", models.ShowStatusRejected)

	// Add search filter if provided
	if search != "" {
		searchPattern := "%" + search + "%"
		baseQuery = baseQuery.Where("title ILIKE ? OR rejection_reason ILIKE ?", searchPattern, searchPattern)
	}

	// Get total count
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count rejected shows: %w", err)
	}

	// Get rejected shows with pagination
	var shows []models.Show
	err := s.db.Preload("Venues").Preload("Artists").
		Where("status = ?", models.ShowStatusRejected).
		Scopes(func(db *gorm.DB) *gorm.DB {
			if search != "" {
				searchPattern := "%" + search + "%"
				return db.Where("title ILIKE ? OR rejection_reason ILIKE ?", searchPattern, searchPattern)
			}
			return db
		}).
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get rejected shows: %w", err)
	}

	// Build responses
	responses := make([]*ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, total, nil
}

// ApproveShow approves a pending show and optionally verifies its venues.
func (s *ShowService) ApproveShow(showID uint, verifyVenues bool) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show
		var show models.Show
		if err := tx.Preload("Venues").First(&show, showID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("show not found")
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is pending
		if show.Status != models.ShowStatusPending {
			return fmt.Errorf("show is not pending (current status: %s)", show.Status)
		}

		// Update show status to approved
		if err := tx.Model(&show).Update("status", models.ShowStatusApproved).Error; err != nil {
			return fmt.Errorf("failed to approve show: %w", err)
		}

		// Optionally verify the venues
		if verifyVenues {
			for _, venue := range show.Venues {
				if !venue.Verified {
					if err := tx.Model(&venue).Update("verified", true).Error; err != nil {
						return fmt.Errorf("failed to verify venue %d: %w", venue.ID, err)
					}
				}
			}
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// RejectShow rejects a pending show with a reason.
func (s *ShowService) RejectShow(showID uint, reason string) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show
		var show models.Show
		if err := tx.First(&show, showID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("show not found")
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is pending
		if show.Status != models.ShowStatusPending {
			return fmt.Errorf("show is not pending (current status: %s)", show.Status)
		}

		// Update show status to rejected with reason
		updates := map[string]interface{}{
			"status":           models.ShowStatusRejected,
			"rejection_reason": reason,
		}
		if err := tx.Model(&show).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to reject show: %w", err)
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// UnpublishShow changes an approved show's status back to pending.
// Only the submitter or an admin can unpublish a show.
func (s *ShowService) UnpublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show
		var show models.Show
		if err := tx.First(&show, showID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("show not found")
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is approved (can only unpublish approved shows)
		if show.Status != models.ShowStatusApproved {
			return fmt.Errorf("can only unpublish approved shows (current status: %s)", show.Status)
		}

		// Check authorization: user must be the submitter or an admin
		if !isAdmin {
			if show.SubmittedBy == nil || *show.SubmittedBy != userID {
				return fmt.Errorf("only the show submitter or an admin can unpublish this show")
			}
		}

		// Update show status to private
		if err := tx.Model(&show).Update("status", models.ShowStatusPrivate).Error; err != nil {
			return fmt.Errorf("failed to unpublish show: %w", err)
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// MakePrivateShow changes a pending show's status to private.
// Only the submitter or an admin can make a show private.
func (s *ShowService) MakePrivateShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show
		var show models.Show
		if err := tx.First(&show, showID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("show not found")
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is pending (can only make private from pending status)
		if show.Status != models.ShowStatusPending {
			return fmt.Errorf("can only make pending shows private (current status: %s)", show.Status)
		}

		// Check authorization: user must be the submitter or an admin
		if !isAdmin {
			if show.SubmittedBy == nil || *show.SubmittedBy != userID {
				return fmt.Errorf("only the show submitter or an admin can make this show private")
			}
		}

		// Update show status to private
		if err := tx.Model(&show).Update("status", models.ShowStatusPrivate).Error; err != nil {
			return fmt.Errorf("failed to make show private: %w", err)
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// PublishShow changes a private show's status to approved or pending.
// If all venues are verified, status becomes approved.
// If any venue is unverified, status becomes pending.
// Only the submitter or an admin can publish a show.
func (s *ShowService) PublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show with venues preloaded
		var show models.Show
		if err := tx.Preload("Venues").First(&show, showID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("show not found")
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is private (can only publish from private status)
		if show.Status != models.ShowStatusPrivate {
			return fmt.Errorf("can only publish private shows (current status: %s)", show.Status)
		}

		// Check authorization: user must be the submitter or an admin
		if !isAdmin {
			if show.SubmittedBy == nil || *show.SubmittedBy != userID {
				return fmt.Errorf("only the show submitter or an admin can publish this show")
			}
		}

		// Determine the new status based on venue verification
		// If all venues are verified, set to approved; otherwise set to pending
		newStatus := models.ShowStatusApproved
		for _, venue := range show.Venues {
			if !venue.Verified {
				newStatus = models.ShowStatusPending
				break
			}
		}

		// Update show status
		if err := tx.Model(&show).Update("status", newStatus).Error; err != nil {
			return fmt.Errorf("failed to publish show: %w", err)
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// associateVenues associates venues with a show, creating new venues if needed.
// Uses VenueService to ensure consistent venue creation logic.
// If isAdmin is true, new venues are automatically verified.
func (s *ShowService) associateVenues(tx *gorm.DB, showID uint, requestVenues []CreateShowVenue, isAdmin bool) ([]VenueResponse, error) {
	var venues []VenueResponse

	// Create venue service for venue operations
	venueService := NewVenueService()

	for _, requestVenue := range requestVenues {
		var venue *models.Venue
		var err error

		// If ID is provided, try to find existing venue by ID
		if requestVenue.ID != nil {
			var venueModel models.Venue
			err = tx.First(&venueModel, *requestVenue.ID).Error
			if err == gorm.ErrRecordNotFound {
				return nil, fmt.Errorf("venue with ID %d not found", *requestVenue.ID)
			} else if err != nil {
				return nil, fmt.Errorf("failed to find venue with ID %d: %w", *requestVenue.ID, err)
			}
			venue = &venueModel
		} else {
			// No ID provided, use VenueService to find or create venue (name unique per city)
			// VenueService will validate required fields
			var addressPtr *string
			if requestVenue.Address != "" {
				addressPtr = &requestVenue.Address
			}

			venue, err = venueService.FindOrCreateVenue(
				requestVenue.Name,
				requestVenue.City,
				requestVenue.State,
				addressPtr,
				nil,    // zipcode
				tx,     // use transaction
				isAdmin, // pass admin status for venue verification
			)
			if err != nil {
				return nil, fmt.Errorf("failed to find or create venue: %w", err)
			}
		}

		// Create show-venue association
		showVenue := models.ShowVenue{
			ShowID:  showID,
			VenueID: venue.ID,
		}
		if err := tx.Create(&showVenue).Error; err != nil {
			return nil, fmt.Errorf("failed to create show-venue association: %w", err)
		}

		venues = append(venues, VenueResponse{
			ID:       venue.ID,
			Name:     venue.Name,
			Address:  venue.Address,
			City:     venue.City,
			State:    venue.State,
			Verified: venue.Verified,
		})
	}

	return venues, nil
}

// associateArtists associates artists with a show, creating new artists if needed
func (s *ShowService) associateArtists(tx *gorm.DB, showID uint, requestArtists []CreateShowArtist) ([]ArtistResponse, error) {
	var artists []ArtistResponse

	for position, requestArtist := range requestArtists {
		var artist models.Artist
		var err error
		isNewArtist := false

		// If ID is provided, try to find existing artist by ID
		if requestArtist.ID != nil {
			err = tx.First(&artist, *requestArtist.ID).Error
			if err == gorm.ErrRecordNotFound {
				return nil, fmt.Errorf("artist with ID %d not found", *requestArtist.ID)
			} else if err != nil {
				return nil, fmt.Errorf("failed to find artist with ID %d: %w", *requestArtist.ID, err)
			}
		} else {
			// No ID provided, use name to find or create artist
			if requestArtist.Name == "" {
				return nil, fmt.Errorf("either ID or Name must be provided for artist")
			}

			err = tx.Where("LOWER(name) = LOWER(?)", requestArtist.Name).First(&artist).Error
			if err == gorm.ErrRecordNotFound {
				// Create new artist
				artist = models.Artist{
					Name: requestArtist.Name,
				}
				if err := tx.Create(&artist).Error; err != nil {
					return nil, fmt.Errorf("failed to create artist %s: %w", requestArtist.Name, err)
				}
				isNewArtist = true
			} else if err != nil {
				return nil, fmt.Errorf("failed to find artist %s: %w", requestArtist.Name, err)
			}
		}

		// Determine set type and IsHeadliner flag
		setType := "opener"
		isHeadliner := false
		if requestArtist.IsHeadliner != nil && *requestArtist.IsHeadliner {
			setType = "headliner"
			isHeadliner = true
		} else if requestArtist.IsHeadliner == nil && position == 0 {
			// Fallback: first artist is headliner if not explicitly specified
			setType = "headliner"
			isHeadliner = true
		}

		// Create show-artist association with position
		showArtist := models.ShowArtist{
			ShowID:   showID,
			ArtistID: artist.ID,
			Position: position,
			SetType:  setType,
		}
		if err := tx.Create(&showArtist).Error; err != nil {
			return nil, fmt.Errorf("failed to create show-artist association: %w", err)
		}

		// Convert artist socials to response format
		socials := ShowArtistSocials{
			Instagram:  artist.Social.Instagram,
			Facebook:   artist.Social.Facebook,
			Twitter:    artist.Social.Twitter,
			YouTube:    artist.Social.YouTube,
			Spotify:    artist.Social.Spotify,
			SoundCloud: artist.Social.SoundCloud,
			Bandcamp:   artist.Social.Bandcamp,
			Website:    artist.Social.Website,
		}

		artists = append(artists, ArtistResponse{
			ID:          artist.ID,
			Name:        artist.Name,
			State:       artist.State,
			City:        artist.City,
			IsHeadliner: &isHeadliner,
			IsNewArtist: &isNewArtist,
			Socials:     socials,
		})
	}

	return artists, nil
}

// buildShowResponse converts a Show model to ShowResponse
func (s *ShowService) buildShowResponse(show *models.Show) *ShowResponse {
	// Build venue responses
	venues := make([]VenueResponse, len(show.Venues))
	for i, venue := range show.Venues {
		venues[i] = VenueResponse{
			ID:       venue.ID,
			Name:     venue.Name,
			Address:  venue.Address,
			City:     venue.City,
			State:    venue.State,
			Verified: venue.Verified,
		}
	}

	// Build artist responses (need to get ordered artists)
	artists := make([]ArtistResponse, 0, len(show.Artists))

	// Get ordered artists from show_artists table
	var showArtists []models.ShowArtist
	s.db.Where("show_id = ?", show.ID).Order("position ASC").Find(&showArtists)

	for _, sa := range showArtists {
		// Find the artist
		var artist models.Artist
		if err := s.db.First(&artist, sa.ArtistID).Error; err == nil {
			// Convert artist socials to response format
			socials := ShowArtistSocials{
				Instagram:  artist.Social.Instagram,
				Facebook:   artist.Social.Facebook,
				Twitter:    artist.Social.Twitter,
				YouTube:    artist.Social.YouTube,
				Spotify:    artist.Social.Spotify,
				SoundCloud: artist.Social.SoundCloud,
				Bandcamp:   artist.Social.Bandcamp,
				Website:    artist.Social.Website,
			}

			// Determine if this artist is a headliner
			isHeadliner := sa.SetType == "headliner"

			// For existing shows, we can't determine if the artist was "new" at creation time
			// So we'll set this to false for all existing artists
			isNewArtist := false

			artists = append(artists, ArtistResponse{
				ID:          artist.ID,
				Name:        artist.Name,
				State:       artist.State,
				City:        artist.City,
				IsHeadliner: &isHeadliner,
				IsNewArtist: &isNewArtist,
				Socials:     socials,
			})
		}
	}

	return &ShowResponse{
		ID:              show.ID,
		Title:           show.Title,
		EventDate:       show.EventDate,
		City:            show.City,
		State:           show.State,
		Price:           show.Price,
		AgeRequirement:  show.AgeRequirement,
		Description:     show.Description,
		Status:          string(show.Status),
		SubmittedBy:     show.SubmittedBy,
		RejectionReason: show.RejectionReason,
		Venues:          venues,
		Artists:         artists,
		CreatedAt:       show.CreatedAt,
		UpdatedAt:       show.UpdatedAt,
	}
}
