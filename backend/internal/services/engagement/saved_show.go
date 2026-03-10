package engagement

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/contracts"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
)

// SavedShowService handles saved show business logic
// Backed by the generic user_bookmarks table via BookmarkService
type SavedShowService struct {
	db       *gorm.DB
	bookmark *BookmarkService
}

// NewSavedShowService creates a new saved show service
func NewSavedShowService(database *gorm.DB) *SavedShowService {
	if database == nil {
		database = db.GetDB()
	}
	return &SavedShowService{
		db:       database,
		bookmark: NewBookmarkService(database),
	}
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

	if err := s.bookmark.CreateBookmark(userID, models.BookmarkEntityShow, showID, models.BookmarkActionSave); err != nil {
		return fmt.Errorf("failed to save show: %w", err)
	}

	return nil
}

// UnsaveShow removes a show from a user's list
func (s *SavedShowService) UnsaveShow(userID, showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	err := s.bookmark.DeleteBookmark(userID, models.BookmarkEntityShow, showID, models.BookmarkActionSave)
	if err != nil {
		if err.Error() == "bookmark not found" {
			return fmt.Errorf("show was not saved")
		}
		return fmt.Errorf("failed to unsave show: %w", err)
	}

	return nil
}

// GetUserSavedShows retrieves all shows saved by a user
// Returns shows ordered by most recently saved first
func (s *SavedShowService) GetUserSavedShows(userID uint, limit, offset int) ([]*contracts.SavedShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get bookmarks with pagination
	bookmarks, total, err := s.bookmark.GetUserBookmarks(userID, models.BookmarkEntityShow, models.BookmarkActionSave, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get saved shows: %w", err)
	}

	// Extract show IDs preserving order
	showIDs := make([]uint, len(bookmarks))
	savedAtMap := make(map[uint]time.Time)
	for i, b := range bookmarks {
		showIDs[i] = b.EntityID
		savedAtMap[b.EntityID] = b.CreatedAt
	}

	if len(showIDs) == 0 {
		return []*contracts.SavedShowResponse{}, total, nil
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
	artistsByShow := make(map[uint][]contracts.ArtistResponse)
	for _, sa := range allShowArtists {
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
		artistsByShow[sa.ShowID] = append(artistsByShow[sa.ShowID], contracts.ArtistResponse{
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

	// Build responses in the order of bookmarks (created_at DESC)
	responses := make([]*contracts.SavedShowResponse, 0, len(shows))
	for _, b := range bookmarks {
		if show, ok := showMap[b.EntityID]; ok {
			showResp := s.buildShowResponse(show, artistsByShow)
			responses = append(responses, &contracts.SavedShowResponse{
				ShowResponse: *showResp,
				SavedAt:      b.CreatedAt,
			})
		}
	}

	return responses, total, nil
}

// buildShowResponse builds a ShowResponse from a models.Show
// artistsByShow contains pre-loaded artist responses keyed by show ID
func (s *SavedShowService) buildShowResponse(show *models.Show, artistsByShow map[uint][]contracts.ArtistResponse) *contracts.ShowResponse {
	// Build venue responses
	venues := make([]contracts.VenueResponse, len(show.Venues))
	for i, venue := range show.Venues {
		var venueSlug string
		if venue.Slug != nil {
			venueSlug = *venue.Slug
		}
		venues[i] = contracts.VenueResponse{
			ID:       venue.ID,
			Slug:     venueSlug,
			Name:     venue.Name,
			Address:  venue.Address,
			City:     venue.City,
			State:    venue.State,
			Verified: venue.Verified,
		}
	}

	artists := artistsByShow[show.ID]

	showSlug := ""
	if show.Slug != nil {
		showSlug = *show.Slug
	}
	return &contracts.ShowResponse{
		ID:                show.ID,
		Slug:              showSlug,
		Title:             show.Title,
		EventDate:         show.EventDate,
		City:              show.City,
		State:             show.State,
		Price:             show.Price,
		AgeRequirement:    show.AgeRequirement,
		Description:       show.Description,
		Status:            string(show.Status),
		SubmittedBy:       show.SubmittedBy,
		RejectionReason:   show.RejectionReason,
		Venues:            venues,
		Artists:           artists,
		CreatedAt:         show.CreatedAt,
		UpdatedAt:         show.UpdatedAt,
		IsSoldOut:         show.IsSoldOut,
		IsCancelled:       show.IsCancelled,
		Source:            string(show.Source),
		SourceVenue:       show.SourceVenue,
		ScrapedAt:         show.ScrapedAt,
		DuplicateOfShowID: show.DuplicateOfShowID,
	}
}

// IsShowSaved checks if a show is saved by a user
func (s *SavedShowService) IsShowSaved(userID, showID uint) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	return s.bookmark.IsBookmarked(userID, models.BookmarkEntityShow, showID, models.BookmarkActionSave)
}

// GetSavedShowIDs returns a set of show IDs that a user has saved
// Useful for batch checking (e.g., mark which shows in a list are saved)
func (s *SavedShowService) GetSavedShowIDs(userID uint, showIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	return s.bookmark.GetBookmarkedEntityIDs(userID, models.BookmarkEntityShow, models.BookmarkActionSave, showIDs)
}
