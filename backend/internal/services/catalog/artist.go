package catalog

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// ArtistService handles artist-related business logic
type ArtistService struct {
	db *gorm.DB
}

// NewArtistService creates a new artist service
func NewArtistService(database *gorm.DB) *ArtistService {
	if database == nil {
		database = db.GetDB()
	}
	return &ArtistService{
		db: database,
	}
}


// CreateArtist creates a new artist
func (s *ArtistService) CreateArtist(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if artist already exists
	var existingArtist models.Artist
	err := s.db.Where("LOWER(name) = LOWER(?)", req.Name).First(&existingArtist).Error
	if err == nil {
		return nil, fmt.Errorf("artist with name '%s' already exists", req.Name)
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check existing artist: %w", err)
	}

	// Generate unique slug
	baseSlug := utils.GenerateArtistSlug(req.Name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Artist{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	// Create the artist
	artist := &models.Artist{
		Name:  req.Name,
		Slug:  &slug,
		State: req.State,
		City:  req.City,
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

	if err := s.db.Create(artist).Error; err != nil {
		return nil, fmt.Errorf("failed to create artist: %w", err)
	}

	return s.buildArtistResponse(artist), nil
}

// GetArtist retrieves an artist by ID
func (s *ArtistService) GetArtist(artistID uint) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var artist models.Artist
	err := s.db.First(&artist, artistID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	return s.buildArtistResponse(&artist), nil
}

// GetArtistByName retrieves an artist by name (case-insensitive)
func (s *ArtistService) GetArtistByName(name string) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var artist models.Artist
	err := s.db.Where("LOWER(name) = LOWER(?)", name).First(&artist).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrArtistNotFound(0)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	return s.buildArtistResponse(&artist), nil
}

// GetArtistBySlug retrieves an artist by slug
func (s *ArtistService) GetArtistBySlug(slug string) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var artist models.Artist
	err := s.db.Where("slug = ?", slug).First(&artist).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrArtistNotFound(0)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	return s.buildArtistResponse(&artist), nil
}

// GetArtists retrieves artists with optional filtering
func (s *ArtistService) GetArtists(filters map[string]interface{}) ([]*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db

	// Apply filters
	if cities, ok := filters["cities"].([]map[string]string); ok && len(cities) > 0 {
		// Multi-city filter: (city = ? AND state = ?) OR ...
		var conditions []string
		var args []interface{}
		for _, cs := range cities {
			if cs["city"] != "" && cs["state"] != "" {
				conditions = append(conditions, "(city = ? AND state = ?)")
				args = append(args, cs["city"], cs["state"])
			}
		}
		if len(conditions) > 0 {
			query = query.Where(strings.Join(conditions, " OR "), args...)
		}
	} else {
		if state, ok := filters["state"].(string); ok && state != "" {
			query = query.Where("state = ?", state)
		}
		if city, ok := filters["city"].(string); ok && city != "" {
			query = query.Where("city = ?", city)
		}
	}
	if name, ok := filters["name"].(string); ok && name != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+name+"%")
	}

	// Default ordering by name
	query = query.Order("name ASC")

	var artists []models.Artist
	err := query.Find(&artists).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get artists: %w", err)
	}

	// Build responses
	responses := make([]*contracts.ArtistDetailResponse, len(artists))
	for i, artist := range artists {
		responses[i] = s.buildArtistResponse(&artist)
	}

	return responses, nil
}

// UpdateArtist updates an existing artist
func (s *ArtistService) UpdateArtist(artistID uint, updates map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if name is being updated and if it conflicts with existing artist
	if name, ok := updates["name"].(string); ok {
		var existingArtist models.Artist
		err := s.db.Where("LOWER(name) = LOWER(?) AND id != ?", name, artistID).First(&existingArtist).Error
		if err == nil {
			return nil, fmt.Errorf("artist with name '%s' already exists", name)
		} else if err != gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("failed to check existing artist: %w", err)
		}

		// Regenerate slug when name changes
		baseSlug := utils.GenerateArtistSlug(name)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&models.Artist{}).Where("slug = ? AND id != ?", candidate, artistID).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
	}

	err := s.db.Model(&models.Artist{}).Where("id = ?", artistID).Updates(updates).Error
	if err != nil {
		return nil, fmt.Errorf("failed to update artist: %w", err)
	}

	return s.GetArtist(artistID)
}

