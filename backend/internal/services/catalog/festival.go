package catalog

import (
	"fmt"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/contracts"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/utils"
)

// FestivalService handles festival-related business logic
type FestivalService struct {
	db *gorm.DB
}

// NewFestivalService creates a new festival service
func NewFestivalService(database *gorm.DB) *FestivalService {
	if database == nil {
		database = db.GetDB()
	}
	return &FestivalService{
		db: database,
	}
}

// CreateFestival creates a new festival
func (s *FestivalService) CreateFestival(req *contracts.CreateFestivalRequest) (*contracts.FestivalDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Generate unique slug from name
	baseSlug := utils.GenerateArtistSlug(req.Name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Festival{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	// Determine status, default to "announced"
	status := models.FestivalStatus(req.Status)
	if status == "" {
		status = models.FestivalStatusAnnounced
	}

	festival := &models.Festival{
		Name:         req.Name,
		Slug:         slug,
		SeriesSlug:   req.SeriesSlug,
		EditionYear:  req.EditionYear,
		Description:  req.Description,
		LocationName: req.LocationName,
		City:         req.City,
		State:        req.State,
		Country:      req.Country,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		Website:      req.Website,
		TicketURL:    req.TicketURL,
		FlyerURL:     req.FlyerURL,
		Status:       status,
		Social:       req.Social,
	}

	if err := s.db.Create(festival).Error; err != nil {
		return nil, fmt.Errorf("failed to create festival: %w", err)
	}

	return s.GetFestival(festival.ID)
}

// GetFestival retrieves a festival by ID
func (s *FestivalService) GetFestival(festivalID uint) (*contracts.FestivalDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var festival models.Festival
	err := s.db.First(&festival, festivalID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrFestivalNotFound(festivalID)
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	return s.buildDetailResponse(&festival)
}

// GetFestivalBySlug retrieves a festival by slug
func (s *FestivalService) GetFestivalBySlug(slug string) (*contracts.FestivalDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var festival models.Festival
	err := s.db.Where("slug = ?", slug).First(&festival).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrFestivalNotFound(0)
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	return s.buildDetailResponse(&festival)
}

// ListFestivals retrieves festivals with optional filtering
func (s *FestivalService) ListFestivals(filters map[string]interface{}) ([]*contracts.FestivalListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.Festival{})

	// Apply filters
	if city, ok := filters["city"].(string); ok && city != "" {
		query = query.Where("city = ?", city)
	}
	if state, ok := filters["state"].(string); ok && state != "" {
		query = query.Where("state = ?", state)
	}
	if year, ok := filters["year"].(int); ok && year > 0 {
		query = query.Where("edition_year = ?", year)
	}
	if status, ok := filters["status"].(string); ok && status != "" {
		query = query.Where("status = ?", status)
	}
	if seriesSlug, ok := filters["series_slug"].(string); ok && seriesSlug != "" {
		query = query.Where("series_slug = ?", seriesSlug)
	}
	if tf, ok := filters["tag_filter"].(TagFilter); ok {
		query = ApplyTagFilter(query, s.db, models.TagEntityFestival, "festivals.id", tf)
	}

	// Order by start_date DESC, name ASC
	query = query.Order("start_date DESC, name ASC")

	var festivals []models.Festival
	err := query.Find(&festivals).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list festivals: %w", err)
	}

	// Batch-load artist counts and venue counts
	festivalIDs := make([]uint, len(festivals))
	for i, f := range festivals {
		festivalIDs[i] = f.ID
	}

	artistCounts := make(map[uint]int)
	venueCounts := make(map[uint]int)

	if len(festivalIDs) > 0 {
		type CountResult struct {
			FestivalID uint
			Count      int
		}

		// Artist counts
		var aCounts []CountResult
		s.db.Table("festival_artists").
			Select("festival_id, COUNT(DISTINCT artist_id) as count").
			Where("festival_id IN ?", festivalIDs).
			Group("festival_id").
			Find(&aCounts)
		for _, c := range aCounts {
			artistCounts[c.FestivalID] = c.Count
		}

		// Venue counts
		var vCounts []CountResult
		s.db.Table("festival_venues").
			Select("festival_id, COUNT(DISTINCT venue_id) as count").
			Where("festival_id IN ?", festivalIDs).
			Group("festival_id").
			Find(&vCounts)
		for _, c := range vCounts {
			venueCounts[c.FestivalID] = c.Count
		}
	}

	// Build responses
	responses := make([]*contracts.FestivalListResponse, len(festivals))
	for i, festival := range festivals {
		responses[i] = &contracts.FestivalListResponse{
			ID:          festival.ID,
			Name:        festival.Name,
			Slug:        festival.Slug,
			SeriesSlug:  festival.SeriesSlug,
			EditionYear: festival.EditionYear,
			City:        festival.City,
			State:       festival.State,
			StartDate:   formatDateString(festival.StartDate),
			EndDate:     formatDateString(festival.EndDate),
			Status:      string(festival.Status),
			ArtistCount: artistCounts[festival.ID],
			VenueCount:  venueCounts[festival.ID],
		}
	}

	return responses, nil
}

// SearchFestivals searches for festivals by name using ILIKE matching
func (s *FestivalService) SearchFestivals(query string) ([]*contracts.FestivalListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Return empty results for empty query
	if query == "" {
		return []*contracts.FestivalListResponse{}, nil
	}

	var festivals []models.Festival
	var err error

	if len(query) <= 2 {
		// For short queries: prefix match
		err = s.db.
			Where("LOWER(name) LIKE LOWER(?)", query+"%").
			Order("name ASC").
			Limit(20).
			Find(&festivals).Error
	} else {
		// For longer queries: ILIKE substring match, ordered by name
		err = s.db.
			Where("name ILIKE ?", "%"+query+"%").
			Order("name ASC").
			Limit(20).
			Find(&festivals).Error
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search festivals: %w", err)
	}

	// Batch-load artist counts and venue counts
	festivalIDs := make([]uint, len(festivals))
	for i, f := range festivals {
		festivalIDs[i] = f.ID
	}

	artistCounts := make(map[uint]int)
	venueCounts := make(map[uint]int)

	if len(festivalIDs) > 0 {
		type CountResult struct {
			FestivalID uint
			Count      int
		}

		// Artist counts
		var aCounts []CountResult
		s.db.Table("festival_artists").
			Select("festival_id, COUNT(DISTINCT artist_id) as count").
			Where("festival_id IN ?", festivalIDs).
			Group("festival_id").
			Find(&aCounts)
		for _, c := range aCounts {
			artistCounts[c.FestivalID] = c.Count
		}

		// Venue counts
		var vCounts []CountResult
		s.db.Table("festival_venues").
			Select("festival_id, COUNT(DISTINCT venue_id) as count").
			Where("festival_id IN ?", festivalIDs).
			Group("festival_id").
			Find(&vCounts)
		for _, c := range vCounts {
			venueCounts[c.FestivalID] = c.Count
		}
	}

	// Build responses
	responses := make([]*contracts.FestivalListResponse, len(festivals))
	for i, festival := range festivals {
		responses[i] = &contracts.FestivalListResponse{
			ID:          festival.ID,
			Name:        festival.Name,
			Slug:        festival.Slug,
			SeriesSlug:  festival.SeriesSlug,
			EditionYear: festival.EditionYear,
			City:        festival.City,
			State:       festival.State,
			StartDate:   formatDateString(festival.StartDate),
			EndDate:     formatDateString(festival.EndDate),
			Status:      string(festival.Status),
			ArtistCount: artistCounts[festival.ID],
			VenueCount:  venueCounts[festival.ID],
		}
	}

	return responses, nil
}

// UpdateFestival updates an existing festival
func (s *FestivalService) UpdateFestival(festivalID uint, req *contracts.UpdateFestivalRequest) (*contracts.FestivalDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if festival exists
	var festival models.Festival
	err := s.db.First(&festival, festivalID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrFestivalNotFound(festivalID)
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	updates := map[string]interface{}{}

	if req.Name != nil {
		updates["name"] = *req.Name
		// Regenerate slug when name changes
		baseSlug := utils.GenerateArtistSlug(*req.Name)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&models.Festival{}).Where("slug = ? AND id != ?", candidate, festivalID).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
	}
	if req.SeriesSlug != nil {
		updates["series_slug"] = *req.SeriesSlug
	}
	if req.EditionYear != nil {
		updates["edition_year"] = *req.EditionYear
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.LocationName != nil {
		updates["location_name"] = *req.LocationName
	}
	if req.City != nil {
		updates["city"] = *req.City
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.Country != nil {
		updates["country"] = *req.Country
	}
	if req.StartDate != nil {
		updates["start_date"] = *req.StartDate
	}
	if req.EndDate != nil {
		updates["end_date"] = *req.EndDate
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}
	if req.TicketURL != nil {
		updates["ticket_url"] = *req.TicketURL
	}
	if req.FlyerURL != nil {
		updates["flyer_url"] = *req.FlyerURL
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Social != nil {
		updates["social"] = *req.Social
	}

	if len(updates) > 0 {
		err = s.db.Model(&models.Festival{}).Where("id = ?", festivalID).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update festival: %w", err)
		}
	}

	return s.GetFestival(festivalID)
}

// DeleteFestival deletes a festival
func (s *FestivalService) DeleteFestival(festivalID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if festival exists
	var festival models.Festival
	err := s.db.First(&festival, festivalID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrFestivalNotFound(festivalID)
		}
		return fmt.Errorf("failed to get festival: %w", err)
	}

	// Delete the festival (cascades handle junction cleanup via FK)
	err = s.db.Delete(&festival).Error
	if err != nil {
		return fmt.Errorf("failed to delete festival: %w", err)
	}

	return nil
}

// GetFestivalArtists retrieves the lineup for a festival
func (s *FestivalService) GetFestivalArtists(festivalID uint, dayDate *string) ([]*contracts.FestivalArtistResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify festival exists
	var festival models.Festival
	if err := s.db.First(&festival, festivalID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrFestivalNotFound(festivalID)
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	query := s.db.Where("festival_id = ?", festivalID)
	if dayDate != nil && *dayDate != "" {
		query = query.Where("day_date = ?", *dayDate)
	}

	var festivalArtists []models.FestivalArtist
	// Order: billing tier priority (headliner first via CASE), then position
	err := query.Order(`
		CASE billing_tier
			WHEN 'headliner' THEN 1
			WHEN 'sub_headliner' THEN 2
			WHEN 'mid_card' THEN 3
			WHEN 'undercard' THEN 4
			WHEN 'local' THEN 5
			WHEN 'dj' THEN 6
			WHEN 'host' THEN 7
			ELSE 8
		END ASC, position ASC
	`).Find(&festivalArtists).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get festival artists: %w", err)
	}

	if len(festivalArtists) == 0 {
		return []*contracts.FestivalArtistResponse{}, nil
	}

	// Batch-load artist details
	artistIDs := make([]uint, len(festivalArtists))
	for i, fa := range festivalArtists {
		artistIDs[i] = fa.ArtistID
	}

	artistMap := make(map[uint]*models.Artist)
	var artists []models.Artist
	s.db.Where("id IN ?", artistIDs).Find(&artists)
	for i := range artists {
		artistMap[artists[i].ID] = &artists[i]
	}

	// Build responses
	responses := make([]*contracts.FestivalArtistResponse, 0, len(festivalArtists))
	for _, fa := range festivalArtists {
		artistModel, ok := artistMap[fa.ArtistID]
		if !ok {
			continue
		}
		artistSlug := ""
		if artistModel.Slug != nil {
			artistSlug = *artistModel.Slug
		}

		resp := &contracts.FestivalArtistResponse{
			ID:          fa.ID,
			ArtistID:    fa.ArtistID,
			ArtistSlug:  artistSlug,
			ArtistName:  artistModel.Name,
			BillingTier: string(fa.BillingTier),
			Position:    fa.Position,
			DayDate:     formatOptionalDateString(fa.DayDate),
			Stage:       fa.Stage,
			SetTime:     fa.SetTime,
			VenueID:     fa.VenueID,
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

// AddFestivalArtist adds an artist to a festival lineup
func (s *FestivalService) AddFestivalArtist(festivalID uint, req *contracts.AddFestivalArtistRequest) (*contracts.FestivalArtistResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify festival exists
	var festival models.Festival
	if err := s.db.First(&festival, festivalID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrFestivalNotFound(festivalID)
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	// Verify artist exists
	var artist models.Artist
	if err := s.db.First(&artist, req.ArtistID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("artist not found")
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	billingTier := models.BillingTier(req.BillingTier)
	if billingTier == "" {
		billingTier = models.BillingTierMidCard
	}

	fa := &models.FestivalArtist{
		FestivalID:  festivalID,
		ArtistID:    req.ArtistID,
		BillingTier: billingTier,
		Position:    req.Position,
		DayDate:     req.DayDate,
		Stage:       req.Stage,
		SetTime:     req.SetTime,
		VenueID:     req.VenueID,
	}

	if err := s.db.Create(fa).Error; err != nil {
		return nil, fmt.Errorf("failed to add artist to festival: %w", err)
	}

	artistSlug := ""
	if artist.Slug != nil {
		artistSlug = *artist.Slug
	}

	return &contracts.FestivalArtistResponse{
		ID:          fa.ID,
		ArtistID:    fa.ArtistID,
		ArtistSlug:  artistSlug,
		ArtistName:  artist.Name,
		BillingTier: string(fa.BillingTier),
		Position:    fa.Position,
		DayDate:     formatOptionalDateString(fa.DayDate),
		Stage:       fa.Stage,
		SetTime:     fa.SetTime,
		VenueID:     fa.VenueID,
	}, nil
}

// UpdateFestivalArtist updates an artist's festival details
func (s *FestivalService) UpdateFestivalArtist(festivalID, artistID uint, req *contracts.UpdateFestivalArtistRequest) (*contracts.FestivalArtistResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Find the festival_artist junction entry
	var fa models.FestivalArtist
	err := s.db.Where("festival_id = ? AND artist_id = ?", festivalID, artistID).First(&fa).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("artist not found in festival lineup")
		}
		return nil, fmt.Errorf("failed to get festival artist: %w", err)
	}

	updates := map[string]interface{}{}
	if req.BillingTier != nil {
		updates["billing_tier"] = *req.BillingTier
	}
	if req.Position != nil {
		updates["position"] = *req.Position
	}
	if req.DayDate != nil {
		updates["day_date"] = *req.DayDate
	}
	if req.Stage != nil {
		updates["stage"] = *req.Stage
	}
	if req.SetTime != nil {
		updates["set_time"] = *req.SetTime
	}
	if req.VenueID != nil {
		updates["venue_id"] = *req.VenueID
	}

	if len(updates) > 0 {
		err = s.db.Model(&models.FestivalArtist{}).Where("id = ?", fa.ID).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update festival artist: %w", err)
		}
	}

	// Reload the updated entry
	s.db.First(&fa, fa.ID)

	// Load artist details
	var artist models.Artist
	s.db.First(&artist, fa.ArtistID)
	artistSlug := ""
	if artist.Slug != nil {
		artistSlug = *artist.Slug
	}

	return &contracts.FestivalArtistResponse{
		ID:          fa.ID,
		ArtistID:    fa.ArtistID,
		ArtistSlug:  artistSlug,
		ArtistName:  artist.Name,
		BillingTier: string(fa.BillingTier),
		Position:    fa.Position,
		DayDate:     formatOptionalDateString(fa.DayDate),
		Stage:       fa.Stage,
		SetTime:     fa.SetTime,
		VenueID:     fa.VenueID,
	}, nil
}

// RemoveFestivalArtist removes an artist from a festival lineup
func (s *FestivalService) RemoveFestivalArtist(festivalID, artistID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("festival_id = ? AND artist_id = ?", festivalID, artistID).Delete(&models.FestivalArtist{})
	if result.Error != nil {
		return fmt.Errorf("failed to remove artist from festival: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("artist not found in festival lineup")
	}

	return nil
}

// GetFestivalVenues retrieves the venues for a festival
func (s *FestivalService) GetFestivalVenues(festivalID uint) ([]*contracts.FestivalVenueResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify festival exists
	var festival models.Festival
	if err := s.db.First(&festival, festivalID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrFestivalNotFound(festivalID)
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	var festivalVenues []models.FestivalVenue
	err := s.db.Where("festival_id = ?", festivalID).Order("is_primary DESC, id ASC").Find(&festivalVenues).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get festival venues: %w", err)
	}

	if len(festivalVenues) == 0 {
		return []*contracts.FestivalVenueResponse{}, nil
	}

	// Batch-load venue details
	venueIDs := make([]uint, len(festivalVenues))
	for i, fv := range festivalVenues {
		venueIDs[i] = fv.VenueID
	}

	venueMap := make(map[uint]*models.Venue)
	var venues []models.Venue
	s.db.Where("id IN ?", venueIDs).Find(&venues)
	for i := range venues {
		venueMap[venues[i].ID] = &venues[i]
	}

	// Build responses
	responses := make([]*contracts.FestivalVenueResponse, 0, len(festivalVenues))
	for _, fv := range festivalVenues {
		venueModel, ok := venueMap[fv.VenueID]
		if !ok {
			continue
		}
		venueSlug := ""
		if venueModel.Slug != nil {
			venueSlug = *venueModel.Slug
		}
		responses = append(responses, &contracts.FestivalVenueResponse{
			ID:        fv.ID,
			VenueID:   fv.VenueID,
			VenueName: venueModel.Name,
			VenueSlug: venueSlug,
			City:      venueModel.City,
			State:     venueModel.State,
			IsPrimary: fv.IsPrimary,
		})
	}

	return responses, nil
}

// AddFestivalVenue adds a venue to a festival
func (s *FestivalService) AddFestivalVenue(festivalID uint, req *contracts.AddFestivalVenueRequest) (*contracts.FestivalVenueResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify festival exists
	var festival models.Festival
	if err := s.db.First(&festival, festivalID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrFestivalNotFound(festivalID)
		}
		return nil, fmt.Errorf("failed to get festival: %w", err)
	}

	// Verify venue exists
	var venue models.Venue
	if err := s.db.First(&venue, req.VenueID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("venue not found")
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	fv := &models.FestivalVenue{
		FestivalID: festivalID,
		VenueID:    req.VenueID,
		IsPrimary:  req.IsPrimary,
	}

	if err := s.db.Create(fv).Error; err != nil {
		return nil, fmt.Errorf("failed to add venue to festival: %w", err)
	}

	venueSlug := ""
	if venue.Slug != nil {
		venueSlug = *venue.Slug
	}

	return &contracts.FestivalVenueResponse{
		ID:        fv.ID,
		VenueID:   fv.VenueID,
		VenueName: venue.Name,
		VenueSlug: venueSlug,
		City:      venue.City,
		State:     venue.State,
		IsPrimary: fv.IsPrimary,
	}, nil
}

// RemoveFestivalVenue removes a venue from a festival
func (s *FestivalService) RemoveFestivalVenue(festivalID, venueID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("festival_id = ? AND venue_id = ?", festivalID, venueID).Delete(&models.FestivalVenue{})
	if result.Error != nil {
		return fmt.Errorf("failed to remove venue from festival: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("venue not found in festival")
	}

	return nil
}

// GetFestivalsForArtist retrieves all festivals an artist has played
func (s *FestivalService) GetFestivalsForArtist(artistID uint) ([]*contracts.ArtistFestivalListResponse, error) {
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

	// Get festival_artist entries for this artist
	var festivalArtists []models.FestivalArtist
	if err := s.db.Where("artist_id = ?", artistID).Find(&festivalArtists).Error; err != nil {
		return nil, fmt.Errorf("failed to get artist festivals: %w", err)
	}

	if len(festivalArtists) == 0 {
		return []*contracts.ArtistFestivalListResponse{}, nil
	}

	// Build mapping: festivalID -> festival artist details
	festivalIDs := make([]uint, 0, len(festivalArtists))
	billingMap := make(map[uint]string)
	dayDateMap := make(map[uint]*string)
	stageMap := make(map[uint]*string)
	for _, fa := range festivalArtists {
		festivalIDs = append(festivalIDs, fa.FestivalID)
		billingMap[fa.FestivalID] = string(fa.BillingTier)
		dayDateMap[fa.FestivalID] = fa.DayDate
		stageMap[fa.FestivalID] = fa.Stage
	}

	// Fetch festivals
	var festivals []models.Festival
	if err := s.db.Where("id IN ?", festivalIDs).Order("start_date DESC, name ASC").Find(&festivals).Error; err != nil {
		return nil, fmt.Errorf("failed to get festivals: %w", err)
	}

	// Batch-load artist and venue counts
	artistCounts := make(map[uint]int)
	venueCounts := make(map[uint]int)

	if len(festivalIDs) > 0 {
		type CountResult struct {
			FestivalID uint
			Count      int
		}

		var aCounts []CountResult
		s.db.Table("festival_artists").
			Select("festival_id, COUNT(DISTINCT artist_id) as count").
			Where("festival_id IN ?", festivalIDs).
			Group("festival_id").
			Find(&aCounts)
		for _, c := range aCounts {
			artistCounts[c.FestivalID] = c.Count
		}

		var vCounts []CountResult
		s.db.Table("festival_venues").
			Select("festival_id, COUNT(DISTINCT venue_id) as count").
			Where("festival_id IN ?", festivalIDs).
			Group("festival_id").
			Find(&vCounts)
		for _, c := range vCounts {
			venueCounts[c.FestivalID] = c.Count
		}
	}

	// Build responses
	responses := make([]*contracts.ArtistFestivalListResponse, len(festivals))
	for i, festival := range festivals {
		responses[i] = &contracts.ArtistFestivalListResponse{
			FestivalListResponse: contracts.FestivalListResponse{
				ID:          festival.ID,
				Name:        festival.Name,
				Slug:        festival.Slug,
				SeriesSlug:  festival.SeriesSlug,
				EditionYear: festival.EditionYear,
				City:        festival.City,
				State:       festival.State,
				StartDate:   formatDateString(festival.StartDate),
				EndDate:     formatDateString(festival.EndDate),
				Status:      string(festival.Status),
				ArtistCount: artistCounts[festival.ID],
				VenueCount:  venueCounts[festival.ID],
			},
			BillingTier: billingMap[festival.ID],
			DayDate:     formatOptionalDateString(dayDateMap[festival.ID]),
			Stage:       stageMap[festival.ID],
		}
	}

	return responses, nil
}

// buildDetailResponse converts a Festival model to contracts.FestivalDetailResponse
func (s *FestivalService) buildDetailResponse(festival *models.Festival) (*contracts.FestivalDetailResponse, error) {
	// Count artists
	var artistCount int64
	s.db.Table("festival_artists").Where("festival_id = ?", festival.ID).Count(&artistCount)

	// Count venues
	var venueCount int64
	s.db.Table("festival_venues").Where("festival_id = ?", festival.ID).Count(&venueCount)

	return &contracts.FestivalDetailResponse{
		ID:           festival.ID,
		Name:         festival.Name,
		Slug:         festival.Slug,
		SeriesSlug:   festival.SeriesSlug,
		EditionYear:  festival.EditionYear,
		Description:  festival.Description,
		LocationName: festival.LocationName,
		City:         festival.City,
		State:        festival.State,
		Country:      festival.Country,
		StartDate:    formatDateString(festival.StartDate),
		EndDate:      formatDateString(festival.EndDate),
		Website:      festival.Website,
		TicketURL:    festival.TicketURL,
		FlyerURL:     festival.FlyerURL,
		Status:       string(festival.Status),
		Social:       festival.Social,
		ArtistCount:  int(artistCount),
		VenueCount:   int(venueCount),
		CreatedAt:    festival.CreatedAt,
		UpdatedAt:    festival.UpdatedAt,
	}, nil
}

// formatDateString normalizes a date string that may include a timestamp suffix
// (e.g., "2026-03-06T00:00:00Z") back to just the date part ("2026-03-06").
func formatDateString(date string) string {
	if len(date) >= 10 {
		return date[:10]
	}
	return date
}

// formatOptionalDateString normalizes an optional date string pointer.
func formatOptionalDateString(date *string) *string {
	if date == nil {
		return nil
	}
	formatted := formatDateString(*date)
	return &formatted
}
