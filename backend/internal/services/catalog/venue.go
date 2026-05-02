package catalog

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
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

// CreateVenue creates a new venue
// If isAdmin is true, the venue is automatically verified.
// If isAdmin is false, the venue requires admin approval (verified = false).
func (s *VenueService) CreateVenue(req *contracts.CreateVenueRequest, isAdmin bool) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if venue already exists (same name in same city)
	var existingVenue catalogm.Venue
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
		s.db.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	// Create the venue - verified if created by admin, unverified otherwise
	venue := &catalogm.Venue{
		Name:        req.Name,
		Slug:        &slug,
		Address:     req.Address,
		City:        req.City,
		State:       req.State,
		Country:     req.Country,
		Zipcode:     req.Zipcode,
		Verified:    isAdmin, // Admins create verified venues, non-admins require approval
		SubmittedBy: req.SubmittedBy,
		Social: catalogm.Social{
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
func (s *VenueService) GetVenue(venueID uint) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
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
func (s *VenueService) GetVenueBySlug(slug string) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
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
func (s *VenueService) GetVenues(filters map[string]interface{}) ([]*contracts.VenueDetailResponse, error) {
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
	if tf, ok := filters["tag_filter"].(TagFilter); ok {
		query = ApplyTagFilter(query, s.db, catalogm.TagEntityVenue, "venues.id", tf)
	}

	// Default ordering by name
	query = query.Order("name ASC")

	var venues []catalogm.Venue
	err := query.Find(&venues).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get venues: %w", err)
	}

	// Build responses
	responses := make([]*contracts.VenueDetailResponse, len(venues))
	for i, venue := range venues {
		responses[i] = s.buildVenueResponse(&venue)
	}

	return responses, nil
}

// UpdateVenue updates an existing venue
func (s *VenueService) UpdateVenue(venueID uint, updates map[string]interface{}) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if name/city is being updated and if it conflicts with existing venue
	// Get current venue to check its city
	var currentVenue catalogm.Venue
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
	var existingVenue catalogm.Venue
	err = s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?) AND id != ?", checkName, checkCity, venueID).First(&existingVenue).Error
	if err == nil {
		return nil, fmt.Errorf("venue with name '%s' already exists in %s", checkName, checkCity)
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Update the venue
	err = s.db.Model(&catalogm.Venue{}).Where("id = ?", venueID).Updates(updates).Error
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
	var venue catalogm.Venue
	err := s.db.First(&venue, venueID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrVenueNotFound(venueID)
		}
		return fmt.Errorf("failed to get venue: %w", err)
	}

	// Check if venue is associated with any shows
	var count int64
	err = s.db.Model(&catalogm.ShowVenue{}).Where("venue_id = ?", venueID).Count(&count).Error
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