// DeleteArtist deletes an artist
func (s *ArtistService) DeleteArtist(artistID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if artist exists
	var artist models.Artist
	err := s.db.First(&artist, artistID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrArtistNotFound(artistID)
		}
		return fmt.Errorf("failed to get artist: %w", err)
	}

	// Check if artist is associated with any shows
	var count int64
	err = s.db.Model(&models.ShowArtist{}).Where("artist_id = ?", artistID).Count(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check artist associations: %w", err)
	}

	if count > 0 {
		return apperrors.ErrArtistHasShows(artistID, count)
	}

	// Delete the artist
	err = s.db.Delete(&artist).Error
	if err != nil {
		return fmt.Errorf("failed to delete artist: %w", err)
	}

	return nil
}

// SearchArtists performs autocomplete search on artist names and aliases.
// Uses pg_trgm extension for performant fuzzy matching with intelligent query strategy:
// - Short queries (1-2 chars): Fast case-insensitive prefix search
// - Longer queries (3+ chars): Similarity-based fuzzy matching with ranking
// Alias matches return the canonical artist (deduplicated).
func (s *ArtistService) SearchArtists(query string) ([]*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Return empty results for empty query
	if query == "" {
		return []*contracts.ArtistDetailResponse{}, nil
	}

	var artists []models.Artist
	var err error

	// Strategy depends on query length for optimal performance
	if len(query) <= 2 {
		// For short queries: use fast case-insensitive prefix search on name + aliases
		err = s.db.
			Where("id IN (?)",
				s.db.Table("artists").Select("id").Where("LOWER(name) LIKE LOWER(?)", query+"%"),
			).
			Or("id IN (?)",
				s.db.Table("artist_aliases").Select("artist_id").Where("LOWER(alias) LIKE LOWER(?)", query+"%"),
			).
			Order("name ASC").
			Limit(10).
			Find(&artists).Error
	} else {
		// For longer queries: search both names and aliases with similarity scoring
		err = s.db.
			Select("DISTINCT ON (artists.id) artists.*, GREATEST(similarity(artists.name, ?), COALESCE((SELECT MAX(similarity(aa.alias, ?)) FROM artist_aliases aa WHERE aa.artist_id = artists.id), 0)) as sim_score", query, query).
			Where("artists.name ILIKE ? OR artists.name % ? OR artists.id IN (?)",
				"%"+query+"%", query,
				s.db.Table("artist_aliases").Select("artist_id").Where("alias ILIKE ? OR alias % ?", "%"+query+"%", query),
			).
			Order("artists.id, sim_score DESC").
			Limit(10).
			Find(&artists).Error
		// Re-sort by sim_score since DISTINCT ON requires ordering by the DISTINCT column first
		if err == nil && len(artists) > 1 {
			// The DISTINCT ON approach may not sort correctly, so we use a subquery approach instead
			artists = nil
			err = s.db.Raw(`
				SELECT a.* FROM artists a
				WHERE a.id IN (
					SELECT id FROM artists WHERE name ILIKE ? OR name % ?
					UNION
					SELECT artist_id FROM artist_aliases WHERE alias ILIKE ? OR alias % ?
				)
				ORDER BY GREATEST(
					similarity(a.name, ?),
					COALESCE((SELECT MAX(similarity(aa.alias, ?)) FROM artist_aliases aa WHERE aa.artist_id = a.id), 0)
				) DESC, a.name ASC
				LIMIT 10
			`, "%"+query+"%", query, "%"+query+"%", query, query, query).
				Scan(&artists).Error
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search artists: %w", err)
	}

	// Build response
	responses := make([]*contracts.ArtistDetailResponse, len(artists))
	for i, artist := range artists {
		responses[i] = s.buildArtistResponse(&artist)
	}

	return responses, nil
}

// ArtistWithCount is used internally for querying artists with their show counts
type ArtistWithCount struct {
	models.Artist
	UpcomingShowCount int64 `gorm:"column:upcoming_show_count"`
}

// contracts.ArtistWithShowCountResponse represents an artist with its upcoming show count

// GetArtistsWithShowCounts retrieves artists that have upcoming approved shows,
// with their show counts. Results are sorted by show count (descending), then name (ascending).
func (s *ArtistService) GetArtistsWithShowCounts(filters map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	// Subquery: count upcoming approved shows per artist
	subquery := s.db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(*) as show_count").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("shows.event_date >= ? AND shows.status = ?", now, models.ShowStatusApproved).
		Group("show_artists.artist_id")

	// Main query: join artists with show counts, only include artists with upcoming shows
	query := s.db.Table("artists").
		Select("artists.*, COALESCE(sc.show_count, 0) as upcoming_show_count").
		Joins("JOIN (?) as sc ON artists.id = sc.artist_id", subquery)

	// Apply filters
	if cities, ok := filters["cities"].([]map[string]string); ok && len(cities) > 0 {
		var conditions []string
		var args []interface{}
		for _, cs := range cities {
			if cs["city"] != "" && cs["state"] != "" {
				conditions = append(conditions, "(artists.city = ? AND artists.state = ?)")
				args = append(args, cs["city"], cs["state"])
			}
		}
		if len(conditions) > 0 {
			query = query.Where(strings.Join(conditions, " OR "), args...)
		}
	} else {
		if state, ok := filters["state"].(string); ok && state != "" {
			query = query.Where("artists.state = ?", state)
		}
		if city, ok := filters["city"].(string); ok && city != "" {
			query = query.Where("artists.city = ?", city)
		}
	}

	var artistsWithCount []ArtistWithCount
	if err := query.Order("upcoming_show_count DESC, artists.name ASC").Find(&artistsWithCount).Error; err != nil {
		return nil, fmt.Errorf("failed to get artists with show counts: %w", err)
	}

	// Build responses
	responses := make([]*contracts.ArtistWithShowCountResponse, len(artistsWithCount))
	for i, ac := range artistsWithCount {
		responses[i] = &contracts.ArtistWithShowCountResponse{
			ArtistDetailResponse: *s.buildArtistResponse(&ac.Artist),
			UpcomingShowCount:    int(ac.UpcomingShowCount),
		}
	}

	return responses, nil
}


// GetArtistCities returns distinct cities for artists that have upcoming approved shows.
// Only artists with both city and state set are included.
// Results are sorted by artist count (descending) to show most active cities first.
func (s *ArtistService) GetArtistCities() ([]*contracts.ArtistCityResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type CityResult struct {
		City        string
		State       string
		ArtistCount int64
	}

	// Subquery: artist IDs that have upcoming approved shows
	artistsWithShows := s.db.Table("show_artists").
		Select("DISTINCT show_artists.artist_id").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("shows.event_date >= ? AND shows.status = ?", now, models.ShowStatusApproved)

	var results []CityResult
	err := s.db.Table("artists").
		Select("city, state, COUNT(*) as artist_count").
		Where("city IS NOT NULL AND city != '' AND state IS NOT NULL AND state != ''").
		Where("id IN (?)", artistsWithShows).
		Group("city, state").
		Order("artist_count DESC, city ASC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get artist cities: %w", err)
	}

	responses := make([]*contracts.ArtistCityResponse, len(results))
	for i, r := range results {
		responses[i] = &contracts.ArtistCityResponse{
			City:        r.City,
			State:       r.State,
			ArtistCount: int(r.ArtistCount),
		}
	}

	return responses, nil
}

