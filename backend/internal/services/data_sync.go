package services

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/utils"
)

// DataSyncService handles exporting and importing data between environments
type DataSyncService struct {
	db           *gorm.DB
	venueService *VenueService
}

// NewDataSyncService creates a new data sync service
func NewDataSyncService(database *gorm.DB) *DataSyncService {
	if database == nil {
		database = db.GetDB()
	}
	return &DataSyncService{
		db:           database,
		venueService: NewVenueService(database),
	}
}

// ExportedArtist represents an artist for export/import
type ExportedArtist struct {
	Name             string  `json:"name"`
	City             *string `json:"city,omitempty"`
	State            *string `json:"state,omitempty"`
	BandcampEmbedURL *string `json:"bandcampEmbedUrl,omitempty"`
	Instagram        *string `json:"instagram,omitempty"`
	Facebook         *string `json:"facebook,omitempty"`
	Twitter          *string `json:"twitter,omitempty"`
	YouTube          *string `json:"youtube,omitempty"`
	Spotify          *string `json:"spotify,omitempty"`
	SoundCloud       *string `json:"soundcloud,omitempty"`
	Bandcamp         *string `json:"bandcamp,omitempty"`
	Website          *string `json:"website,omitempty"`
}

// ExportedVenue represents a venue for export/import
type ExportedVenue struct {
	Name       string  `json:"name"`
	Address    *string `json:"address,omitempty"`
	City       string  `json:"city"`
	State      string  `json:"state"`
	Zipcode    *string `json:"zipcode,omitempty"`
	Verified   bool    `json:"verified"`
	Instagram  *string `json:"instagram,omitempty"`
	Facebook   *string `json:"facebook,omitempty"`
	Twitter    *string `json:"twitter,omitempty"`
	YouTube    *string `json:"youtube,omitempty"`
	Spotify    *string `json:"spotify,omitempty"`
	SoundCloud *string `json:"soundcloud,omitempty"`
	Bandcamp   *string `json:"bandcamp,omitempty"`
	Website    *string `json:"website,omitempty"`
}

// ExportedShowArtist represents an artist in a show lineup
type ExportedShowArtist struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	SetType  string `json:"setType"`
}

// ExportedShow represents a show for export/import
type ExportedShow struct {
	Title          string               `json:"title"`
	EventDate      string               `json:"eventDate"` // ISO format
	City           *string              `json:"city,omitempty"`
	State          *string              `json:"state,omitempty"`
	Price          *float64             `json:"price,omitempty"`
	AgeRequirement *string              `json:"ageRequirement,omitempty"`
	Description    *string              `json:"description,omitempty"`
	Status         string               `json:"status"`
	IsSoldOut      bool                 `json:"isSoldOut"`
	IsCancelled    bool                 `json:"isCancelled"`
	Venues         []ExportedVenue      `json:"venues"`
	Artists        []ExportedShowArtist `json:"artists"`
}

// ExportShowsParams contains filters for show export
type ExportShowsParams struct {
	Limit      int
	Offset     int
	Status     string // "approved", "pending", "all"
	FromDate   *time.Time
	City       string
	State      string
	IncludeAll bool // Include all related data
}

// ExportShowsResult contains exported shows with pagination info
type ExportShowsResult struct {
	Shows []ExportedShow `json:"shows"`
	Total int64          `json:"total"`
}

