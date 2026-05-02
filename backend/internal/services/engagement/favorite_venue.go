package engagement

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// FavoriteVenueService handles favorite venue business logic
// Backed by the generic user_bookmarks table via BookmarkService
type FavoriteVenueService struct {
	db       *gorm.DB
	bookmark *BookmarkService
}

// NewFavoriteVenueService creates a new favorite venue service
func NewFavoriteVenueService(database *gorm.DB) *FavoriteVenueService {
	if database == nil {
		database = db.GetDB()
	}
	return &FavoriteVenueService{
		db:       database,
		bookmark: NewBookmarkService(database),
	}
}

// FavoriteVenue adds a venue to user's favorites
func (s *FavoriteVenueService) FavoriteVenue(userID, venueID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if venue exists
	var venue catalogm.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrVenueNotFound(venueID)
		}
		return fmt.Errorf("failed to verify venue: %w", err)
	}

	if err := s.bookmark.CreateBookmark(userID, engagementm.BookmarkEntityVenue, venueID, engagementm.BookmarkActionFollow); err != nil {
		return fmt.Errorf("failed to favorite venue: %w", err)
	}

	return nil
}

// UnfavoriteVenue removes a venue from user's favorites
func (s *FavoriteVenueService) UnfavoriteVenue(userID, venueID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	err := s.bookmark.DeleteBookmark(userID, engagementm.BookmarkEntityVenue, venueID, engagementm.BookmarkActionFollow)
	if err != nil {
		if err.Error() == "bookmark not found" {
			return fmt.Errorf("venue was not favorited")
		}
		return fmt.Errorf("failed to unfavorite venue: %w", err)
	}

	return nil
}

