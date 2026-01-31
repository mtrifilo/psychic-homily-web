package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// FavoriteVenueService handles favorite venue business logic
type FavoriteVenueService struct {
	db *gorm.DB
}

// NewFavoriteVenueService creates a new favorite venue service
func NewFavoriteVenueService() *FavoriteVenueService {
	return &FavoriteVenueService{
		db: db.GetDB(),
	}
}

// FavoriteVenueResponse represents a favorite venue with metadata
type FavoriteVenueResponse struct {
	ID                 uint      `json:"id"`
	Slug               string    `json:"slug"`
	Name               string    `json:"name"`
	Address            *string   `json:"address"`
	City               string    `json:"city"`
	State              string    `json:"state"`
	Verified           bool      `json:"verified"`
	FavoritedAt        time.Time `json:"favorited_at"`
	UpcomingShowCount  int       `json:"upcoming_show_count"`
}

// FavoriteVenueShowResponse represents a show from a favorite venue
type FavoriteVenueShowResponse struct {
	ID             uint             `json:"id"`
	Slug           string           `json:"slug"`
	Title          string           `json:"title"`
	EventDate      time.Time        `json:"event_date"`
	City           *string          `json:"city"`
	State          *string          `json:"state"`
	Price          *float64         `json:"price"`
	AgeRequirement *string          `json:"age_requirement"`
	VenueID        uint             `json:"venue_id"`
	VenueName      string           `json:"venue_name"`
	VenueSlug      string           `json:"venue_slug"`
	Artists        []ArtistResponse `json:"artists"`
}

// FavoriteVenue adds a venue to user's favorites
func (s *FavoriteVenueService) FavoriteVenue(userID, venueID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if venue exists
	var venue models.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("venue not found")
		}
		return fmt.Errorf("failed to verify venue: %w", err)
	}

	// Create favorite venue record (or update if exists)
	favoriteVenue := models.UserFavoriteVenue{
		UserID:      userID,
		VenueID:     venueID,
		FavoritedAt: time.Now().UTC(),
	}

	// Use FirstOrCreate to avoid duplicate key errors
	err := s.db.Where(models.UserFavoriteVenue{UserID: userID, VenueID: venueID}).
		Assign(models.UserFavoriteVenue{FavoritedAt: favoriteVenue.FavoritedAt}).
		FirstOrCreate(&favoriteVenue).Error

	if err != nil {
		return fmt.Errorf("failed to favorite venue: %w", err)
	}

	return nil
}

// UnfavoriteVenue removes a venue from user's favorites
func (s *FavoriteVenueService) UnfavoriteVenue(userID, venueID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("user_id = ? AND venue_id = ?", userID, venueID).
		Delete(&models.UserFavoriteVenue{})

	if result.Error != nil {
		return fmt.Errorf("failed to unfavorite venue: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("venue was not favorited")
	}

	return nil
}

// GetUserFavoriteVenues retrieves all venues favorited by a user
// Returns venues ordered by most recently favorited first
func (s *FavoriteVenueService) GetUserFavoriteVenues(userID uint, limit, offset int) ([]*FavoriteVenueResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count
	var total int64
	if err := s.db.Model(&models.UserFavoriteVenue{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count favorite venues: %w", err)
	}

	// Get favorite venue records with pagination, ordered by favorited_at DESC
	var favoriteVenues []models.UserFavoriteVenue
	err := s.db.Where("user_id = ?", userID).
		Order("favorited_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&favoriteVenues).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get favorite venues: %w", err)
	}

	// Extract venue IDs
	venueIDs := make([]uint, len(favoriteVenues))
	favoritedAtMap := make(map[uint]time.Time)
	for i, fv := range favoriteVenues {
		venueIDs[i] = fv.VenueID
		favoritedAtMap[fv.VenueID] = fv.FavoritedAt
	}

	if len(venueIDs) == 0 {
		return []*FavoriteVenueResponse{}, total, nil
	}

	// Fetch venues
	var venues []models.Venue
	err = s.db.Where("id IN ?", venueIDs).Find(&venues).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch venues: %w", err)
	}

	// Create a map for O(1) lookup
	venueMap := make(map[uint]*models.Venue)
	for i := range venues {
		venueMap[venues[i].ID] = &venues[i]
	}

	// Get upcoming show counts for each venue
	now := time.Now().UTC()
	type showCount struct {
		VenueID uint
		Count   int
	}
	var showCounts []showCount
	err = s.db.Table("shows").
		Select("show_venues.venue_id as venue_id, COUNT(*) as count").
		Joins("JOIN show_venues ON shows.id = show_venues.show_id").
		Where("show_venues.venue_id IN ?", venueIDs).
		Where("shows.event_date >= ?", now).
		Where("shows.status = ?", models.ShowStatusApproved).
		Group("show_venues.venue_id").
		Scan(&showCounts).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	showCountMap := make(map[uint]int)
	for _, sc := range showCounts {
		showCountMap[sc.VenueID] = sc.Count
	}

	// Build responses in the order of favoriteVenues (favorited_at DESC)
	responses := make([]*FavoriteVenueResponse, 0, len(venues))
	for _, fv := range favoriteVenues {
		if venue, ok := venueMap[fv.VenueID]; ok {
			var slug string
			if venue.Slug != nil {
				slug = *venue.Slug
			}
			responses = append(responses, &FavoriteVenueResponse{
				ID:                venue.ID,
				Slug:              slug,
				Name:              venue.Name,
				Address:           venue.Address,
				City:              venue.City,
				State:             venue.State,
				Verified:          venue.Verified,
				FavoritedAt:       fv.FavoritedAt,
				UpcomingShowCount: showCountMap[venue.ID],
			})
		}
	}

	return responses, total, nil
}

