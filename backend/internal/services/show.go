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
}

// ShowResponse represents the show data returned to clients
type ShowResponse struct {
	ID             uint             `json:"id"`
	Title          string           `json:"title"`
	EventDate      time.Time        `json:"event_date"`
	City           *string          `json:"city"`
	State          *string          `json:"state"`
	Price          *float64         `json:"price"`
	AgeRequirement *string          `json:"age_requirement"`
	Description    *string          `json:"description"`
	Venues         []VenueResponse  `json:"venues"`
	Artists        []ArtistResponse `json:"artists"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// VenueResponse represents venue data in show responses
type VenueResponse struct {
	ID      uint    `json:"id"`
	Name    string  `json:"name"`
	Address *string `json:"address"`
	City    string  `json:"city"`
	State   string  `json:"state"`
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

		// Create the show
		show := &models.Show{
			Title:          req.Title,
			EventDate:      req.EventDate.UTC(), // Ensure UTC storage
			City:           &req.City,
			State:          &req.State,
			Price:          req.Price,
			AgeRequirement: &req.AgeRequirement,
			Description:    &req.Description,
		}

		if err := tx.Create(show).Error; err != nil {
			return fmt.Errorf("failed to create show: %w", err)
		}

		// Associate venues
		venues, err := s.associateVenues(tx, show.ID, req.Venues)
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
			ID:             show.ID,
			Title:          show.Title,
			EventDate:      show.EventDate,
			City:           show.City,
			State:          show.State,
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			Description:    show.Description,
			Venues:         venues,
			Artists:        artists,
			CreatedAt:      show.CreatedAt,
			UpdatedAt:      show.UpdatedAt,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
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

// UpdateShow updates an existing show
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
// Returns shows, next cursor (nil if no more), and error.
func (s *ShowService) GetUpcomingShows(timezone string, cursor string, limit int) ([]*ShowResponse, *string, error) {
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

// associateVenues associates venues with a show, creating new venues if needed.
// Uses VenueService to ensure consistent venue creation logic.
func (s *ShowService) associateVenues(tx *gorm.DB, showID uint, requestVenues []CreateShowVenue) ([]VenueResponse, error) {
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
				nil, // zipcode
				tx,  // use transaction
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
			ID:      venue.ID,
			Name:    venue.Name,
			Address: venue.Address,
			City:    venue.City,
			State:   venue.State,
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
			ID:      venue.ID,
			Name:    venue.Name,
			Address: venue.Address,
			City:    venue.City,
			State:   venue.State,
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
		ID:             show.ID,
		Title:          show.Title,
		EventDate:      show.EventDate,
		City:           show.City,
		State:          show.State,
		Price:          show.Price,
		AgeRequirement: show.AgeRequirement,
		Description:    show.Description,
		Venues:         venues,
		Artists:        artists,
		CreatedAt:      show.CreatedAt,
		UpdatedAt:      show.UpdatedAt,
	}
}
