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

// LabelService handles label-related business logic
type LabelService struct {
	db *gorm.DB
}

// NewLabelService creates a new label service
func NewLabelService(database *gorm.DB) *LabelService {
	if database == nil {
		database = db.GetDB()
	}
	return &LabelService{
		db: database,
	}
}

// CreateLabel creates a new label
func (s *LabelService) CreateLabel(req *contracts.CreateLabelRequest) (*contracts.LabelDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Generate unique slug
	baseSlug := utils.GenerateArtistSlug(req.Name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Label{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	// Determine status, default to "active"
	status := models.LabelStatus(req.Status)
	if status == "" {
		status = models.LabelStatusActive
	}

	// Create the label
	label := &models.Label{
		Name:        req.Name,
		Slug:        &slug,
		City:        req.City,
		State:       req.State,
		Country:     req.Country,
		FoundedYear: req.FoundedYear,
		Status:      status,
		Description: req.Description,
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

	if err := s.db.Create(label).Error; err != nil {
		return nil, fmt.Errorf("failed to create label: %w", err)
	}

	return s.GetLabel(label.ID)
}

// GetLabel retrieves a label by ID
func (s *LabelService) GetLabel(labelID uint) (*contracts.LabelDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var label models.Label
	err := s.db.First(&label, labelID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrLabelNotFound(labelID)
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}

	return s.buildDetailResponse(&label)
}

// GetLabelBySlug retrieves a label by slug
func (s *LabelService) GetLabelBySlug(slug string) (*contracts.LabelDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var label models.Label
	err := s.db.Where("slug = ?", slug).First(&label).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrLabelNotFound(0)
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}

	return s.buildDetailResponse(&label)
}

// ListLabels retrieves labels with optional filtering
func (s *LabelService) ListLabels(filters map[string]interface{}) ([]*contracts.LabelListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.Label{})

	// Apply filters
	if status, ok := filters["status"].(string); ok && status != "" {
		query = query.Where("status = ?", status)
	}
	if city, ok := filters["city"].(string); ok && city != "" {
		query = query.Where("city = ?", city)
	}
	if state, ok := filters["state"].(string); ok && state != "" {
		query = query.Where("state = ?", state)
	}

	// Order by name ASC
	query = query.Order("name ASC")

	var labels []models.Label
	err := query.Find(&labels).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	// Batch-load artist counts and release counts
	labelIDs := make([]uint, len(labels))
	for i, l := range labels {
		labelIDs[i] = l.ID
	}

	artistCounts := make(map[uint]int)
	releaseCounts := make(map[uint]int)

	if len(labelIDs) > 0 {
		type CountResult struct {
			LabelID uint
			Count   int
		}

		// Artist counts
		var aCounts []CountResult
		s.db.Table("artist_labels").
			Select("label_id, COUNT(DISTINCT artist_id) as count").
			Where("label_id IN ?", labelIDs).
			Group("label_id").
			Find(&aCounts)
		for _, c := range aCounts {
			artistCounts[c.LabelID] = c.Count
		}

		// Release counts
		var rCounts []CountResult
		s.db.Table("release_labels").
			Select("label_id, COUNT(DISTINCT release_id) as count").
			Where("label_id IN ?", labelIDs).
			Group("label_id").
			Find(&rCounts)
		for _, c := range rCounts {
			releaseCounts[c.LabelID] = c.Count
		}
	}

	// Build responses
	responses := make([]*contracts.LabelListResponse, len(labels))
	for i, label := range labels {
		slug := ""
		if label.Slug != nil {
			slug = *label.Slug
		}
		responses[i] = &contracts.LabelListResponse{
			ID:           label.ID,
			Name:         label.Name,
			Slug:         slug,
			City:         label.City,
			State:        label.State,
			Status:       string(label.Status),
			ArtistCount:  artistCounts[label.ID],
			ReleaseCount: releaseCounts[label.ID],
		}
	}

	return responses, nil
}

// SearchLabels searches for labels by name using ILIKE matching
func (s *LabelService) SearchLabels(query string) ([]*contracts.LabelListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Return empty results for empty query
	if query == "" {
		return []*contracts.LabelListResponse{}, nil
	}

	var labels []models.Label
	var err error

	if len(query) <= 2 {
		// For short queries: prefix match
		err = s.db.
			Where("LOWER(name) LIKE LOWER(?)", query+"%").
			Order("name ASC").
			Limit(20).
			Find(&labels).Error
	} else {
		// For longer queries: ILIKE substring match, ordered by name
		err = s.db.
			Where("name ILIKE ?", "%"+query+"%").
			Order("name ASC").
			Limit(20).
			Find(&labels).Error
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search labels: %w", err)
	}

	// Batch-load artist counts and release counts
	labelIDs := make([]uint, len(labels))
	for i, l := range labels {
		labelIDs[i] = l.ID
	}

	artistCounts := make(map[uint]int)
	releaseCounts := make(map[uint]int)

	if len(labelIDs) > 0 {
		type CountResult struct {
			LabelID uint
			Count   int
		}

		// Artist counts
		var aCounts []CountResult
		s.db.Table("artist_labels").
			Select("label_id, COUNT(DISTINCT artist_id) as count").
			Where("label_id IN ?", labelIDs).
			Group("label_id").
			Find(&aCounts)
		for _, c := range aCounts {
			artistCounts[c.LabelID] = c.Count
		}

		// Release counts
		var rCounts []CountResult
		s.db.Table("release_labels").
			Select("label_id, COUNT(DISTINCT release_id) as count").
			Where("label_id IN ?", labelIDs).
			Group("label_id").
			Find(&rCounts)
		for _, c := range rCounts {
			releaseCounts[c.LabelID] = c.Count
		}
	}

	// Build responses
	responses := make([]*contracts.LabelListResponse, len(labels))
	for i, label := range labels {
		slug := ""
		if label.Slug != nil {
			slug = *label.Slug
		}
		responses[i] = &contracts.LabelListResponse{
			ID:           label.ID,
			Name:         label.Name,
			Slug:         slug,
			City:         label.City,
			State:        label.State,
			Status:       string(label.Status),
			ArtistCount:  artistCounts[label.ID],
			ReleaseCount: releaseCounts[label.ID],
		}
	}

	return responses, nil
}

// UpdateLabel updates an existing label
func (s *LabelService) UpdateLabel(labelID uint, req *contracts.UpdateLabelRequest) (*contracts.LabelDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if label exists
	var label models.Label
	err := s.db.First(&label, labelID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrLabelNotFound(labelID)
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}

	updates := map[string]interface{}{}

	if req.Name != nil {
		updates["name"] = *req.Name
		// Regenerate slug when name changes
		baseSlug := utils.GenerateArtistSlug(*req.Name)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&models.Label{}).Where("slug = ? AND id != ?", candidate, labelID).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
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
	if req.FoundedYear != nil {
		updates["founded_year"] = *req.FoundedYear
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Instagram != nil {
		updates["instagram"] = *req.Instagram
	}
	if req.Facebook != nil {
		updates["facebook"] = *req.Facebook
	}
	if req.Twitter != nil {
		updates["twitter"] = *req.Twitter
	}
	if req.YouTube != nil {
		updates["youtube"] = *req.YouTube
	}
	if req.Spotify != nil {
		updates["spotify"] = *req.Spotify
	}
	if req.SoundCloud != nil {
		updates["soundcloud"] = *req.SoundCloud
	}
	if req.Bandcamp != nil {
		updates["bandcamp"] = *req.Bandcamp
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}

	if len(updates) > 0 {
		err = s.db.Model(&models.Label{}).Where("id = ?", labelID).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update label: %w", err)
		}
	}

	return s.GetLabel(labelID)
}

// DeleteLabel deletes a label
func (s *LabelService) DeleteLabel(labelID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if label exists
	var label models.Label
	err := s.db.First(&label, labelID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrLabelNotFound(labelID)
		}
		return fmt.Errorf("failed to get label: %w", err)
	}

	// Delete the label (cascades handle junction cleanup via FK)
	err = s.db.Delete(&label).Error
	if err != nil {
		return fmt.Errorf("failed to delete label: %w", err)
	}

	return nil
}