// ExportShows exports shows with their artists and venues
func (s *DataSyncService) ExportShows(params ExportShowsParams) (*ExportShowsResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}

	// Build query
	query := s.db.Model(&models.Show{}).
		Preload("Venues").
		Preload("Artists")

	// Apply status filter
	switch params.Status {
	case "approved":
		query = query.Where("status = ?", models.ShowStatusApproved)
	case "pending":
		query = query.Where("status = ?", models.ShowStatusPending)
	case "rejected":
		query = query.Where("status = ?", models.ShowStatusRejected)
	case "all", "":
		// No filter - include all
	default:
		query = query.Where("status = ?", params.Status)
	}

	// Apply date filter
	if params.FromDate != nil {
		query = query.Where("event_date >= ?", params.FromDate)
	}

	// Apply location filters
	if params.City != "" {
		query = query.Where("city = ?", params.City)
	}
	if params.State != "" {
		query = query.Where("state = ?", params.State)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count shows: %w", err)
	}

	// Get shows with pagination
	var shows []models.Show
	if err := query.Order("event_date DESC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&shows).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch shows: %w", err)
	}

	// Get show artists with position info
	showIDs := make([]uint, len(shows))
	for i, show := range shows {
		showIDs[i] = show.ID
	}

	var showArtists []models.ShowArtist
	if len(showIDs) > 0 {
		if err := s.db.Where("show_id IN ?", showIDs).Find(&showArtists).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch show artists: %w", err)
		}
	}

	// Build show artist map
	showArtistMap := make(map[uint][]models.ShowArtist)
	for _, sa := range showArtists {
		showArtistMap[sa.ShowID] = append(showArtistMap[sa.ShowID], sa)
	}

	// Convert to exported format
	exported := make([]ExportedShow, len(shows))
	for i, show := range shows {
		exported[i] = ExportedShow{
			Title:          show.Title,
			EventDate:      show.EventDate.Format(time.RFC3339),
			City:           show.City,
			State:          show.State,
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			Description:    show.Description,
			Status:         string(show.Status),
			IsSoldOut:      show.IsSoldOut,
			IsCancelled:    show.IsCancelled,
			Venues:         make([]ExportedVenue, len(show.Venues)),
			Artists:        make([]ExportedShowArtist, 0),
		}

		// Convert venues
		for j, venue := range show.Venues {
			exported[i].Venues[j] = ExportedVenue{
				Name:       venue.Name,
				Address:    venue.Address,
				City:       venue.City,
				State:      venue.State,
				Zipcode:    venue.Zipcode,
				Verified:   venue.Verified,
				Instagram:  venue.Social.Instagram,
				Facebook:   venue.Social.Facebook,
				Twitter:    venue.Social.Twitter,
				YouTube:    venue.Social.YouTube,
				Spotify:    venue.Social.Spotify,
				SoundCloud: venue.Social.SoundCloud,
				Bandcamp:   venue.Social.Bandcamp,
				Website:    venue.Social.Website,
			}
		}

		// Convert artists with position info
		for _, artist := range show.Artists {
			// Find position info from showArtistMap
			position := 0
			setType := "performer"
			for _, sa := range showArtistMap[show.ID] {
				if sa.ArtistID == artist.ID {
					position = sa.Position
					setType = sa.SetType
					break
				}
			}

			exported[i].Artists = append(exported[i].Artists, ExportedShowArtist{
				Name:     artist.Name,
				Position: position,
				SetType:  setType,
			})
		}
	}

	return &ExportShowsResult{
		Shows: exported,
		Total: total,
	}, nil
}

// ExportArtistsParams contains filters for artist export
type ExportArtistsParams struct {
	Limit  int
	Offset int
	Search string
}

// ExportArtistsResult contains exported artists with pagination info
type ExportArtistsResult struct {
	Artists []ExportedArtist `json:"artists"`
	Total   int64            `json:"total"`
}

// ExportArtists exports artists
func (s *DataSyncService) ExportArtists(params ExportArtistsParams) (*ExportArtistsResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}

	query := s.db.Model(&models.Artist{})

	if params.Search != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+params.Search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists: %w", err)
	}

	var artists []models.Artist
	if err := query.Order("name ASC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&artists).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch artists: %w", err)
	}

	exported := make([]ExportedArtist, len(artists))
	for i, artist := range artists {
		exported[i] = ExportedArtist{
			Name:             artist.Name,
			City:             artist.City,
			State:            artist.State,
			BandcampEmbedURL: artist.BandcampEmbedURL,
			Instagram:        artist.Social.Instagram,
			Facebook:         artist.Social.Facebook,
			Twitter:          artist.Social.Twitter,
			YouTube:          artist.Social.YouTube,
			Spotify:          artist.Social.Spotify,
			SoundCloud:       artist.Social.SoundCloud,
			Bandcamp:         artist.Social.Bandcamp,
			Website:          artist.Social.Website,
		}
	}

	return &ExportArtistsResult{
		Artists: exported,
		Total:   total,
	}, nil
}

// ExportVenuesParams contains filters for venue export
type ExportVenuesParams struct {
	Limit    int
	Offset   int
	Search   string
	Verified *bool
	City     string
	State    string
}