// buildArtistResponse converts an Artist model to contracts.ArtistDetailResponse
func (s *ArtistService) buildArtistResponse(artist *models.Artist) *contracts.ArtistDetailResponse {
	slug := ""
	if artist.Slug != nil {
		slug = *artist.Slug
	}
	return &contracts.ArtistDetailResponse{
		ID:               artist.ID,
		Slug:             slug,
		Name:             artist.Name,
		State:            artist.State,
		City:             artist.City,
		BandcampEmbedURL: artist.BandcampEmbedURL,
		Social: contracts.SocialResponse{
			Instagram:  artist.Social.Instagram,
			Facebook:   artist.Social.Facebook,
			Twitter:    artist.Social.Twitter,
			YouTube:    artist.Social.YouTube,
			Spotify:    artist.Social.Spotify,
			SoundCloud: artist.Social.SoundCloud,
			Bandcamp:   artist.Social.Bandcamp,
			Website:    artist.Social.Website,
		},
		CreatedAt: artist.CreatedAt,
		UpdatedAt: artist.UpdatedAt,
	}
}

// contracts.ArtistLabelResponse represents a label the artist is on

// GetLabelsForArtist retrieves all labels associated with an artist
func (s *ArtistService) GetLabelsForArtist(artistID uint) ([]*contracts.ArtistLabelResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify artist exists
	var artist models.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	// Get label IDs from junction table
	var artistLabels []models.ArtistLabel
	s.db.Where("artist_id = ?", artistID).Find(&artistLabels)

	if len(artistLabels) == 0 {
		return []*contracts.ArtistLabelResponse{}, nil
	}

	labelIDs := make([]uint, len(artistLabels))
	for i, al := range artistLabels {
		labelIDs[i] = al.LabelID
	}

	var labels []models.Label
	s.db.Where("id IN ?", labelIDs).Order("name ASC").Find(&labels)

	responses := make([]*contracts.ArtistLabelResponse, len(labels))
	for i, label := range labels {
		slug := ""
		if label.Slug != nil {
			slug = *label.Slug
		}
		responses[i] = &contracts.ArtistLabelResponse{
			ID:    label.ID,
			Name:  label.Name,
			Slug:  slug,
			City:  label.City,
			State: label.State,
		}
	}

	return responses, nil
}