// GetUserFavoriteVenues retrieves all venues favorited by a user
// Returns venues ordered by most recently favorited first
func (s *FavoriteVenueService) GetUserFavoriteVenues(userID uint, limit, offset int) ([]*contracts.FavoriteVenueResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get bookmarks with pagination
	bookmarks, total, err := s.bookmark.GetUserBookmarks(userID, engagementm.BookmarkEntityVenue, engagementm.BookmarkActionFollow, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get favorite venues: %w", err)
	}

	// Extract venue IDs
	venueIDs := make([]uint, len(bookmarks))
	favoritedAtMap := make(map[uint]time.Time)
	for i, b := range bookmarks {
		venueIDs[i] = b.EntityID
		favoritedAtMap[b.EntityID] = b.CreatedAt
	}

	if len(venueIDs) == 0 {
		return []*contracts.FavoriteVenueResponse{}, total, nil
	}

	// Fetch venues
	var venues []catalogm.Venue
	err = s.db.Where("id IN ?", venueIDs).Find(&venues).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch venues: %w", err)
	}

	// Create a map for O(1) lookup
	venueMap := make(map[uint]*catalogm.Venue)
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
		Where("shows.status = ?", catalogm.ShowStatusApproved).
		Group("show_venues.venue_id").
		Scan(&showCounts).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	showCountMap := make(map[uint]int)
	for _, sc := range showCounts {
		showCountMap[sc.VenueID] = sc.Count
	}

	// Build responses in the order of bookmarks (created_at DESC)
	responses := make([]*contracts.FavoriteVenueResponse, 0, len(venues))
	for _, b := range bookmarks {
		if venue, ok := venueMap[b.EntityID]; ok {
			var slug string
			if venue.Slug != nil {
				slug = *venue.Slug
			}
			responses = append(responses, &contracts.FavoriteVenueResponse{
				ID:                venue.ID,
				Slug:              slug,
				Name:              venue.Name,
				Address:           venue.Address,
				City:              venue.City,
				State:             venue.State,
				Verified:          venue.Verified,
				FavoritedAt:       b.CreatedAt,
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

	return s.bookmark.IsBookmarked(userID, engagementm.BookmarkEntityVenue, venueID, engagementm.BookmarkActionFollow)
}

// GetUpcomingShowsFromFavorites retrieves all upcoming shows from a user's favorite venues
// Shows are sorted by event date (soonest first)
func (s *FavoriteVenueService) GetUpcomingShowsFromFavorites(userID uint, timezone string, limit, offset int) ([]*contracts.FavoriteVenueShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get favorite venue IDs for the user
	favoriteBookmarks, err := s.bookmark.GetUserBookmarksByEntityType(userID, engagementm.BookmarkEntityVenue, engagementm.BookmarkActionFollow)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get favorite venues: %w", err)
	}

	if len(favoriteBookmarks) == 0 {
		return []*contracts.FavoriteVenueShowResponse{}, 0, nil
	}

	venueIDs := make([]uint, len(favoriteBookmarks))
	for i, b := range favoriteBookmarks {
		venueIDs[i] = b.EntityID
	}

	// Build subquery for show IDs that have venues in the favorites list
	now := time.Now().UTC()
	showIDsSubquery := s.db.Table("show_venues").
		Select("DISTINCT show_id").
		Where("venue_id IN ?", venueIDs)

	// Get total count of upcoming shows from favorite venues
	var total int64
	err = s.db.Model(&catalogm.Show{}).
		Where("id IN (?)", showIDsSubquery).
		Where("event_date >= ?", now).
		Where("status = ?", catalogm.ShowStatusApproved).
		Count(&total).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	// Get shows with pagination using subquery
	var shows []catalogm.Show
	err = s.db.
		Where("id IN (?)", showIDsSubquery).
		Where("event_date >= ?", now).
		Where("status = ?", catalogm.ShowStatusApproved).
		Order("event_date ASC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get upcoming shows: %w", err)
	}

	if len(shows) == 0 {
		return []*contracts.FavoriteVenueShowResponse{}, total, nil
	}

	// Collect show IDs for fetching venues
	showIDs := make([]uint, len(shows))
	for i, show := range shows {
		showIDs[i] = show.ID
	}

	// Fetch show_venues join table entries
	type showVenueEntry struct {
		ShowID  uint
		VenueID uint
	}
	var showVenueEntries []showVenueEntry
	err = s.db.Table("show_venues").
		Select("show_id, venue_id").
		Where("show_id IN ?", showIDs).
		Where("venue_id IN ?", venueIDs). // Only get venues that are in favorites
		Scan(&showVenueEntries).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get show venues: %w", err)
	}

	// Get unique venue IDs from the entries
	venueIDSet := make(map[uint]bool)
	for _, entry := range showVenueEntries {
		venueIDSet[entry.VenueID] = true
	}
	uniqueVenueIDs := make([]uint, 0, len(venueIDSet))
	for id := range venueIDSet {
		uniqueVenueIDs = append(uniqueVenueIDs, id)
	}

	// Fetch venues
	var venues []catalogm.Venue
	if len(uniqueVenueIDs) > 0 {
		err = s.db.Where("id IN ?", uniqueVenueIDs).Find(&venues).Error
		if err != nil {
			return nil, 0, fmt.Errorf("failed to fetch venues: %w", err)
		}
	}

	// Create venue map for quick lookup
	venueMap := make(map[uint]*catalogm.Venue)
	for i := range venues {
		venueMap[venues[i].ID] = &venues[i]
	}

	// Create show -> venue mapping
	showToVenue := make(map[uint]*catalogm.Venue)
	for _, entry := range showVenueEntries {
		if venue, ok := venueMap[entry.VenueID]; ok {
			showToVenue[entry.ShowID] = venue
		}
	}

	// Batch-load all artists for all shows
	artistsByShow := s.getOrderedArtistsForShows(showIDs)

	// Build responses
	responses := make([]*contracts.FavoriteVenueShowResponse, 0, len(shows))
	for _, show := range shows {
		var venueName, venueSlug string
		var venueID uint

		if venue, ok := showToVenue[show.ID]; ok {
			venueID = venue.ID
			venueName = venue.Name
			if venue.Slug != nil {
				venueSlug = *venue.Slug
			}
		}

		artists := artistsByShow[show.ID]

		var slug string
		if show.Slug != nil {
			slug = *show.Slug
		}

		responses = append(responses, &contracts.FavoriteVenueShowResponse{
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

// getOrderedArtistsForShows batch-loads artists for multiple shows in display order
func (s *FavoriteVenueService) getOrderedArtistsForShows(showIDs []uint) map[uint][]contracts.ArtistResponse {
	result := make(map[uint][]contracts.ArtistResponse)
	if len(showIDs) == 0 {
		return result
	}

	// Batch-load all ShowArtist records
	var showArtists []catalogm.ShowArtist
	s.db.Where("show_id IN ?", showIDs).Order("position ASC").Find(&showArtists)

	// Collect all unique artist IDs
	var allArtistIDs []uint
	for _, sa := range showArtists {
		allArtistIDs = append(allArtistIDs, sa.ArtistID)
	}

	// Batch-fetch all artists in one query
	artistMap := make(map[uint]*catalogm.Artist)
	if len(allArtistIDs) > 0 {
		var allArtists []catalogm.Artist
		s.db.Where("id IN ?", allArtistIDs).Find(&allArtists)
		for i := range allArtists {
			artistMap[allArtists[i].ID] = &allArtists[i]
		}
	}

	// Build per-show artist slices using map lookup
	for _, sa := range showArtists {
		artist, ok := artistMap[sa.ArtistID]
		if !ok {
			continue
		}
		socials := contracts.ShowArtistSocials{
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

		result[sa.ShowID] = append(result[sa.ShowID], contracts.ArtistResponse{
			ID:               artist.ID,
			Slug:             slug,
			Name:             artist.Name,
			State:            artist.State,
			City:             artist.City,
			IsHeadliner:      &isHeadliner,
			SetType:          sa.SetType,
			Position:         sa.Position,
			IsNewArtist:      &isNewArtist,
			BandcampEmbedURL: artist.BandcampEmbedURL,
			Socials:          socials,
		})
	}

	return result
}

// GetFavoriteVenueIDs returns a set of venue IDs that a user has favorited
// Useful for batch checking (e.g., mark which venues in a list are favorited)
func (s *FavoriteVenueService) GetFavoriteVenueIDs(userID uint, venueIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	return s.bookmark.GetBookmarkedEntityIDs(userID, engagementm.BookmarkEntityVenue, engagementm.BookmarkActionFollow, venueIDs)
}