// IsVenueFavorited checks if a venue is favorited by a user
func (s *FavoriteVenueService) IsVenueFavorited(userID, venueID uint) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	var count int64
	err := s.db.Model(&models.UserFavoriteVenue{}).
		Where("user_id = ? AND venue_id = ?", userID, venueID).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check if venue is favorited: %w", err)
	}

	return count > 0, nil
}

// GetUpcomingShowsFromFavorites retrieves all upcoming shows from a user's favorite venues
// Shows are sorted by event date (soonest first)
func (s *FavoriteVenueService) GetUpcomingShowsFromFavorites(userID uint, timezone string, limit, offset int) ([]*FavoriteVenueShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get favorite venue IDs for the user
	var favoriteVenues []models.UserFavoriteVenue
	err := s.db.Where("user_id = ?", userID).Find(&favoriteVenues).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get favorite venues: %w", err)
	}

	if len(favoriteVenues) == 0 {
		return []*FavoriteVenueShowResponse{}, 0, nil
	}

	venueIDs := make([]uint, len(favoriteVenues))
	for i, fv := range favoriteVenues {
		venueIDs[i] = fv.VenueID
	}

	// Get total count of upcoming shows from favorite venues
	now := time.Now().UTC()
	var total int64
	err = s.db.Table("shows").
		Joins("JOIN show_venues ON shows.id = show_venues.show_id").
		Where("show_venues.venue_id IN ?", venueIDs).
		Where("shows.event_date >= ?", now).
		Where("shows.status = ?", models.ShowStatusApproved).
		Count(&total).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	// Get shows with pagination
	var shows []models.Show
	err = s.db.Preload("Venues").Preload("Artists").
		Joins("JOIN show_venues ON shows.id = show_venues.show_id").
		Where("show_venues.venue_id IN ?", venueIDs).
		Where("shows.event_date >= ?", now).
		Where("shows.status = ?", models.ShowStatusApproved).
		Order("shows.event_date ASC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get upcoming shows: %w", err)
	}

	// Build responses
	responses := make([]*FavoriteVenueShowResponse, 0, len(shows))
	for _, show := range shows {
		// Find the venue that's in the favorites list
		var venueName, venueSlug string
		var venueID uint
		for _, venue := range show.Venues {
			for _, favVenueID := range venueIDs {
				if venue.ID == favVenueID {
					venueID = venue.ID
					venueName = venue.Name
					if venue.Slug != nil {
						venueSlug = *venue.Slug
					}
					break
				}
			}
			if venueID != 0 {
				break
			}
		}

		// If no favorite venue found (shouldn't happen), use first venue
		if venueID == 0 && len(show.Venues) > 0 {
			venueID = show.Venues[0].ID
			venueName = show.Venues[0].Name
			if show.Venues[0].Slug != nil {
				venueSlug = *show.Venues[0].Slug
			}
		}

		// Get ordered artists from show_artists table
		artists := s.getOrderedArtistsForShow(show.ID)

		var slug string
		if show.Slug != nil {
			slug = *show.Slug
		}

		responses = append(responses, &FavoriteVenueShowResponse{
			ID:             show.ID,
			Slug:           slug,
			Title:          show.Title,
			EventDate:      show.EventDate,
			City:           show.City,
			State:          show.State,
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			VenueID:        venueID,
			VenueName:      venueName,
			VenueSlug:      venueSlug,
			Artists:        artists,
		})
	}

	return responses, total, nil
}

// getOrderedArtistsForShow gets artists in their display order for a show
func (s *FavoriteVenueService) getOrderedArtistsForShow(showID uint) []ArtistResponse {
	var showArtists []models.ShowArtist
	s.db.Where("show_id = ?", showID).Order("position ASC").Find(&showArtists)

	artists := make([]ArtistResponse, 0, len(showArtists))
	for _, sa := range showArtists {
		var artist models.Artist
		if err := s.db.First(&artist, sa.ArtistID).Error; err == nil {
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

			var slug string
			if artist.Slug != nil {
				slug = *artist.Slug
			}

			artists = append(artists, ArtistResponse{
				ID:               artist.ID,
				Slug:             slug,
				Name:             artist.Name,
				State:            artist.State,
				City:             artist.City,
				IsHeadliner:      &isHeadliner,
				IsNewArtist:      &isNewArtist,
				BandcampEmbedURL: artist.BandcampEmbedURL,
				Socials:          socials,
			})
		}
	}

	return artists
}

// GetFavoriteVenueIDs returns a set of venue IDs that a user has favorited
// Useful for batch checking (e.g., mark which venues in a list are favorited)
func (s *FavoriteVenueService) GetFavoriteVenueIDs(userID uint, venueIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[uint]bool)

	if len(venueIDs) == 0 {
		return result, nil
	}

	var favoriteVenues []models.UserFavoriteVenue
	err := s.db.Where("user_id = ? AND venue_id IN ?", userID, venueIDs).
		Find(&favoriteVenues).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get favorite venue IDs: %w", err)
	}

	for _, fv := range favoriteVenues {
		result[fv.VenueID] = true
	}

	return result, nil
}