// GetLabelRoster retrieves all artists on a label
func (s *LabelService) GetLabelRoster(labelID uint) ([]*contracts.LabelArtistResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify label exists
	var label models.Label
	if err := s.db.First(&label, labelID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrLabelNotFound(labelID)
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}

	// Get artist IDs from junction table
	var artistLabels []models.ArtistLabel
	s.db.Where("label_id = ?", labelID).Find(&artistLabels)

	if len(artistLabels) == 0 {
		return []*contracts.LabelArtistResponse{}, nil
	}

	artistIDs := make([]uint, len(artistLabels))
	for i, al := range artistLabels {
		artistIDs[i] = al.ArtistID
	}

	var artists []models.Artist
	s.db.Where("id IN ?", artistIDs).Order("name ASC").Find(&artists)

	responses := make([]*contracts.LabelArtistResponse, len(artists))
	for i, artist := range artists {
		slug := ""
		if artist.Slug != nil {
			slug = *artist.Slug
		}
		responses[i] = &contracts.LabelArtistResponse{
			ID:   artist.ID,
			Slug: slug,
			Name: artist.Name,
		}
	}

	return responses, nil
}

// GetLabelCatalog retrieves all releases on a label
func (s *LabelService) GetLabelCatalog(labelID uint) ([]*contracts.LabelReleaseResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify label exists
	var label models.Label
	if err := s.db.First(&label, labelID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrLabelNotFound(labelID)
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}

	// Get release IDs and catalog numbers from junction table
	var releaseLabels []models.ReleaseLabel
	s.db.Where("label_id = ?", labelID).Find(&releaseLabels)

	if len(releaseLabels) == 0 {
		return []*contracts.LabelReleaseResponse{}, nil
	}

	releaseIDs := make([]uint, len(releaseLabels))
	catalogMap := make(map[uint]*string)
	for i, rl := range releaseLabels {
		releaseIDs[i] = rl.ReleaseID
		catalogMap[rl.ReleaseID] = rl.CatalogNumber
	}

	var releases []models.Release
	s.db.Where("id IN ?", releaseIDs).Order("release_year DESC NULLS LAST, title ASC").Find(&releases)

	responses := make([]*contracts.LabelReleaseResponse, len(releases))
	for i, release := range releases {
		slug := ""
		if release.Slug != nil {
			slug = *release.Slug
		}
		responses[i] = &contracts.LabelReleaseResponse{
			ID:            release.ID,
			Title:         release.Title,
			Slug:          slug,
			ReleaseType:   string(release.ReleaseType),
			ReleaseYear:   release.ReleaseYear,
			CoverArtURL:   release.CoverArtURL,
			CatalogNumber: catalogMap[release.ID],
		}
	}

	return responses, nil
}