// GetShowsForArtist retrieves shows for a specific artist with time filtering.
// timeFilter can be: "upcoming" (event_date >= today), "past" (event_date < today), or "all"
// Only returns approved shows.
func (s *ArtistService) GetShowsForArtist(artistID uint, timezone string, limit int, timeFilter string) ([]*contracts.ArtistShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Verify artist exists
	var artist models.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, 0, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, 0, fmt.Errorf("failed to get artist: %w", err)
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
	baseQuery := s.db.Table("show_artists").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("show_artists.artist_id = ? AND shows.status = ?", artistID, models.ShowStatusApproved)

	// Apply time filter and determine ordering
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
	countQuery := s.db.Table("show_artists").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("show_artists.artist_id = ? AND shows.status = ?", artistID, models.ShowStatusApproved)
	if dateCondition != "" {
		countQuery = countQuery.Where(dateCondition, startOfTodayUTC)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count shows: %w", err)
	}

	// Get show IDs with limit
	var showIDs []uint
	showQuery := s.db.Table("show_artists").
		Select("show_artists.show_id").
		Joins("JOIN shows ON show_artists.show_id = shows.id").
		Where("show_artists.artist_id = ? AND shows.status = ?", artistID, models.ShowStatusApproved)
	if dateCondition != "" {
		showQuery = showQuery.Where(dateCondition, startOfTodayUTC)
	}
	if err := showQuery.Order(orderDirection).Limit(limit).Pluck("show_artists.show_id", &showIDs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get show IDs: %w", err)
	}

	// Fetch full show data with artists
	var shows []models.Show
	if len(showIDs) > 0 {
		if err := s.db.Preload("Artists").Where("id IN ?", showIDs).Order(orderDirection).Find(&shows).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to get shows: %w", err)
		}
	}

	// Batch-load all ShowVenue records
	showIDsList := make([]uint, len(shows))
	for i, show := range shows {
		showIDsList[i] = show.ID
	}

	var allShowVenues []models.ShowVenue
	s.db.Where("show_id IN ?", showIDsList).Find(&allShowVenues)

	// Batch-fetch all venue models
	venueIDs := make([]uint, 0, len(allShowVenues))
	showVenueMap := make(map[uint]uint) // showID -> venueID
	for _, sv := range allShowVenues {
		showVenueMap[sv.ShowID] = sv.VenueID
		venueIDs = append(venueIDs, sv.VenueID)
	}
	venueMap := make(map[uint]*models.Venue)
	if len(venueIDs) > 0 {
		var venues []models.Venue
		s.db.Where("id IN ?", venueIDs).Find(&venues)
		for i := range venues {
			venueMap[venues[i].ID] = &venues[i]
		}
	}

	// Batch-load all ShowArtist records
	var allShowArtists []models.ShowArtist
	s.db.Where("show_id IN ?", showIDsList).Order("position ASC").Find(&allShowArtists)
	showArtistsMap := make(map[uint][]models.ShowArtist)
	var allArtistIDs []uint
	for _, sa := range allShowArtists {
		showArtistsMap[sa.ShowID] = append(showArtistsMap[sa.ShowID], sa)
		allArtistIDs = append(allArtistIDs, sa.ArtistID)
	}
	artistMap := make(map[uint]*models.Artist)
	if len(allArtistIDs) > 0 {
		var artists []models.Artist
		s.db.Where("id IN ?", allArtistIDs).Find(&artists)
		for i := range artists {
			artistMap[artists[i].ID] = &artists[i]
		}
	}

	// Build responses
	responses := make([]*contracts.ArtistShowResponse, len(shows))
	for i, show := range shows {
		// Look up venue for this show
		var venue *contracts.ArtistShowVenueResponse
		if venueID, ok := showVenueMap[show.ID]; ok {
			if venueModel, ok := venueMap[venueID]; ok {
				var venueSlug string
				if venueModel.Slug != nil {
					venueSlug = *venueModel.Slug
				}
				venue = &contracts.ArtistShowVenueResponse{
					ID:    venueModel.ID,
					Slug:  venueSlug,
					Name:  venueModel.Name,
					City:  venueModel.City,
					State: venueModel.State,
				}
			}
		}

		// Look up ordered artists for this show
		artists := make([]contracts.ArtistShowArtist, 0)
		for _, sa := range showArtistsMap[show.ID] {
			if artistModel, ok := artistMap[sa.ArtistID]; ok {
				var artistSlug string
				if artistModel.Slug != nil {
					artistSlug = *artistModel.Slug
				}
				artists = append(artists, contracts.ArtistShowArtist{
					ID:   artistModel.ID,
					Slug: artistSlug,
					Name: artistModel.Name,
				})
			}
		}

		responses[i] = &contracts.ArtistShowResponse{
			ID:             show.ID,
			Title:          show.Title,
			EventDate:      show.EventDate,
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			Venue:          venue,
			Artists:        artists,
		}
	}

	return responses, total, nil
}

