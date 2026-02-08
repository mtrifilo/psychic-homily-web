package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/utils"
)

// VenueService handles venue-related business logic
type VenueService struct {
	db *gorm.DB
}

// NewVenueService creates a new venue service
func NewVenueService(database *gorm.DB) *VenueService {
	if database == nil {
		database = db.GetDB()
	}
	return &VenueService{
		db: database,
	}
}

// CreateVenueRequest represents the data needed to create a new venue
type CreateVenueRequest struct {
	Name       string  `json:"name" validate:"required"`
	Address    *string `json:"address"`
	City       string  `json:"city" validate:"required"`
	State      string  `json:"state" validate:"required"`
	Zipcode    *string `json:"zipcode"`
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// VenueDetailResponse represents the venue data returned to clients
type VenueDetailResponse struct {
	ID          uint           `json:"id"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Address     *string        `json:"address"`
	City        string         `json:"city"`
	State       string         `json:"state"`
	Zipcode     *string        `json:"zipcode"`
	Verified    bool           `json:"verified"`    // Admin-verified as legitimate venue
	SubmittedBy *uint          `json:"submitted_by"` // User ID who originally submitted this venue
	Social      SocialResponse `json:"social"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// CreateVenue creates a new venue
// If isAdmin is true, the venue is automatically verified.
// If isAdmin is false, the venue requires admin approval (verified = false).
func (s *VenueService) CreateVenue(req *CreateVenueRequest, isAdmin bool) (*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if venue already exists (same name in same city)
	var existingVenue models.Venue
	err := s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", req.Name, req.City).First(&existingVenue).Error
	if err == nil {
		return nil, fmt.Errorf("venue with name '%s' already exists in %s", req.Name, req.City)
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Generate unique slug
	baseSlug := utils.GenerateVenueSlug(req.Name, req.City, req.State)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	// Create the venue - verified if created by admin, unverified otherwise
	venue := &models.Venue{
		Name:     req.Name,
		Slug:     &slug,
		Address:  req.Address,
		City:     req.City,
		State:    req.State,
		Zipcode:  req.Zipcode,
		Verified: isAdmin, // Admins create verified venues, non-admins require approval
		Social: models.Social{
			Instagram:  req.Instagram,
			Facebook:   req.Facebook,
			Twitter:    req.Twitter,
			YouTube:    req.YouTube,
			Spotify:    req.Spotify,
			SoundCloud: req.SoundCloud,
			Bandcamp:   req.Bandcamp,
			Website:    req.Website,
		},
	}

	if err := s.db.Create(venue).Error; err != nil {
		return nil, fmt.Errorf("failed to create venue: %w", err)
	}

	return s.buildVenueResponse(venue), nil
}

// GetVenue retrieves a venue by ID
func (s *VenueService) GetVenue(venueID uint) (*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue models.Venue
	err := s.db.First(&venue, venueID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	return s.buildVenueResponse(&venue), nil
}

// GetVenueBySlug retrieves a venue by slug
func (s *VenueService) GetVenueBySlug(slug string) (*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue models.Venue
	err := s.db.Where("slug = ?", slug).First(&venue).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrVenueNotFound(0)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	return s.buildVenueResponse(&venue), nil
}

// GetVenues retrieves venues with optional filtering
func (s *VenueService) GetVenues(filters map[string]interface{}) ([]*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db

	// Apply filters
	if state, ok := filters["state"].(string); ok && state != "" {
		query = query.Where("state = ?", state)
	}
	if city, ok := filters["city"].(string); ok && city != "" {
		query = query.Where("city = ?", city)
	}
	if name, ok := filters["name"].(string); ok && name != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+name+"%")
	}

	// Default ordering by name
	query = query.Order("name ASC")

	var venues []models.Venue
	err := query.Find(&venues).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get venues: %w", err)
	}

	// Build responses
	responses := make([]*VenueDetailResponse, len(venues))
	for i, venue := range venues {
		responses[i] = s.buildVenueResponse(&venue)
	}

	return responses, nil
}

// UpdateVenue updates an existing venue
func (s *VenueService) UpdateVenue(venueID uint, updates map[string]interface{}) (*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if name/city is being updated and if it conflicts with existing venue
	// Get current venue to check its city
	var currentVenue models.Venue
	err := s.db.First(&currentVenue, venueID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// Determine what values to check
	var checkName string
	var checkCity string

	// If name is being updated, use new name, otherwise use current
	if name, ok := updates["name"].(string); ok && name != "" {
		checkName = name
	} else {
		checkName = currentVenue.Name
	}

	// If city is being updated, use new city, otherwise use current
	if city, ok := updates["city"].(string); ok && city != "" {
		checkCity = city
	} else {
		checkCity = currentVenue.City
	}

	// Check for duplicate venue with same name and city (excluding current venue)
	var existingVenue models.Venue
	err = s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?) AND id != ?", checkName, checkCity, venueID).First(&existingVenue).Error
	if err == nil {
		return nil, fmt.Errorf("venue with name '%s' already exists in %s", checkName, checkCity)
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Update the venue
	err = s.db.Model(&models.Venue{}).Where("id = ?", venueID).Updates(updates).Error
	if err != nil {
		return nil, fmt.Errorf("failed to update venue: %w", err)
	}

	return s.GetVenue(venueID)
}

// DeleteVenue deletes a venue
func (s *VenueService) DeleteVenue(venueID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if venue exists
	var venue models.Venue
	err := s.db.First(&venue, venueID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrVenueNotFound(venueID)
		}
		return fmt.Errorf("failed to get venue: %w", err)
	}

	// Check if venue is associated with any shows
	var count int64
	err = s.db.Model(&models.ShowVenue{}).Where("venue_id = ?", venueID).Count(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check venue associations: %w", err)
	}

	if count > 0 {
		return apperrors.ErrVenueHasShows(venueID, count)
	}

	// Delete the venue
	err = s.db.Delete(&venue).Error
	if err != nil {
		return fmt.Errorf("failed to delete venue: %w", err)
	}

	return nil
}

func (venueService *VenueService) SearchVenues(query string) ([]*VenueDetailResponse, error) {
	if venueService.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if query == "" {
		return []*VenueDetailResponse{}, nil
	}

	var venues []models.Venue
	var err error

	if len(query) <= 2 {
		err = venueService.db.
			Where("LOWER(name) LIKE LOWER(?)", query+"%").
			Order("name ASC").
			Limit(10).
			Find(&venues).Error
	} else {
		err = venueService.db.
			Select("venues.*, similarity(name, ?) as sim_score", query).
			Where("name ILIKE ? OR name % ?", "%"+query+"%", query).
			Order("sim_score DESC, name ASC").
			Limit(10).
			Find(&venues).Error
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search venues: %w", err)
	}

	responses := make([]*VenueDetailResponse, len(venues))
	for i, venue := range venues {
		responses[i] = venueService.buildVenueResponse(&venue)
	}

	return responses, nil
}

// FindOrCreateVenue finds an existing venue by name and city or creates a new one.
// This method can be used within a transaction context by passing a *gorm.DB.
// If tx is nil, it uses the service's default database connection.
// City and state are required - this function will return an error if they're empty.
// If isAdmin is true, new venues are automatically verified.
// If isAdmin is false, new venues require admin approval (verified = false).
// Returns the venue and a boolean indicating if it was newly created.
func (s *VenueService) FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*models.Venue, bool, error) {
	// Use provided db or fall back to service's db
	query := db
	if query == nil {
		query = s.db
	}

	if query == nil {
		return nil, false, fmt.Errorf("database not initialized")
	}

	// Validate required fields
	if name == "" {
		return nil, false, fmt.Errorf("venue name is required")
	}
	if city == "" {
		return nil, false, fmt.Errorf("venue city is required")
	}
	if state == "" {
		return nil, false, fmt.Errorf("venue state is required")
	}

	// Check if venue already exists by name and city
	var venue models.Venue
	err := query.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", name, city).First(&venue).Error

	if err == nil {
		// Venue exists — backfill slug if missing
		if venue.Slug == nil {
			baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				query.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			venue.Slug = &slug
			query.Model(&venue).Update("slug", slug)
		}
		return &venue, false, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, false, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Venue doesn't exist, create it - verified if created by admin, unverified otherwise
	// Generate unique slug
	baseSlug := utils.GenerateVenueSlug(name, city, state)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		query.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	venue = models.Venue{
		Name:     name,
		Slug:     &slug,
		Address:  address,
		City:     city,
		State:    state,
		Zipcode:  zipcode,
		Verified: isAdmin, // Admins create verified venues, non-admins require approval
		Social:   models.Social{}, // Empty social fields
	}

	if err := query.Create(&venue).Error; err != nil {
		return nil, false, fmt.Errorf("failed to create venue: %w", err)
	}

	return &venue, true, nil // true = newly created
}

// VerifyVenue marks a venue as verified by an admin.
func (s *VenueService) VerifyVenue(venueID uint) (*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get the venue
	var venue models.Venue
	err := s.db.First(&venue, venueID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// Check if already verified
	if venue.Verified {
		return s.buildVenueResponse(&venue), nil
	}

	// Generate slug if missing
	updates := map[string]interface{}{"verified": true}
	if venue.Slug == nil {
		baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
	}

	// Update verified status (and slug if generated)
	if err := s.db.Model(&venue).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to verify venue: %w", err)
	}

	// Reload to get updated data
	if err := s.db.First(&venue, venueID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload venue: %w", err)
	}

	return s.buildVenueResponse(&venue), nil
}

// buildVenueResponse converts a Venue model to VenueDetailResponse
func (s *VenueService) buildVenueResponse(venue *models.Venue) *VenueDetailResponse {
	slug := ""
	if venue.Slug != nil {
		slug = *venue.Slug
	}
	// Hide address and zipcode for unverified venues
	var address *string
	var zipcode *string
	if venue.Verified {
		address = venue.Address
		zipcode = venue.Zipcode
	}

	return &VenueDetailResponse{
		ID:          venue.ID,
		Slug:        slug,
		Name:        venue.Name,
		Address:     address,
		City:        venue.City,
		State:       venue.State,
		Zipcode:     zipcode,
		Verified:    venue.Verified,
		SubmittedBy: venue.SubmittedBy,
		Social: SocialResponse{
			Instagram:  venue.Social.Instagram,
			Facebook:   venue.Social.Facebook,
			Twitter:    venue.Social.Twitter,
			YouTube:    venue.Social.YouTube,
			Spotify:    venue.Social.Spotify,
			SoundCloud: venue.Social.SoundCloud,
			Bandcamp:   venue.Social.Bandcamp,
			Website:    venue.Social.Website,
		},
		CreatedAt: venue.CreatedAt,
		UpdatedAt: venue.UpdatedAt,
	}
}

// VenueWithShowCountResponse represents a venue with its upcoming show count
type VenueWithShowCountResponse struct {
	VenueDetailResponse
	UpcomingShowCount int `json:"upcoming_show_count"`
}

// VenueListFilters contains filter options for listing venues
type VenueListFilters struct {
	State    string
	City     string
	Verified *bool
}

// VenueWithCount is used internally for querying venues with their show counts
type VenueWithCount struct {
	models.Venue
	UpcomingShowCount int64 `gorm:"column:upcoming_show_count"`
}

// GetVenuesWithShowCounts retrieves verified venues with their upcoming show counts.
// Results are sorted by upcoming show count (descending), then by name (ascending),
// so venues with upcoming shows appear first.
func (s *VenueService) GetVenuesWithShowCounts(filters VenueListFilters, limit, offset int) ([]*VenueWithShowCountResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	// Build the base query with show count subquery
	// This allows us to sort by show count while also paginating correctly
	subquery := s.db.Table("show_venues").
		Select("show_venues.venue_id, COUNT(*) as show_count").
		Joins("JOIN shows ON show_venues.show_id = shows.id").
		Where("shows.event_date >= ? AND shows.status = ?", now, models.ShowStatusApproved).
		Group("show_venues.venue_id")

	// Start with verified venues only for public display
	query := s.db.Table("venues").
		Select("venues.*, COALESCE(sc.show_count, 0) as upcoming_show_count").
		Joins("LEFT JOIN (?) as sc ON venues.id = sc.venue_id", subquery).
		Where("venues.verified = ?", true)

	// Apply optional filters
	if filters.State != "" {
		query = query.Where("venues.state = ?", filters.State)
	}
	if filters.City != "" {
		query = query.Where("venues.city = ?", filters.City)
	}

	// Get total count of matching venues
	var total int64
	countQuery := s.db.Table("venues").Where("verified = ?", true)
	if filters.State != "" {
		countQuery = countQuery.Where("state = ?", filters.State)
	}
	if filters.City != "" {
		countQuery = countQuery.Where("city = ?", filters.City)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count venues: %w", err)
	}

	// Get venues with pagination, sorted by show count (desc) then name (asc)
	var venuesWithCount []VenueWithCount
	if err := query.Order("upcoming_show_count DESC, venues.name ASC").Limit(limit).Offset(offset).Find(&venuesWithCount).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get venues: %w", err)
	}

	// Build responses
	responses := make([]*VenueWithShowCountResponse, len(venuesWithCount))
	for i, vc := range venuesWithCount {
		responses[i] = &VenueWithShowCountResponse{
			VenueDetailResponse: *s.buildVenueResponse(&vc.Venue),
			UpcomingShowCount:   int(vc.UpcomingShowCount),
		}
	}

	return responses, total, nil
}