// AddArtistToLabel creates an artist-label association (idempotent — no error if link already exists)
func (s *LabelService) AddArtistToLabel(labelID, artistID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Verify label exists
	var label models.Label
	if err := s.db.First(&label, labelID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrLabelNotFound(labelID)
		}
		return fmt.Errorf("failed to get label: %w", err)
	}

	// Verify artist exists
	var artist models.Artist
	if err := s.db.First(&artist, artistID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("artist not found: %d", artistID)
		}
		return fmt.Errorf("failed to get artist: %w", err)
	}

	// Idempotent: use FirstOrCreate to skip if already exists
	al := models.ArtistLabel{
		ArtistID: artistID,
		LabelID:  labelID,
	}
	if err := s.db.Where("artist_id = ? AND label_id = ?", artistID, labelID).FirstOrCreate(&al).Error; err != nil {
		return fmt.Errorf("failed to create artist-label link: %w", err)
	}

	return nil
}

// AddReleaseToLabel creates a release-label association (idempotent — no error if link already exists)
func (s *LabelService) AddReleaseToLabel(labelID, releaseID uint, catalogNumber *string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Verify label exists
	var label models.Label
	if err := s.db.First(&label, labelID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrLabelNotFound(labelID)
		}
		return fmt.Errorf("failed to get label: %w", err)
	}

	// Verify release exists
	var release models.Release
	if err := s.db.First(&release, releaseID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("release not found: %d", releaseID)
		}
		return fmt.Errorf("failed to get release: %w", err)
	}

	// Idempotent: use FirstOrCreate to skip if already exists
	rl := models.ReleaseLabel{
		ReleaseID:     releaseID,
		LabelID:       labelID,
		CatalogNumber: catalogNumber,
	}
	if err := s.db.Where("release_id = ? AND label_id = ?", releaseID, labelID).FirstOrCreate(&rl).Error; err != nil {
		return fmt.Errorf("failed to create release-label link: %w", err)
	}

	return nil
}

// buildDetailResponse converts a Label model to contracts.LabelDetailResponse
func (s *LabelService) buildDetailResponse(label *models.Label) (*contracts.LabelDetailResponse, error) {
	slug := ""
	if label.Slug != nil {
		slug = *label.Slug
	}

	// Count artists
	var artistCount int64
	s.db.Table("artist_labels").Where("label_id = ?", label.ID).Count(&artistCount)

	// Count releases
	var releaseCount int64
	s.db.Table("release_labels").Where("label_id = ?", label.ID).Count(&releaseCount)

	return &contracts.LabelDetailResponse{
		ID:          label.ID,
		Name:        label.Name,
		Slug:        slug,
		City:        label.City,
		State:       label.State,
		Country:     label.Country,
		FoundedYear: label.FoundedYear,
		Status:      string(label.Status),
		Description: label.Description,
		Social: contracts.SocialResponse{
			Instagram:  label.Social.Instagram,
			Facebook:   label.Social.Facebook,
			Twitter:    label.Social.Twitter,
			YouTube:    label.Social.YouTube,
			Spotify:    label.Social.Spotify,
			SoundCloud: label.Social.SoundCloud,
			Bandcamp:   label.Social.Bandcamp,
			Website:    label.Social.Website,
		},
		ArtistCount:  int(artistCount),
		ReleaseCount: int(releaseCount),
		CreatedAt:    label.CreatedAt,
		UpdatedAt:    label.UpdatedAt,
	}, nil
}
