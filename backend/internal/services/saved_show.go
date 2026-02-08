package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
)

// SavedShowService handles saved show business logic
type SavedShowService struct {
	db *gorm.DB
}

// NewSavedShowService creates a new saved show service
func NewSavedShowService(database *gorm.DB) *SavedShowService {
	if database == nil {
		database = db.GetDB()
	}
	return &SavedShowService{
		db: database,
	}
}

// SavedShowResponse represents a saved show with metadata
type SavedShowResponse struct {
	ShowResponse
	SavedAt time.Time `json:"saved_at"`
}

// SaveShow saves a show to a user's list
// Note: Unlike the original plan, this allows saving shows of any status (pending/approved/rejected)
// as per user requirements
func (s *SavedShowService) SaveShow(userID, showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if show exists
	var show models.Show
	if err := s.db.First(&show, showID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrShowNotFound(showID)
		}
		return fmt.Errorf("failed to verify show: %w", err)
	}

	// Create saved show record (or update if exists)
	savedShow := models.UserSavedShow{
		UserID:  userID,
		ShowID:  showID,
		SavedAt: time.Now().UTC(),
	}

	// Use FirstOrCreate to avoid duplicate key errors
	err := s.db.Where(models.UserSavedShow{UserID: userID, ShowID: showID}).
		Assign(models.UserSavedShow{SavedAt: savedShow.SavedAt}).
		FirstOrCreate(&savedShow).Error

	if err != nil {
		return fmt.Errorf("failed to save show: %w", err)
	}

	return nil
}

// UnsaveShow removes a show from a user's list
func (s *SavedShowService) UnsaveShow(userID, showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("user_id = ? AND show_id = ?", userID, showID).
		Delete(&models.UserSavedShow{})

	if result.Error != nil {
		return fmt.Errorf("failed to unsave show: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("show was not saved")
	}

	return nil
}

// GetUserSavedShows retrieves all shows saved by a user
// Returns shows ordered by most recently saved first
func (s *SavedShowService) GetUserSavedShows(userID uint, limit, offset int) ([]*SavedShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count
	var total int64
	if err := s.db.Model(&models.UserSavedShow{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count saved shows: %w", err)
	}

	// Get saved show records with pagination, ordered by saved_at DESC
	var savedShows []models.UserSavedShow
	err := s.db.Where("user_id = ?", userID).
		Order("saved_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&savedShows).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get saved shows: %w", err)
	}

	// Extract show IDs
	showIDs := make([]uint, len(savedShows))
	savedAtMap := make(map[uint]time.Time)
	for i, ss := range savedShows {
		showIDs[i] = ss.ShowID
		savedAtMap[ss.ShowID] = ss.SavedAt
	}

	if len(showIDs) == 0 {
		return []*SavedShowResponse{}, total, nil
	}

	// Fetch shows with associations (no status filter - user can save any show)
	var shows []models.Show
	err = s.db.Preload("Venues").
		Where("id IN ?", showIDs).
		Find(&shows).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch shows: %w", err)
	}

	// Create a map for O(1) lookup
	showMap := make(map[uint]*models.Show)
	for i := range shows {
		showMap[shows[i].ID] = &shows[i]
	}

	// Batch-load all ShowArtist records for all shows
	var allShowArtists []models.ShowArtist
	if len(showIDs) > 0 {
		s.db.Where("show_id IN ?", showIDs).Order("position ASC").Find(&allShowArtists)
	}

	// Collect all unique artist IDs
	var allArtistIDs []uint
	for _, sa := range allShowArtists {
		allArtistIDs = append(allArtistIDs, sa.ArtistID)
	}

	// Batch-fetch all artists in one query
	artistMap := make(map[uint]*models.Artist)
	if len(allArtistIDs) > 0 {
		var allArtists []models.Artist
		s.db.Where("id IN ?", allArtistIDs).Find(&allArtists)
		for i := range allArtists {
			artistMap[allArtists[i].ID] = &allArtists[i]
		}
	}

	// Build per-show artist response slices
	artistsByShow := make(map[uint][]ArtistResponse)
	for _, sa := range allShowArtists {
		artist, ok := artistMap[sa.ArtistID]
		if !ok {
			continue
		}
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
		isHeadliner := sa.SetType == "headliner"
		isNewArtist := false
		artistsByShow[sa.ShowID] = append(artistsByShow[sa.ShowID], ArtistResponse{
			ID:          artist.ID,
			Name:        artist.Name,
			State:       artist.State,
			City:        artist.City,
			IsHeadliner: &isHeadliner,
			IsNewArtist: &isNewArtist,
			Socials:     socials,
		})
	}

	// Build responses in the order of savedShows (saved_at DESC)
	responses := make([]*SavedShowResponse, 0, len(shows))
	for _, ss := range savedShows {
		if show, ok := showMap[ss.ShowID]; ok {
			showResp := s.buildShowResponse(show, artistsByShow)
			responses = append(responses, &SavedShowResponse{
				ShowResponse: *showResp,
				SavedAt:      ss.SavedAt,
			})
		}
	}

	return responses, total, nil
}

// buildShowResponse builds a ShowResponse from a models.Show
// artistsByShow contains pre-loaded artist responses keyed by show ID
func (s *SavedShowService) buildShowResponse(show *models.Show, artistsByShow map[uint][]ArtistResponse) *ShowResponse {
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

	artists := artistsByShow[show.ID]

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

// IsShowSaved checks if a show is saved by a user
func (s *SavedShowService) IsShowSaved(userID, showID uint) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	var count int64
	err := s.db.Model(&models.UserSavedShow{}).
		Where("user_id = ? AND show_id = ?", userID, showID).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check if show is saved: %w", err)
	}

	return count > 0, nil
}

// GetSavedShowIDs returns a set of show IDs that a user has saved
// Useful for batch checking (e.g., mark which shows in a list are saved)
func (s *SavedShowService) GetSavedShowIDs(userID uint, showIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[uint]bool)

	if len(showIDs) == 0 {
		return result, nil
	}

	var savedShows []models.UserSavedShow
	err := s.db.Where("user_id = ? AND show_id IN ?", userID, showIDs).
		Find(&savedShows).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get saved show IDs: %w", err)
	}

	for _, ss := range savedShows {
		result[ss.ShowID] = true
	}

	return result, nil
}