// VenueShowResponse represents a show in the venue shows endpoint
type VenueShowResponse struct {
	ID             uint             `json:"id"`
	Title          string           `json:"title"`
	EventDate      time.Time        `json:"event_date"`
	City           *string          `json:"city"`
	State          *string          `json:"state"`
	Price          *float64         `json:"price"`
	AgeRequirement *string          `json:"age_requirement"`
	Artists        []ArtistResponse `json:"artists"`
}

// GetUpcomingShowsForVenue retrieves upcoming shows at a specific venue.
// Only returns approved shows with event_date >= today in the specified timezone.
// Deprecated: Use GetShowsForVenue with timeFilter="upcoming" instead.
func (s *VenueService) GetUpcomingShowsForVenue(venueID uint, timezone string, limit int) ([]*VenueShowResponse, int64, error) {
	return s.GetShowsForVenue(venueID, timezone, limit, "upcoming")
}

// GetShowsForVenue retrieves shows at a specific venue with time filtering.
// timeFilter can be: "upcoming" (event_date >= today), "past" (event_date < today), or "all"
// Only returns approved shows.
func (s *VenueService) GetShowsForVenue(venueID uint, timezone string, limit int, timeFilter string) ([]*VenueShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Verify venue exists
	var venue models.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, 0, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, 0, fmt.Errorf("failed to get venue: %w", err)
	}

	// Load timezone, default to UTC
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	// Get start of today in the user's timezone, then convert to UTC for query
	now := time.Now().In(loc)
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	startOfTodayUTC := startOfToday.UTC()

	// Build base query
	baseQuery := s.db.Table("show_venues").
		Joins("JOIN shows ON show_venues.show_id = shows.id").
		Where("show_venues.venue_id = ? AND shows.status = ?", venueID, models.ShowStatusApproved)

	// Apply time filter
	var dateCondition string
	var orderDirection string
	switch timeFilter {
	case "past":
		baseQuery = baseQuery.Where("shows.event_date < ?", startOfTodayUTC)
		dateCondition = "shows.event_date < ?"
		orderDirection = "shows.event_date DESC" // Most recent past shows first
	case "all":
		dateCondition = "" // No date filter
		orderDirection = "shows.event_date ASC"
	default: // "upcoming"
		baseQuery = baseQuery.Where("shows.event_date >= ?", startOfTodayUTC)
		dateCondition = "shows.event_date >= ?"
		orderDirection = "shows.event_date ASC" // Soonest upcoming shows first
	}

	// Count total shows matching the filter
	var total int64
	countQuery := s.db.Table("show_venues").
		Joins("JOIN shows ON show_venues.show_id = shows.id").
		Where("show_venues.venue_id = ? AND shows.status = ?", venueID, models.ShowStatusApproved)
	if dateCondition != "" {
		countQuery = countQuery.Where(dateCondition, startOfTodayUTC)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count shows: %w", err)
	}

	// Get show IDs with limit
	var showIDs []uint
	showQuery := s.db.Table("show_venues").
		Select("show_venues.show_id").
		Joins("JOIN shows ON show_venues.show_id = shows.id").
		Where("show_venues.venue_id = ? AND shows.status = ?", venueID, models.ShowStatusApproved)
	if dateCondition != "" {
		showQuery = showQuery.Where(dateCondition, startOfTodayUTC)
	}
	if err := showQuery.Order(orderDirection).Limit(limit).Pluck("show_venues.show_id", &showIDs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get show IDs: %w", err)
	}

	// Fetch full show data
	var shows []models.Show
	if len(showIDs) > 0 {
		if err := s.db.Where("id IN ?", showIDs).Order(orderDirection).Find(&shows).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to get shows: %w", err)
		}
	}

	// Batch-load all ShowArtist records and collect artist IDs
	allShowArtists := make(map[uint][]models.ShowArtist)
	var allArtistIDs []uint
	if len(shows) > 0 {
		var showArtistRecords []models.ShowArtist
		showIDsForArtists := make([]uint, len(shows))
		for i, show := range shows {
			showIDsForArtists[i] = show.ID
		}
		s.db.Where("show_id IN ?", showIDsForArtists).Order("position ASC").Find(&showArtistRecords)
		for _, sa := range showArtistRecords {
			allShowArtists[sa.ShowID] = append(allShowArtists[sa.ShowID], sa)
			allArtistIDs = append(allArtistIDs, sa.ArtistID)
		}
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

	// Build responses using map lookup
	responses := make([]*VenueShowResponse, len(shows))
	for i, show := range shows {
		artists := make([]ArtistResponse, 0)
		for _, sa := range allShowArtists[show.ID] {
			artist, ok := artistMap[sa.ArtistID]
			if !ok {
				continue
			}
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
			var slug string
			if artist.Slug != nil {
				slug = *artist.Slug
			}
			artists = append(artists, ArtistResponse{
				ID:          artist.ID,
				Slug:        slug,
				Name:        artist.Name,
				State:       artist.State,
				City:        artist.City,
				IsHeadliner: &isHeadliner,
				IsNewArtist: &isNewArtist,
				Socials:     socials,
			})
		}

		responses[i] = &VenueShowResponse{
			ID:             show.ID,
			Title:          show.Title,
			EventDate:      show.EventDate,
			City:           show.City,
			State:          show.State,
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			Artists:        artists,
		}
	}

	return responses, total, nil
}