// ──────────────────────────────────────────────
// Alias CRUD
// ──────────────────────────────────────────────

// AddArtistAlias adds an alias for an artist. Validates uniqueness against
// other aliases and artist names (case-insensitive).
func (s *ArtistService) AddArtistAlias(artistID uint, alias string) (*contracts.ArtistAliasResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil, fmt.Errorf("alias cannot be empty")
	}

	// Verify artist exists
	var artist models.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	// Check for duplicate alias (case-insensitive)
	var existing models.ArtistAlias
	if err := s.db.Where("LOWER(alias) = LOWER(?)", alias).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("alias '%s' already exists", alias)
	}

	// Check if alias matches an existing artist name
	var existingArtist models.Artist
	if err := s.db.Where("LOWER(name) = LOWER(?)", alias).First(&existingArtist).Error; err == nil {
		return nil, fmt.Errorf("alias '%s' conflicts with existing artist name", alias)
	}

	artistAlias := &models.ArtistAlias{
		ArtistID: artistID,
		Alias:    alias,
	}

	if err := s.db.Create(artistAlias).Error; err != nil {
		return nil, fmt.Errorf("failed to create alias: %w", err)
	}

	return &contracts.ArtistAliasResponse{
		ID:        artistAlias.ID,
		ArtistID:  artistAlias.ArtistID,
		Alias:     artistAlias.Alias,
		CreatedAt: artistAlias.CreatedAt.Format(time.RFC3339),
	}, nil
}