// ExportVenuesResult contains exported venues with pagination info
type ExportVenuesResult struct {
	Venues []ExportedVenue `json:"venues"`
	Total  int64           `json:"total"`
}

// ExportVenues exports venues
func (s *DataSyncService) ExportVenues(params ExportVenuesParams) (*ExportVenuesResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}

	query := s.db.Model(&models.Venue{})

	if params.Search != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+params.Search+"%")
	}
	if params.Verified != nil {
		query = query.Where("verified = ?", *params.Verified)
	}
	if params.City != "" {
		query = query.Where("city = ?", params.City)
	}
	if params.State != "" {
		query = query.Where("state = ?", params.State)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	}

	var venues []models.Venue
	if err := query.Order("name ASC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&venues).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch venues: %w", err)
	}

	exported := make([]ExportedVenue, len(venues))
	for i, venue := range venues {
		exported[i] = ExportedVenue{
			Name:       venue.Name,
			Address:    venue.Address,
			City:       venue.City,
			State:      venue.State,
			Zipcode:    venue.Zipcode,
			Verified:   venue.Verified,
			Instagram:  venue.Social.Instagram,
			Facebook:   venue.Social.Facebook,
			Twitter:    venue.Social.Twitter,
			YouTube:    venue.Social.YouTube,
			Spotify:    venue.Social.Spotify,
			SoundCloud: venue.Social.SoundCloud,
			Bandcamp:   venue.Social.Bandcamp,
			Website:    venue.Social.Website,
		}
	}

	return &ExportVenuesResult{
		Venues: exported,
		Total:  total,
	}, nil
}

// DataImportRequest represents a batch import request
type DataImportRequest struct {
	Shows   []ExportedShow   `json:"shows,omitempty"`
	Artists []ExportedArtist `json:"artists,omitempty"`
	Venues  []ExportedVenue  `json:"venues,omitempty"`
	DryRun  bool             `json:"dryRun"`
}

// DataImportResult contains statistics about the import operation
type DataImportResult struct {
	Shows struct {
		Total      int      `json:"total"`
		Imported   int      `json:"imported"`
		Duplicates int      `json:"duplicates"`
		Errors     int      `json:"errors"`
		Messages   []string `json:"messages"`
	} `json:"shows"`
	Artists struct {
		Total      int      `json:"total"`
		Imported   int      `json:"imported"`
		Duplicates int      `json:"duplicates"`
		Updated    int      `json:"updated"`
		Errors     int      `json:"errors"`
		Messages   []string `json:"messages"`
	} `json:"artists"`
	Venues struct {
		Total      int      `json:"total"`
		Imported   int      `json:"imported"`
		Duplicates int      `json:"duplicates"`
		Updated    int      `json:"updated"`
		Errors     int      `json:"errors"`
		Messages   []string `json:"messages"`
	} `json:"venues"`
}

// ImportData imports shows, artists, and venues with deduplication
func (s *DataSyncService) ImportData(req DataImportRequest) (*DataImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &DataImportResult{}
	result.Shows.Messages = make([]string, 0)
	result.Artists.Messages = make([]string, 0)
	result.Venues.Messages = make([]string, 0)

	// Import artists first (shows depend on them)
	result.Artists.Total = len(req.Artists)
	for _, artist := range req.Artists {
		msg, status := s.importArtist(&artist, req.DryRun)
		result.Artists.Messages = append(result.Artists.Messages, msg)
		switch status {
		case "imported":
			result.Artists.Imported++
		case "duplicate":
			result.Artists.Duplicates++
		case "updated":
			result.Artists.Updated++
		case "error":
			result.Artists.Errors++
		}
	}

	// Import venues second (shows depend on them)
	result.Venues.Total = len(req.Venues)
	for _, venue := range req.Venues {
		msg, status := s.importVenue(&venue, req.DryRun)
		result.Venues.Messages = append(result.Venues.Messages, msg)
		switch status {
		case "imported":
			result.Venues.Imported++
		case "duplicate":
			result.Venues.Duplicates++
		case "updated":
			result.Venues.Updated++
		case "error":
			result.Venues.Errors++
		}
	}

	// Import shows last
	result.Shows.Total = len(req.Shows)
	for _, show := range req.Shows {
		msg, status := s.importShow(&show, req.DryRun)
		result.Shows.Messages = append(result.Shows.Messages, msg)
		switch status {
		case "imported":
			result.Shows.Imported++
		case "duplicate":
			result.Shows.Duplicates++
		case "error":
			result.Shows.Errors++
		}
	}

	return result, nil
}