func (venueService *VenueService) SearchVenues(query string) ([]*contracts.VenueDetailResponse, error) {
	if venueService.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if query == "" {
		return []*contracts.VenueDetailResponse{}, nil
	}

	var venues []catalogm.Venue
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

	responses := make([]*contracts.VenueDetailResponse, len(venues))
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
func (s *VenueService) FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*catalogm.Venue, bool, error) {
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
	var venue catalogm.Venue
	err := query.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", name, city).First(&venue).Error

	if err == nil {
		// Venue exists — backfill slug if missing
		if venue.Slug == nil {
			baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				query.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
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
		query.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	venue = catalogm.Venue{
		Name:     name,
		Slug:     &slug,
		Address:  address,
		City:     city,
		State:    state,
		Zipcode:  zipcode,
		Verified: isAdmin,           // Admins create verified venues, non-admins require approval
		Social:   catalogm.Social{}, // Empty social fields
	}

	if err := query.Create(&venue).Error; err != nil {
		return nil, false, fmt.Errorf("failed to create venue: %w", err)
	}

	return &venue, true, nil // true = newly created
}

// VerifyVenue marks a venue as verified by an admin.
func (s *VenueService) VerifyVenue(venueID uint) (*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get the venue
	var venue catalogm.Venue
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
			s.db.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
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

// buildVenueResponse converts a Venue model to contracts.VenueDetailResponse
func (s *VenueService) buildVenueResponse(venue *catalogm.Venue) *contracts.VenueDetailResponse {
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

	return &contracts.VenueDetailResponse{
		ID:          venue.ID,
		Slug:        slug,
		Name:        venue.Name,
		Address:     address,
		City:        venue.City,
		State:       venue.State,
		Country:     venue.Country,
		Zipcode:     zipcode,
		Description: venue.Description,
		ImageURL:    venue.ImageURL,
		Verified:    venue.Verified,
		SubmittedBy: venue.SubmittedBy,
		Social: contracts.SocialResponse{
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

// contracts.VenueWithShowCountResponse represents a venue with its upcoming show count

// VenueWithCount is used internally for querying venues with their show counts
type VenueWithCount struct {
	catalogm.Venue
	UpcomingShowCount int64 `gorm:"column:upcoming_show_count"`
}

// GetVenuesWithShowCounts retrieves verified venues with their upcoming show counts.
// Results are sorted by upcoming show count (descending), then by name (ascending),
// so venues with upcoming shows appear first.
func (s *VenueService) GetVenuesWithShowCounts(filters contracts.VenueListFilters, limit, offset int) ([]*contracts.VenueWithShowCountResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	// Build the base query with show count subquery
	// This allows us to sort by show count while also paginating correctly
	subquery := s.db.Table("show_venues").
		Select("show_venues.venue_id, COUNT(*) as show_count").
		Joins("JOIN shows ON show_venues.show_id = shows.id").
		Where("shows.event_date >= ? AND shows.status = ?", now, catalogm.ShowStatusApproved).
		Group("show_venues.venue_id")

	// Start with verified venues only for public display
	query := s.db.Table("venues").
		Select("venues.*, COALESCE(sc.show_count, 0) as upcoming_show_count").
		Joins("LEFT JOIN (?) as sc ON venues.id = sc.venue_id", subquery).
		Where("venues.verified = ?", true)

	// Apply optional filters
	if len(filters.Cities) > 0 {
		var conditions []string
		var args []interface{}
		for _, cs := range filters.Cities {
			if cs.City != "" && cs.State != "" {
				conditions = append(conditions, "(venues.city = ? AND venues.state = ?)")
				args = append(args, cs.City, cs.State)
			}
		}
		if len(conditions) > 0 {
			query = query.Where(strings.Join(conditions, " OR "), args...)
		}
	} else {
		if filters.State != "" {
			query = query.Where("venues.state = ?", filters.State)
		}
		if filters.City != "" {
			query = query.Where("venues.city = ?", filters.City)
		}
	}
	tf := TagFilter{TagSlugs: filters.TagSlugs, MatchAny: filters.TagMatchAny}
	query = ApplyTagFilter(query, s.db, catalogm.TagEntityVenue, "venues.id", tf)

	// Get total count of matching venues
	var total int64
	countQuery := s.db.Table("venues").Where("verified = ?", true)
	if len(filters.Cities) > 0 {
		var conditions []string
		var args []interface{}
		for _, cs := range filters.Cities {
			if cs.City != "" && cs.State != "" {
				conditions = append(conditions, "(city = ? AND state = ?)")
				args = append(args, cs.City, cs.State)
			}
		}
		if len(conditions) > 0 {
			countQuery = countQuery.Where(strings.Join(conditions, " OR "), args...)
		}
	} else {
		if filters.State != "" {
			countQuery = countQuery.Where("state = ?", filters.State)
		}
		if filters.City != "" {
			countQuery = countQuery.Where("city = ?", filters.City)
		}
	}
	countQuery = ApplyTagFilter(countQuery, s.db, catalogm.TagEntityVenue, "venues.id", tf)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count venues: %w", err)
	}

	// Get venues with pagination, sorted by show count (desc) then name (asc)
	var venuesWithCount []VenueWithCount
	if err := query.Order("upcoming_show_count DESC, venues.name ASC").Limit(limit).Offset(offset).Find(&venuesWithCount).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get venues: %w", err)
	}

	// Build responses
	responses := make([]*contracts.VenueWithShowCountResponse, len(venuesWithCount))
	for i, vc := range venuesWithCount {
		responses[i] = &contracts.VenueWithShowCountResponse{
			VenueDetailResponse: *s.buildVenueResponse(&vc.Venue),
			UpcomingShowCount:   int(vc.UpcomingShowCount),
		}
	}

	return responses, total, nil
}

// GetUpcomingShowsForVenue retrieves upcoming shows at a specific venue.
// Only returns approved shows with event_date >= today in the specified timezone.
// Deprecated: Use GetShowsForVenue with timeFilter="upcoming" instead.
func (s *VenueService) GetUpcomingShowsForVenue(venueID uint, timezone string, limit int) ([]*contracts.VenueShowResponse, int64, error) {
	return s.GetShowsForVenue(venueID, timezone, limit, "upcoming")
}

// GetShowsForVenue retrieves shows at a specific venue with time filtering.
// timeFilter can be: "upcoming" (event_date >= today), "past" (event_date < today), or "all"
// Only returns approved shows.
func (s *VenueService) GetShowsForVenue(venueID uint, timezone string, limit int, timeFilter string) ([]*contracts.VenueShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Verify venue exists
	var venue catalogm.Venue
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
		Where("show_venues.venue_id = ? AND shows.status = ?", venueID, catalogm.ShowStatusApproved)

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
		Where("show_venues.venue_id = ? AND shows.status = ?", venueID, catalogm.ShowStatusApproved)
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
		Where("show_venues.venue_id = ? AND shows.status = ?", venueID, catalogm.ShowStatusApproved)
	if dateCondition != "" {
		showQuery = showQuery.Where(dateCondition, startOfTodayUTC)
	}
	if err := showQuery.Order(orderDirection).Limit(limit).Pluck("show_venues.show_id", &showIDs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get show IDs: %w", err)
	}

	// Fetch full show data
	var shows []catalogm.Show
	if len(showIDs) > 0 {
		if err := s.db.Where("id IN ?", showIDs).Order(orderDirection).Find(&shows).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to get shows: %w", err)
		}
	}

	// Batch-load all ShowArtist records and collect artist IDs
	allShowArtists := make(map[uint][]catalogm.ShowArtist)
	var allArtistIDs []uint
	if len(shows) > 0 {
		var showArtistRecords []catalogm.ShowArtist
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
	artistMap := make(map[uint]*catalogm.Artist)
	if len(allArtistIDs) > 0 {
		var allArtists []catalogm.Artist
		s.db.Where("id IN ?", allArtistIDs).Find(&allArtists)
		for i := range allArtists {
			artistMap[allArtists[i].ID] = &allArtists[i]
		}
	}

	// Build responses using map lookup
	responses := make([]*contracts.VenueShowResponse, len(shows))
	for i, show := range shows {
		artists := make([]contracts.ArtistResponse, 0)
		for _, sa := range allShowArtists[show.ID] {
			artist, ok := artistMap[sa.ArtistID]
			if !ok {
				continue
			}
			isHeadliner := sa.SetType == "headliner"
			isNewArtist := false
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
			var slug string
			if artist.Slug != nil {
				slug = *artist.Slug
			}
			artists = append(artists, contracts.ArtistResponse{
				ID:          artist.ID,
				Slug:        slug,
				Name:        artist.Name,
				State:       artist.State,
				City:        artist.City,
				IsHeadliner: &isHeadliner,
				SetType:     sa.SetType,
				Position:    sa.Position,
				IsNewArtist: &isNewArtist,
				Socials:     socials,
			})
		}

		responses[i] = &contracts.VenueShowResponse{
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

// contracts.VenueCityResponse represents a city with venue count for filtering

// GetVenueCities returns distinct cities that have verified venues, with venue counts.
// Results are sorted by venue count (descending) to show most active cities first.
func (s *VenueService) GetVenueCities() ([]*contracts.VenueCityResponse, error) {
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

	responses := make([]*contracts.VenueCityResponse, len(results))
	for i, r := range results {
		responses[i] = &contracts.VenueCityResponse{
			City:       r.City,
			State:      r.State,
			VenueCount: int(r.VenueCount),
		}
	}

	return responses, nil
}

// GetVenueGenreProfile returns genre tags derived from artists who have played approved
// shows at this venue. Returns the top 5 genres ranked by distinct artist count.
// Returns empty if the venue has fewer than 10 shows with tagged artists.
func (s *VenueService) GetVenueGenreProfile(venueID uint) ([]contracts.GenreCount, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// First check if venue has enough shows with tagged artists
	var showCount int64
	err := s.db.Raw(`
		SELECT COUNT(DISTINCT sa.show_id)
		FROM show_artists sa
		JOIN show_venues sv ON sv.show_id = sa.show_id
		JOIN shows s ON s.id = sa.show_id
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = sa.artist_id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE sv.venue_id = ? AND s.status = ?
	`, venueID, catalogm.ShowStatusApproved).Scan(&showCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count tagged shows for venue: %w", err)
	}

	if showCount < 10 {
		return []contracts.GenreCount{}, nil
	}

	type genreRow struct {
		TagID uint   `gorm:"column:tag_id"`
		Name  string `gorm:"column:name"`
		Slug  string `gorm:"column:slug"`
		Count int    `gorm:"column:count"`
	}

	var rows []genreRow
	err = s.db.Raw(`
		SELECT t.id AS tag_id, t.name, t.slug, COUNT(DISTINCT sa.artist_id) AS count
		FROM show_artists sa
		JOIN show_venues sv ON sv.show_id = sa.show_id
		JOIN shows s ON s.id = sa.show_id
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = sa.artist_id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE sv.venue_id = ? AND s.status = ?
		GROUP BY t.id, t.name, t.slug
		ORDER BY count DESC
		LIMIT 5
	`, venueID, catalogm.ShowStatusApproved).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get venue genre profile: %w", err)
	}

	result := make([]contracts.GenreCount, len(rows))
	for i, r := range rows {
		result[i] = contracts.GenreCount{
			TagID: r.TagID,
			Name:  r.Name,
			Slug:  r.Slug,
			Count: r.Count,
		}
	}

	return result, nil
}

// GetVenueModel retrieves a raw venue model (used by handlers to check ownership)
func (s *VenueService) GetVenueModel(venueID uint) (*catalogm.Venue, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue catalogm.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrVenueNotFound(venueID)
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	return &venue, nil
}

// contracts.UnverifiedVenueResponse represents an unverified venue for admin review

// GetUnverifiedVenues retrieves all unverified venues for admin review.
// Results are sorted by creation date (newest first).
func (s *VenueService) GetUnverifiedVenues(limit, offset int) ([]*contracts.UnverifiedVenueResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count of unverified venues
	var total int64
	if err := s.db.Model(&catalogm.Venue{}).Where("verified = ?", false).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count unverified venues: %w", err)
	}

	// Build query with show count subquery
	subquery := s.db.Table("show_venues").
		Select("venue_id, COUNT(*) as show_count").
		Group("venue_id")

	var results []struct {
		catalogm.Venue
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
	responses := make([]*contracts.UnverifiedVenueResponse, len(results))
	for i, r := range results {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		responses[i] = &contracts.UnverifiedVenueResponse{
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