// RemoveArtistAlias deletes an alias by ID.
func (s *ArtistService) RemoveArtistAlias(aliasID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Delete(&models.ArtistAlias{}, aliasID)
	if result.Error != nil {
		return fmt.Errorf("failed to delete alias: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("alias not found")
	}

	return nil
}

// GetArtistAliases returns all aliases for an artist.
func (s *ArtistService) GetArtistAliases(artistID uint) ([]*contracts.ArtistAliasResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify artist exists
	var artist models.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrArtistNotFound(artistID)
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	var aliases []models.ArtistAlias
	if err := s.db.Where("artist_id = ?", artistID).Order("alias ASC").Find(&aliases).Error; err != nil {
		return nil, fmt.Errorf("failed to list aliases: %w", err)
	}

	responses := make([]*contracts.ArtistAliasResponse, len(aliases))
	for i, a := range aliases {
		responses[i] = &contracts.ArtistAliasResponse{
			ID:        a.ID,
			ArtistID:  a.ArtistID,
			Alias:     a.Alias,
			CreatedAt: a.CreatedAt.Format(time.RFC3339),
		}
	}

	return responses, nil
}

// ──────────────────────────────────────────────
// Artist Merge
// ──────────────────────────────────────────────

// MergeArtists merges the "mergeFrom" artist into the "canonical" artist.
// All relationships (shows, releases, labels, festivals, etc.) are transferred
// to the canonical artist. Conflicts (duplicate rows) are deleted before transfer.
// The merged artist's name is added as an alias, then the merged artist is deleted.
func (s *ArtistService) MergeArtists(canonicalID, mergeFromID uint) (*contracts.MergeArtistResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if canonicalID == mergeFromID {
		return nil, fmt.Errorf("cannot merge an artist with itself")
	}

	// Verify both artists exist
	var canonical models.Artist
	if err := s.db.First(&canonical, canonicalID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrArtistNotFound(canonicalID)
		}
		return nil, fmt.Errorf("failed to get canonical artist: %w", err)
	}

	var mergeFrom models.Artist
	if err := s.db.First(&mergeFrom, mergeFromID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrArtistNotFound(mergeFromID)
		}
		return nil, fmt.Errorf("failed to get merge-from artist: %w", err)
	}

	result := &contracts.MergeArtistResult{
		CanonicalArtistID: canonicalID,
		MergedArtistID:    mergeFromID,
		MergedArtistName:  mergeFrom.Name,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. show_artists: delete conflicts, then update remaining
		tx.Where("artist_id = ? AND show_id IN (?)", mergeFromID,
			tx.Table("show_artists").Select("show_id").Where("artist_id = ?", canonicalID),
		).Delete(&models.ShowArtist{})
		r := tx.Model(&models.ShowArtist{}).Where("artist_id = ?", mergeFromID).Update("artist_id", canonicalID)
		result.ShowsMoved = r.RowsAffected

		// 2. artist_releases: delete conflicts, then update remaining
		tx.Exec("DELETE FROM artist_releases WHERE artist_id = ? AND (release_id, role) IN (SELECT release_id, role FROM artist_releases WHERE artist_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE artist_releases SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)
		result.ReleasesMoved = r.RowsAffected

		// 3. artist_labels: delete conflicts, then update remaining
		tx.Exec("DELETE FROM artist_labels WHERE artist_id = ? AND label_id IN (SELECT label_id FROM artist_labels WHERE artist_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE artist_labels SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)
		result.LabelsMoved = r.RowsAffected

		// 4. festival_artists: delete conflicts, then update remaining
		tx.Exec("DELETE FROM festival_artists WHERE artist_id = ? AND festival_id IN (SELECT festival_id FROM festival_artists WHERE artist_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE festival_artists SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)
		result.FestivalsMoved = r.RowsAffected

		// 5. artist_relationships: re-canonicalize with source < target, delete self-referential and conflicts
		// First delete any that would become self-referential
		tx.Exec("DELETE FROM artist_relationship_votes WHERE (source_artist_id = ? OR target_artist_id = ?) AND (source_artist_id = ? OR target_artist_id = ?)",
			mergeFromID, mergeFromID, canonicalID, canonicalID)
		tx.Exec("DELETE FROM artist_relationships WHERE (source_artist_id = ? AND target_artist_id = ?) OR (source_artist_id = ? AND target_artist_id = ?)",
			mergeFromID, canonicalID, canonicalID, mergeFromID)
		// Delete votes for relationships that will be deleted as self-referential after merge
		tx.Exec("DELETE FROM artist_relationship_votes WHERE source_artist_id = ? OR target_artist_id = ?", mergeFromID, mergeFromID)
		// Delete remaining relationships involving mergeFrom (after conflicts removed)
		r = tx.Exec("DELETE FROM artist_relationships WHERE source_artist_id = ? OR target_artist_id = ?", mergeFromID, mergeFromID)
		result.RelationshipsMoved = r.RowsAffected

		// 6. entity_tags: delete conflicts, then update remaining
		tx.Exec("DELETE FROM entity_tags WHERE entity_type = 'artist' AND entity_id = ? AND tag_id IN (SELECT tag_id FROM entity_tags WHERE entity_type = 'artist' AND entity_id = ?)", mergeFromID, canonicalID)
		tx.Exec("UPDATE entity_tags SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)

		// 7. user_bookmarks: delete conflicts, then update remaining
		tx.Exec("DELETE FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ? AND (user_id, action) IN (SELECT user_id, action FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ?)", mergeFromID, canonicalID)
		r = tx.Exec("UPDATE user_bookmarks SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)
		result.BookmarksMoved = r.RowsAffected

		// 8. artist_reports: delete conflicts, then update remaining
		tx.Exec("DELETE FROM artist_reports WHERE artist_id = ? AND reported_by IN (SELECT reported_by FROM artist_reports WHERE artist_id = ?)", mergeFromID, canonicalID)
		tx.Exec("UPDATE artist_reports SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)

		// 9. revisions: just update entity_id (no conflict key)
		tx.Exec("UPDATE revisions SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)

		// 10. tag_votes for entity tags: delete conflicts, then update remaining
		tx.Exec(`DELETE FROM tag_votes WHERE entity_type = 'artist' AND entity_id = ?
			AND (tag_id, user_id) IN (SELECT tag_id, user_id FROM tag_votes WHERE entity_type = 'artist' AND entity_id = ?)`, mergeFromID, canonicalID)
		tx.Exec("UPDATE tag_votes SET entity_id = ? WHERE entity_type = 'artist' AND entity_id = ?", canonicalID, mergeFromID)

		// 11. Transfer aliases from merged artist to canonical
		tx.Exec("UPDATE artist_aliases SET artist_id = ? WHERE artist_id = ?", canonicalID, mergeFromID)

		// 12. Create alias from merged artist's name (if not conflicting)
		var aliasCount int64
		tx.Model(&models.ArtistAlias{}).Where("LOWER(alias) = LOWER(?)", mergeFrom.Name).Count(&aliasCount)
		var nameCount int64
		tx.Model(&models.Artist{}).Where("LOWER(name) = LOWER(?)", mergeFrom.Name).Where("id != ?", mergeFromID).Count(&nameCount)
		if aliasCount == 0 && nameCount == 0 {
			newAlias := models.ArtistAlias{
				ArtistID: canonicalID,
				Alias:    mergeFrom.Name,
			}
			if err := tx.Create(&newAlias).Error; err != nil {
				return fmt.Errorf("failed to create alias from merged artist name: %w", err)
			}
			result.AliasCreated = true
		}

		// 13. Delete the merged artist
		if err := tx.Delete(&mergeFrom).Error; err != nil {
			return fmt.Errorf("failed to delete merged artist: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("merge failed: %w", err)
	}

	return result, nil
}