// importArtist imports a single artist with deduplication
func (s *DataSyncService) importArtist(artist *ExportedArtist, dryRun bool) (string, string) {
	if artist.Name == "" {
		return "SKIP: Artist name is required", "error"
	}

	// Check for existing artist by name (case insensitive)
	var existing models.Artist
	err := s.db.Where("LOWER(name) = LOWER(?)", artist.Name).First(&existing).Error
	if err == nil {
		// Artist exists — backfill slug if missing
		if existing.Slug == nil && !dryRun {
			baseSlug := utils.GenerateArtistSlug(existing.Name)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				s.db.Model(&models.Artist{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			s.db.Model(&existing).Update("slug", slug)
		}
		return fmt.Sprintf("DUPLICATE: Artist '%s' already exists (ID: %d)", artist.Name, existing.ID), "duplicate"
	} else if err != gorm.ErrRecordNotFound {
		return fmt.Sprintf("ERROR: Failed to check artist '%s': %v", artist.Name, err), "error"
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: Artist '%s'", artist.Name), "imported"
	}

	// Create new artist with slug
	baseSlug := utils.GenerateArtistSlug(artist.Name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Artist{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	newArtist := models.Artist{
		Name:             artist.Name,
		Slug:             &slug,
		City:             artist.City,
		State:            artist.State,
		BandcampEmbedURL: artist.BandcampEmbedURL,
		Social: models.Social{
			Instagram:  artist.Instagram,
			Facebook:   artist.Facebook,
			Twitter:    artist.Twitter,
			YouTube:    artist.YouTube,
			Spotify:    artist.Spotify,
			SoundCloud: artist.SoundCloud,
			Bandcamp:   artist.Bandcamp,
			Website:    artist.Website,
		},
	}

	if err := s.db.Create(&newArtist).Error; err != nil {
		return fmt.Sprintf("ERROR: Failed to create artist '%s': %v", artist.Name, err), "error"
	}

	return fmt.Sprintf("IMPORTED: Artist '%s' (ID: %d)", artist.Name, newArtist.ID), "imported"
}

// importVenue imports a single venue with deduplication
func (s *DataSyncService) importVenue(venue *ExportedVenue, dryRun bool) (string, string) {
	if venue.Name == "" || venue.City == "" || venue.State == "" {
		return "SKIP: Venue name, city, and state are required", "error"
	}

	// Check for existing venue by name + city (case insensitive)
	var existing models.Venue
	err := s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", venue.Name, venue.City).First(&existing).Error
	if err == nil {
		// Venue exists — backfill slug if missing
		if existing.Slug == nil && !dryRun {
			baseSlug := utils.GenerateVenueSlug(existing.Name, existing.City, existing.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				s.db.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			s.db.Model(&existing).Update("slug", slug)
		}
		return fmt.Sprintf("DUPLICATE: Venue '%s' in %s already exists (ID: %d)", venue.Name, venue.City, existing.ID), "duplicate"
	} else if err != gorm.ErrRecordNotFound {
		return fmt.Sprintf("ERROR: Failed to check venue '%s': %v", venue.Name, err), "error"
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: Venue '%s' in %s, %s", venue.Name, venue.City, venue.State), "imported"
	}

	// Create new venue with slug
	baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	newVenue := models.Venue{
		Name:     venue.Name,
		Slug:     &slug,
		Address:  venue.Address,
		City:     venue.City,
		State:    venue.State,
		Zipcode:  venue.Zipcode,
		Verified: venue.Verified,
		Social: models.Social{
			Instagram:  venue.Instagram,
			Facebook:   venue.Facebook,
			Twitter:    venue.Twitter,
			YouTube:    venue.YouTube,
			Spotify:    venue.Spotify,
			SoundCloud: venue.SoundCloud,
			Bandcamp:   venue.Bandcamp,
			Website:    venue.Website,
		},
	}

	if err := s.db.Create(&newVenue).Error; err != nil {
		return fmt.Sprintf("ERROR: Failed to create venue '%s': %v", venue.Name, err), "error"
	}

	return fmt.Sprintf("IMPORTED: Venue '%s' in %s (ID: %d)", venue.Name, venue.City, newVenue.ID), "imported"
}

// importShow imports a single show with deduplication
func (s *DataSyncService) importShow(show *ExportedShow, dryRun bool) (string, string) {
	if show.Title == "" || show.EventDate == "" {
		return "SKIP: Show title and event date are required", "error"
	}

	// Parse event date
	eventDate, err := time.Parse(time.RFC3339, show.EventDate)
	if err != nil {
		return fmt.Sprintf("ERROR: Invalid event date '%s': %v", show.EventDate, err), "error"
	}

	// Get venue name for deduplication
	venueName := ""
	if len(show.Venues) > 0 {
		venueName = show.Venues[0].Name
	}

	// Check for duplicate: same title + venue + date
	if venueName != "" {
		startOfDay := time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), 0, 0, 0, 0, time.UTC)
		endOfDay := startOfDay.Add(24 * time.Hour)

		var existingShow models.Show
		err := s.db.Joins("JOIN show_venues ON shows.id = show_venues.show_id").
			Joins("JOIN venues ON show_venues.venue_id = venues.id").
			Where("LOWER(shows.title) = LOWER(?) AND LOWER(venues.name) = LOWER(?) AND shows.event_date >= ? AND shows.event_date < ?",
				show.Title, venueName, startOfDay, endOfDay).
			First(&existingShow).Error
		if err == nil {
			// Backfill slugs for the existing show and its associated entities
			if !dryRun {
				s.backfillShowSlugs(&existingShow, show, eventDate, venueName)
			}
			return fmt.Sprintf("DUPLICATE: Show '%s' at %s on %s already exists (ID: %d)",
				show.Title, venueName, eventDate.Format("2006-01-02"), existingShow.ID), "duplicate"
		} else if err != gorm.ErrRecordNotFound {
			return fmt.Sprintf("ERROR: Failed to check show '%s': %v", show.Title, err), "error"
		}
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: Show '%s' at %s on %s", show.Title, venueName, eventDate.Format("2006-01-02")), "imported"
	}

	// Create show in a transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Parse status
		status := models.ShowStatusApproved
		switch strings.ToLower(show.Status) {
		case "pending":
			status = models.ShowStatusPending
		case "rejected":
			status = models.ShowStatusRejected
		case "private":
			status = models.ShowStatusPrivate
		}

		// Determine headliner name for show slug
		headlinerName := ""
		for _, a := range show.Artists {
			if a.Position == 0 || headlinerName == "" {
				headlinerName = a.Name
			}
		}

		// Generate show slug
		baseShowSlug := utils.GenerateShowSlug(eventDate.UTC(), headlinerName, venueName)
		showSlug := utils.GenerateUniqueSlug(baseShowSlug, func(candidate string) bool {
			var count int64
			tx.Model(&models.Show{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})

		// Create show
		newShow := models.Show{
			Title:          show.Title,
			Slug:           &showSlug,
			EventDate:      eventDate.UTC(),
			City:           show.City,
			State:          show.State,
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			Description:    show.Description,
			Status:         status,
			Source:         models.ShowSourceUser,
			IsSoldOut:      show.IsSoldOut,
			IsCancelled:    show.IsCancelled,
		}

		if err := tx.Create(&newShow).Error; err != nil {
			return fmt.Errorf("failed to create show: %w", err)
		}

		// Link venues
		for _, exportedVenue := range show.Venues {
			var venue models.Venue
			err := tx.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)",
				exportedVenue.Name, exportedVenue.City).First(&venue).Error
			if err == gorm.ErrRecordNotFound {
				// Create venue with slug
				venueBaseSlug := utils.GenerateVenueSlug(exportedVenue.Name, exportedVenue.City, exportedVenue.State)
				venueSlug := utils.GenerateUniqueSlug(venueBaseSlug, func(candidate string) bool {
					var count int64
					tx.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
					return count > 0
				})
				venue = models.Venue{
					Name:     exportedVenue.Name,
					Slug:     &venueSlug,
					Address:  exportedVenue.Address,
					City:     exportedVenue.City,
					State:    exportedVenue.State,
					Zipcode:  exportedVenue.Zipcode,
					Verified: exportedVenue.Verified,
				}
				if err := tx.Create(&venue).Error; err != nil {
					return fmt.Errorf("failed to create venue: %w", err)
				}
			} else if err != nil {
				return fmt.Errorf("failed to find venue: %w", err)
			} else if venue.Slug == nil {
				// Backfill slug for existing venue
				venueBaseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
				venueSlug := utils.GenerateUniqueSlug(venueBaseSlug, func(candidate string) bool {
					var count int64
					tx.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
					return count > 0
				})
				tx.Model(&venue).Update("slug", venueSlug)
			}

			// Create show-venue association
			showVenue := models.ShowVenue{
				ShowID:  newShow.ID,
				VenueID: venue.ID,
			}
			if err := tx.Create(&showVenue).Error; err != nil {
				return fmt.Errorf("failed to link venue: %w", err)
			}
		}

		// Link artists
		for _, exportedArtist := range show.Artists {
			var artist models.Artist
			err := tx.Where("LOWER(name) = LOWER(?)", exportedArtist.Name).First(&artist).Error
			if err == gorm.ErrRecordNotFound {
				// Create artist with slug
				artistBaseSlug := utils.GenerateArtistSlug(exportedArtist.Name)
				artistSlug := utils.GenerateUniqueSlug(artistBaseSlug, func(candidate string) bool {
					var count int64
					tx.Model(&models.Artist{}).Where("slug = ?", candidate).Count(&count)
					return count > 0
				})
				artist = models.Artist{
					Name: exportedArtist.Name,
					Slug: &artistSlug,
				}
				if err := tx.Create(&artist).Error; err != nil {
					return fmt.Errorf("failed to create artist: %w", err)
				}
			} else if err != nil {
				return fmt.Errorf("failed to find artist: %w", err)
			} else if artist.Slug == nil {
				// Backfill slug for existing artist
				artistBaseSlug := utils.GenerateArtistSlug(artist.Name)
				artistSlug := utils.GenerateUniqueSlug(artistBaseSlug, func(candidate string) bool {
					var count int64
					tx.Model(&models.Artist{}).Where("slug = ?", candidate).Count(&count)
					return count > 0
				})
				tx.Model(&artist).Update("slug", artistSlug)
			}

			// Create show-artist association
			showArtist := models.ShowArtist{
				ShowID:   newShow.ID,
				ArtistID: artist.ID,
				Position: exportedArtist.Position,
				SetType:  exportedArtist.SetType,
			}
			if err := tx.Create(&showArtist).Error; err != nil {
				return fmt.Errorf("failed to link artist: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Sprintf("ERROR: Failed to import show '%s': %v", show.Title, err), "error"
	}

	return fmt.Sprintf("IMPORTED: Show '%s' at %s on %s", show.Title, venueName, eventDate.Format("2006-01-02")), "imported"
}

// backfillShowSlugs generates slugs for an existing show and its associated artists/venues if missing.
func (s *DataSyncService) backfillShowSlugs(existingShow *models.Show, show *ExportedShow, eventDate time.Time, venueName string) {
	// Backfill show slug
	if existingShow.Slug == nil {
		headlinerName := ""
		for _, a := range show.Artists {
			if a.Position == 0 || headlinerName == "" {
				headlinerName = a.Name
			}
		}
		baseSlug := utils.GenerateShowSlug(eventDate.UTC(), headlinerName, venueName)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&models.Show{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})
		s.db.Model(existingShow).Update("slug", slug)
	}

	// Backfill artist slugs
	for _, exportedArtist := range show.Artists {
		var artist models.Artist
		if err := s.db.Where("LOWER(name) = LOWER(?)", exportedArtist.Name).First(&artist).Error; err != nil {
			continue
		}
		if artist.Slug == nil {
			baseSlug := utils.GenerateArtistSlug(artist.Name)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				s.db.Model(&models.Artist{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			s.db.Model(&artist).Update("slug", slug)
		}
	}

	// Backfill venue slugs
	for _, exportedVenue := range show.Venues {
		var venue models.Venue
		if err := s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", exportedVenue.Name, exportedVenue.City).First(&venue).Error; err != nil {
			continue
		}
		if venue.Slug == nil {
			baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				s.db.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			s.db.Model(&venue).Update("slug", slug)
		}
	}
}