// VenueCityResponse represents a city with venue count for filtering
type VenueCityResponse struct {
	City       string `json:"city"`
	State      string `json:"state"`
	VenueCount int    `json:"venue_count"`
}

// GetVenueCities returns distinct cities that have verified venues, with venue counts.
// Results are sorted by venue count (descending) to show most active cities first.
func (s *VenueService) GetVenueCities() ([]*VenueCityResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	type CityResult struct {
		City       string
		State      string
		VenueCount int64
	}

	var results []CityResult
	err := s.db.Table("venues").
		Select("city, state, COUNT(*) as venue_count").
		Where("verified = ?", true).
		Group("city, state").
		Order("venue_count DESC, city ASC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get venue cities: %w", err)
	}

	responses := make([]*VenueCityResponse, len(results))
	for i, r := range results {
		responses[i] = &VenueCityResponse{
			City:       r.City,
			State:      r.State,
			VenueCount: int(r.VenueCount),
		}
	}

	return responses, nil
}

// ============================================================================
// Pending Venue Edit Types and Methods
// ============================================================================

// VenueEditRequest represents the data for updating a venue
type VenueEditRequest struct {
	Name       *string `json:"name"`
	Address    *string `json:"address"`
	City       *string `json:"city"`
	State      *string `json:"state"`
	Zipcode    *string `json:"zipcode"`
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// PendingVenueEditResponse represents a pending venue edit returned to clients
type PendingVenueEditResponse struct {
	ID          uint                    `json:"id"`
	VenueID     uint                    `json:"venue_id"`
	SubmittedBy uint                    `json:"submitted_by"`
	Status      models.VenueEditStatus  `json:"status"`

	// Proposed changes
	Name       *string `json:"name,omitempty"`
	Address    *string `json:"address,omitempty"`
	City       *string `json:"city,omitempty"`
	State      *string `json:"state,omitempty"`
	Zipcode    *string `json:"zipcode,omitempty"`
	Instagram  *string `json:"instagram,omitempty"`
	Facebook   *string `json:"facebook,omitempty"`
	Twitter    *string `json:"twitter,omitempty"`
	YouTube    *string `json:"youtube,omitempty"`
	Spotify    *string `json:"spotify,omitempty"`
	SoundCloud *string `json:"soundcloud,omitempty"`
	Bandcamp   *string `json:"bandcamp,omitempty"`
	Website    *string `json:"website,omitempty"`

	// Workflow fields
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	ReviewedBy      *uint      `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Embedded venue info for context
	Venue         *VenueDetailResponse `json:"venue,omitempty"`
	SubmitterName *string              `json:"submitter_name,omitempty"`
	ReviewerName  *string              `json:"reviewer_name,omitempty"`
}

// buildPendingVenueEditResponse converts a PendingVenueEdit model to response
func (s *VenueService) buildPendingVenueEditResponse(edit *models.PendingVenueEdit, includeVenue bool) *PendingVenueEditResponse {
	resp := &PendingVenueEditResponse{
		ID:              edit.ID,
		VenueID:         edit.VenueID,
		SubmittedBy:     edit.SubmittedBy,
		Status:          edit.Status,
		Name:            edit.Name,
		Address:         edit.Address,
		City:            edit.City,
		State:           edit.State,
		Zipcode:         edit.Zipcode,
		Instagram:       edit.Instagram,
		Facebook:        edit.Facebook,
		Twitter:         edit.Twitter,
		YouTube:         edit.YouTube,
		Spotify:         edit.Spotify,
		SoundCloud:      edit.SoundCloud,
		Bandcamp:        edit.Bandcamp,
		Website:         edit.Website,
		RejectionReason: edit.RejectionReason,
		ReviewedBy:      edit.ReviewedBy,
		ReviewedAt:      edit.ReviewedAt,
		CreatedAt:       edit.CreatedAt,
		UpdatedAt:       edit.UpdatedAt,
	}

	if includeVenue && edit.Venue.ID != 0 {
		resp.Venue = s.buildVenueResponse(&edit.Venue)
	}

	if edit.SubmittedByUser.ID != 0 {
		submitterName := buildUserDisplayName(&edit.SubmittedByUser)
		resp.SubmitterName = &submitterName
	}

	if edit.ReviewedByUser != nil && edit.ReviewedByUser.ID != 0 {
		reviewerName := buildUserDisplayName(edit.ReviewedByUser)
		resp.ReviewerName = &reviewerName
	}

	return resp
}

// buildUserDisplayName creates a display name from user's first/last name or email
func buildUserDisplayName(user *models.User) string {
	if user.FirstName != nil && user.LastName != nil {
		return *user.FirstName + " " + *user.LastName
	}
	if user.FirstName != nil {
		return *user.FirstName
	}
	if user.Username != nil {
		return *user.Username
	}
	if user.Email != nil {
		return *user.Email
	}
	return "Unknown User"
}

// CreatePendingVenueEdit creates a new pending edit for a venue (for non-admin users)
func (s *VenueService) CreatePendingVenueEdit(venueID uint, userID uint, req *VenueEditRequest) (*PendingVenueEditResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if venue exists
	var venue models.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// Check if user already has a pending edit for this venue
	var existingEdit models.PendingVenueEdit
	err := s.db.Where("venue_id = ? AND submitted_by = ? AND status = ?", venueID, userID, models.VenueEditStatusPending).First(&existingEdit).Error
	if err == nil {
		return nil, apperrors.ErrVenuePendingEditExists(venueID)
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check existing edits: %w", err)
	}

	// Create the pending edit
	pendingEdit := &models.PendingVenueEdit{
		VenueID:     venueID,
		SubmittedBy: userID,
		Name:        req.Name,
		Address:     req.Address,
		City:        req.City,
		State:       req.State,
		Zipcode:     req.Zipcode,
		Instagram:   req.Instagram,
		Facebook:    req.Facebook,
		Twitter:     req.Twitter,
		YouTube:     req.YouTube,
		Spotify:     req.Spotify,
		SoundCloud:  req.SoundCloud,
		Bandcamp:    req.Bandcamp,
		Website:     req.Website,
		Status:      models.VenueEditStatusPending,
	}

	if err := s.db.Create(pendingEdit).Error; err != nil {
		return nil, fmt.Errorf("failed to create pending edit: %w", err)
	}

	// Load the edit with relationships
	if err := s.db.Preload("Venue").Preload("SubmittedByUser").First(pendingEdit, pendingEdit.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load pending edit: %w", err)
	}

	return s.buildPendingVenueEditResponse(pendingEdit, true), nil
}

// GetPendingEditForVenue retrieves a user's pending edit for a specific venue
func (s *VenueService) GetPendingEditForVenue(venueID uint, userID uint) (*PendingVenueEditResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var edit models.PendingVenueEdit
	err := s.db.Preload("Venue").Preload("SubmittedByUser").Preload("ReviewedByUser").
		Where("venue_id = ? AND submitted_by = ? AND status = ?", venueID, userID, models.VenueEditStatusPending).
		First(&edit).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // No pending edit found
		}
		return nil, fmt.Errorf("failed to get pending edit: %w", err)
	}

	return s.buildPendingVenueEditResponse(&edit, true), nil
}

// GetPendingVenueEdits retrieves all pending venue edits for admin review
func (s *VenueService) GetPendingVenueEdits(limit, offset int) ([]*PendingVenueEditResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	var total int64
	if err := s.db.Model(&models.PendingVenueEdit{}).Where("status = ?", models.VenueEditStatusPending).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count pending edits: %w", err)
	}

	var edits []models.PendingVenueEdit
	err := s.db.Preload("Venue").Preload("SubmittedByUser").
		Where("status = ?", models.VenueEditStatusPending).
		Order("created_at ASC").
		Limit(limit).Offset(offset).
		Find(&edits).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get pending edits: %w", err)
	}

	responses := make([]*PendingVenueEditResponse, len(edits))
	for i, edit := range edits {
		responses[i] = s.buildPendingVenueEditResponse(&edit, true)
	}

	return responses, total, nil
}

// GetPendingVenueEdit retrieves a single pending venue edit by ID
func (s *VenueService) GetPendingVenueEdit(editID uint) (*PendingVenueEditResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var edit models.PendingVenueEdit
	err := s.db.Preload("Venue").Preload("SubmittedByUser").Preload("ReviewedByUser").
		First(&edit, editID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("pending edit not found")
		}
		return nil, fmt.Errorf("failed to get pending edit: %w", err)
	}

	return s.buildPendingVenueEditResponse(&edit, true), nil
}

// ApproveVenueEdit approves a pending venue edit and applies changes to the venue
func (s *VenueService) ApproveVenueEdit(editID uint, reviewerID uint) (*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get the pending edit
	var edit models.PendingVenueEdit
	if err := s.db.First(&edit, editID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("pending edit not found")
		}
		return nil, fmt.Errorf("failed to get pending edit: %w", err)
	}

	if edit.Status != models.VenueEditStatusPending {
		return nil, fmt.Errorf("edit has already been %s", edit.Status)
	}

	// Get the venue
	var venue models.Venue
	if err := s.db.First(&venue, edit.VenueID).Error; err != nil {
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// Start transaction
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Apply changes to venue (only non-nil fields)
	updates := make(map[string]interface{})
	if edit.Name != nil {
		updates["name"] = *edit.Name
	}
	if edit.Address != nil {
		updates["address"] = *edit.Address
	}
	if edit.City != nil {
		updates["city"] = *edit.City
	}
	if edit.State != nil {
		updates["state"] = *edit.State
	}
	if edit.Zipcode != nil {
		updates["zipcode"] = *edit.Zipcode
	}
	if edit.Instagram != nil {
		updates["instagram"] = *edit.Instagram
	}
	if edit.Facebook != nil {
		updates["facebook"] = *edit.Facebook
	}
	if edit.Twitter != nil {
		updates["twitter"] = *edit.Twitter
	}
	if edit.YouTube != nil {
		updates["youtube"] = *edit.YouTube
	}
	if edit.Spotify != nil {
		updates["spotify"] = *edit.Spotify
	}
	if edit.SoundCloud != nil {
		updates["soundcloud"] = *edit.SoundCloud
	}
	if edit.Bandcamp != nil {
		updates["bandcamp"] = *edit.Bandcamp
	}
	if edit.Website != nil {
		updates["website"] = *edit.Website
	}

	if len(updates) > 0 {
		if err := tx.Model(&venue).Updates(updates).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update venue: %w", err)
		}
	}

	// Update the pending edit status
	now := time.Now()
	if err := tx.Model(&edit).Updates(map[string]interface{}{
		"status":      models.VenueEditStatusApproved,
		"reviewed_by": reviewerID,
		"reviewed_at": now,
	}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update pending edit: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Reload the venue
	if err := s.db.First(&venue, edit.VenueID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload venue: %w", err)
	}

	return s.buildVenueResponse(&venue), nil
}

// RejectVenueEdit rejects a pending venue edit
func (s *VenueService) RejectVenueEdit(editID uint, reviewerID uint, reason string) (*PendingVenueEditResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get the pending edit
	var edit models.PendingVenueEdit
	if err := s.db.First(&edit, editID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("pending edit not found")
		}
		return nil, fmt.Errorf("failed to get pending edit: %w", err)
	}

	if edit.Status != models.VenueEditStatusPending {
		return nil, fmt.Errorf("edit has already been %s", edit.Status)
	}

	// Update the pending edit status
	now := time.Now()
	if err := s.db.Model(&edit).Updates(map[string]interface{}{
		"status":           models.VenueEditStatusRejected,
		"rejection_reason": reason,
		"reviewed_by":      reviewerID,
		"reviewed_at":      now,
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to reject pending edit: %w", err)
	}

	// Reload with relationships
	if err := s.db.Preload("Venue").Preload("SubmittedByUser").Preload("ReviewedByUser").First(&edit, editID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload pending edit: %w", err)
	}

	return s.buildPendingVenueEditResponse(&edit, true), nil
}

// CancelPendingVenueEdit allows a user to cancel their own pending edit
func (s *VenueService) CancelPendingVenueEdit(editID uint, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Get the pending edit
	var edit models.PendingVenueEdit
	if err := s.db.First(&edit, editID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("pending edit not found")
		}
		return fmt.Errorf("failed to get pending edit: %w", err)
	}

	// Verify ownership
	if edit.SubmittedBy != userID {
		return fmt.Errorf("you can only cancel your own pending edits")
	}

	if edit.Status != models.VenueEditStatusPending {
		return fmt.Errorf("edit has already been %s", edit.Status)
	}

	// Delete the pending edit
	if err := s.db.Delete(&edit).Error; err != nil {
		return fmt.Errorf("failed to cancel pending edit: %w", err)
	}

	return nil
}

// GetVenueModel retrieves a raw venue model (used by handlers to check ownership)
func (s *VenueService) GetVenueModel(venueID uint) (*models.Venue, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue models.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	return &venue, nil
}

// UnverifiedVenueResponse represents an unverified venue for admin review
type UnverifiedVenueResponse struct {
	ID          uint      `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Address     *string   `json:"address"`
	City        string    `json:"city"`
	State       string    `json:"state"`
	Zipcode     *string   `json:"zipcode"`
	SubmittedBy *uint     `json:"submitted_by"`
	CreatedAt   time.Time `json:"created_at"`
	ShowCount   int       `json:"show_count"` // Number of shows using this venue
}

// GetUnverifiedVenues retrieves all unverified venues for admin review.
// Results are sorted by creation date (newest first).
func (s *VenueService) GetUnverifiedVenues(limit, offset int) ([]*UnverifiedVenueResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count of unverified venues
	var total int64
	if err := s.db.Model(&models.Venue{}).Where("verified = ?", false).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count unverified venues: %w", err)
	}

	// Build query with show count subquery
	subquery := s.db.Table("show_venues").
		Select("venue_id, COUNT(*) as show_count").
		Group("venue_id")

	var results []struct {
		models.Venue
		ShowCount int64 `gorm:"column:show_count"`
	}

	err := s.db.Table("venues").
		Select("venues.*, COALESCE(sc.show_count, 0) as show_count").
		Joins("LEFT JOIN (?) as sc ON venues.id = sc.venue_id", subquery).
		Where("venues.verified = ?", false).
		Order("venues.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&results).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get unverified venues: %w", err)
	}

	// Build responses
	responses := make([]*UnverifiedVenueResponse, len(results))
	for i, r := range results {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		responses[i] = &UnverifiedVenueResponse{
			ID:          r.ID,
			Slug:        slug,
			Name:        r.Name,
			Address:     r.Address,
			City:        r.City,
			State:       r.State,
			Zipcode:     r.Zipcode,
			SubmittedBy: r.SubmittedBy,
			CreatedAt:   r.CreatedAt,
			ShowCount:   int(r.ShowCount),
		}
	}

	return responses, total, nil
}
