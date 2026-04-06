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

// ReleaseService handles release-related business logic
type ReleaseService struct {
	db *gorm.DB
}

// NewReleaseService creates a new release service
func NewReleaseService(database *gorm.DB) *ReleaseService {
	if database == nil {
		database = db.GetDB()
	}
	return &ReleaseService{
		db: database,
	}
}

// CreateRelease creates a new release
func (s *ReleaseService) CreateRelease(req *contracts.CreateReleaseRequest) (*contracts.ReleaseDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Generate unique slug
	baseSlug := utils.GenerateArtistSlug(req.Title)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Release{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	// Determine release type, default to "lp"
	releaseType := models.ReleaseType(req.ReleaseType)
	if releaseType == "" {
		releaseType = models.ReleaseTypeLP
	}

	// Create the release
	release := &models.Release{
		Title:       req.Title,
		Slug:        &slug,
		ReleaseType: releaseType,
		ReleaseYear: req.ReleaseYear,
		ReleaseDate: req.ReleaseDate,
		CoverArtURL: req.CoverArtURL,
		Description: req.Description,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(release).Error; err != nil {
			return fmt.Errorf("failed to create release: %w", err)
		}

		// Create artist_releases entries
		for i, artistEntry := range req.Artists {
			role := artistEntry.Role
			if role == "" {
				role = string(models.ArtistReleaseRoleMain)
			}
			ar := &models.ArtistRelease{
				ArtistID:  artistEntry.ArtistID,
				ReleaseID: release.ID,
				Role:      models.ArtistReleaseRole(role),
				Position:  i,
			}
			if err := tx.Create(ar).Error; err != nil {
				return fmt.Errorf("failed to create artist-release link: %w", err)
			}
		}

		// Create external links
		for _, linkEntry := range req.ExternalLinks {
			link := &models.ReleaseExternalLink{
				ReleaseID: release.ID,
				Platform:  linkEntry.Platform,
				URL:       linkEntry.URL,
			}
			if err := tx.Create(link).Error; err != nil {
				return fmt.Errorf("failed to create external link: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetRelease(release.ID)
}

// GetRelease retrieves a release by ID
func (s *ReleaseService) GetRelease(releaseID uint) (*contracts.ReleaseDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var release models.Release
	err := s.db.Preload("ExternalLinks").First(&release, releaseID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrReleaseNotFound(releaseID)
		}
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	return s.buildDetailResponse(&release)
}

// GetReleaseBySlug retrieves a release by slug
func (s *ReleaseService) GetReleaseBySlug(slug string) (*contracts.ReleaseDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var release models.Release
	err := s.db.Preload("ExternalLinks").Where("slug = ?", slug).First(&release).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrReleaseNotFound(0)
		}
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	return s.buildDetailResponse(&release)
}

// ListReleases retrieves releases with optional filtering, search, sorting, and pagination.
// Returns the list of releases and the total count matching the filters (before pagination).
func (s *ReleaseService) ListReleases(filters contracts.ReleaseListFilters) ([]*contracts.ReleaseListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.Release{})

	// Apply filters
	if filters.ArtistID > 0 {
		query = query.Where("releases.id IN (?)",
			s.db.Table("artist_releases").Select("release_id").Where("artist_id = ?", filters.ArtistID),
		)
	}
	if filters.ReleaseType != "" {
		query = query.Where("releases.release_type = ?", filters.ReleaseType)
	}
	if filters.Year > 0 {
		query = query.Where("releases.release_year = ?", filters.Year)
	}
	if filters.LabelID > 0 {
		query = query.Where("releases.id IN (?)",
			s.db.Table("release_labels").Select("release_id").Where("label_id = ?", filters.LabelID),
		)
	}
	if filters.Search != "" {
		searchPattern := "%" + filters.Search + "%"
		// Search by release title OR by artist name (via artist_releases + artists join)
		query = query.Where(
			"releases.title ILIKE ? OR releases.id IN (?)",
			searchPattern,
			s.db.Table("artist_releases").
				Select("artist_releases.release_id").
				Joins("JOIN artists ON artists.id = artist_releases.artist_id").
				Where("artists.name ILIKE ?", searchPattern),
		)
	}

	// Get total count before pagination
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count releases: %w", err)
	}

	// Apply sorting
	switch filters.Sort {
	case "oldest":
		query = query.Order("releases.release_year ASC NULLS LAST, releases.title ASC")
	case "title_asc":
		query = query.Order("releases.title ASC")
	case "title_desc":
		query = query.Order("releases.title DESC")
	case "recently_added":
		query = query.Order("releases.created_at DESC")
	default: // "newest" or empty
		query = query.Order("releases.release_year DESC NULLS LAST, releases.title ASC")
	}

	// Apply pagination
	limit := filters.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query = query.Limit(limit)
	if filters.Offset > 0 {
		query = query.Offset(filters.Offset)
	}

	var releases []models.Release
	err := query.Find(&releases).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list releases: %w", err)
	}

	responses, err := s.buildListResponses(releases)
	if err != nil {
		return nil, 0, err
	}
	return responses, total, nil
}

// SearchReleases searches for releases by title using ILIKE matching
func (s *ReleaseService) SearchReleases(query string) ([]*contracts.ReleaseListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Return empty results for empty query
	if query == "" {
		return []*contracts.ReleaseListResponse{}, nil
	}

	var releases []models.Release
	var err error

	if len(query) <= 2 {
		// For short queries: prefix match
		err = s.db.
			Where("LOWER(title) LIKE LOWER(?)", query+"%").
			Order("title ASC").
			Limit(20).
			Find(&releases).Error
	} else {
		// For longer queries: ILIKE substring match, ordered by title
		err = s.db.
			Where("title ILIKE ?", "%"+query+"%").
			Order("title ASC").
			Limit(20).
			Find(&releases).Error
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search releases: %w", err)
	}

	return s.buildListResponses(releases)
}

// UpdateRelease updates an existing release
func (s *ReleaseService) UpdateRelease(releaseID uint, req *contracts.UpdateReleaseRequest) (*contracts.ReleaseDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if release exists
	var release models.Release
	err := s.db.First(&release, releaseID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrReleaseNotFound(releaseID)
		}
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	updates := map[string]interface{}{}

	if req.Title != nil {
		updates["title"] = *req.Title
		// Regenerate slug when title changes
		baseSlug := utils.GenerateArtistSlug(*req.Title)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&models.Release{}).Where("slug = ? AND id != ?", candidate, releaseID).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
	}
	if req.ReleaseType != nil {
		updates["release_type"] = *req.ReleaseType
	}
	if req.ReleaseYear != nil {
		updates["release_year"] = *req.ReleaseYear
	}
	if req.ReleaseDate != nil {
		updates["release_date"] = *req.ReleaseDate
	}
	if req.CoverArtURL != nil {
		updates["cover_art_url"] = *req.CoverArtURL
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if len(updates) > 0 {
		err = s.db.Model(&models.Release{}).Where("id = ?", releaseID).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update release: %w", err)
		}
	}

	return s.GetRelease(releaseID)
}

// DeleteRelease deletes a release
func (s *ReleaseService) DeleteRelease(releaseID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if release exists
	var release models.Release
	err := s.db.First(&release, releaseID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrReleaseNotFound(releaseID)
		}
		return fmt.Errorf("failed to get release: %w", err)
	}

	// Delete the release (cascades handle junction cleanup via FK)
	err = s.db.Delete(&release).Error
	if err != nil {
		return fmt.Errorf("failed to delete release: %w", err)
	}

	return nil
}

// GetReleasesForArtist retrieves all releases for a given artist
func (s *ReleaseService) GetReleasesForArtist(artistID uint) ([]*contracts.ReleaseListResponse, error) {
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

	releases, _, err := s.ListReleases(contracts.ReleaseListFilters{ArtistID: artistID})
	return releases, err
}

// GetReleasesForArtistWithRoles retrieves all releases for an artist, including their role on each release
func (s *ReleaseService) GetReleasesForArtistWithRoles(artistID uint) ([]*contracts.ArtistReleaseListResponse, error) {
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

	// Get artist_releases junction entries for this artist (includes role)
	var artistReleases []models.ArtistRelease
	if err := s.db.Where("artist_id = ?", artistID).Find(&artistReleases).Error; err != nil {
		return nil, fmt.Errorf("failed to get artist releases: %w", err)
	}

	if len(artistReleases) == 0 {
		return []*contracts.ArtistReleaseListResponse{}, nil
	}

	// Build role map: releaseID -> role (an artist can have multiple roles on a release,
	// but for grouping we use the first/primary one)
	roleMap := make(map[uint]string)
	releaseIDs := make([]uint, 0, len(artistReleases))
	for _, ar := range artistReleases {
		if _, exists := roleMap[ar.ReleaseID]; !exists {
			releaseIDs = append(releaseIDs, ar.ReleaseID)
		}
		roleMap[ar.ReleaseID] = string(ar.Role)
	}

	// Fetch releases
	var releases []models.Release
	if err := s.db.Where("id IN ?", releaseIDs).Order("release_year DESC NULLS LAST, title ASC").Find(&releases).Error; err != nil {
		return nil, fmt.Errorf("failed to get releases: %w", err)
	}

	// Build base list responses (includes artist names, counts, labels)
	listResponses, err := s.buildListResponses(releases)
	if err != nil {
		return nil, err
	}

	// Wrap with role info
	responses := make([]*contracts.ArtistReleaseListResponse, len(listResponses))
	for i, lr := range listResponses {
		responses[i] = &contracts.ArtistReleaseListResponse{
			ReleaseListResponse: *lr,
			Role:                roleMap[lr.ID],
		}
	}

	return responses, nil
}

// AddExternalLink adds an external link to a release
func (s *ReleaseService) AddExternalLink(releaseID uint, platform, url string) (*contracts.ReleaseExternalLinkResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify release exists
	var release models.Release
	if err := s.db.First(&release, releaseID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrReleaseNotFound(releaseID)
		}
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	link := &models.ReleaseExternalLink{
		ReleaseID: releaseID,
		Platform:  platform,
		URL:       url,
	}

	if err := s.db.Create(link).Error; err != nil {
		return nil, fmt.Errorf("failed to create external link: %w", err)
	}

	return &contracts.ReleaseExternalLinkResponse{
		ID:       link.ID,
		Platform: link.Platform,
		URL:      link.URL,
	}, nil
}

// RemoveExternalLink removes an external link
func (s *ReleaseService) RemoveExternalLink(linkID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Delete(&models.ReleaseExternalLink{}, linkID)
	if result.Error != nil {
		return fmt.Errorf("failed to delete external link: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("external link not found")
	}

	return nil
}

// buildListResponses converts a slice of Release models to ReleaseListResponse, batch-loading
// artist counts, artist names, and primary label info.
func (s *ReleaseService) buildListResponses(releases []models.Release) ([]*contracts.ReleaseListResponse, error) {
	if len(releases) == 0 {
		return []*contracts.ReleaseListResponse{}, nil
	}

	releaseIDs := make([]uint, len(releases))
	for i, r := range releases {
		releaseIDs[i] = r.ID
	}

	// Batch-load artist counts
	artistCounts := make(map[uint]int)
	{
		type CountResult struct {
			ReleaseID uint
			Count     int
		}
		var counts []CountResult
		s.db.Table("artist_releases").
			Select("release_id, COUNT(DISTINCT artist_id) as count").
			Where("release_id IN ?", releaseIDs).
			Group("release_id").
			Find(&counts)
		for _, c := range counts {
			artistCounts[c.ReleaseID] = c.Count
		}
	}

	// Batch-load artist details (id, name, slug) per release via artist_releases + artists join
	releaseArtists := make(map[uint][]contracts.ReleaseListArtist)
	{
		type ArtistRow struct {
			ReleaseID uint
			ArtistID  uint
			Name      string
			Slug      *string
			Position  int
		}
		var rows []ArtistRow
		s.db.Table("artist_releases").
			Select("artist_releases.release_id, artist_releases.artist_id, artists.name, artists.slug, artist_releases.position").
			Joins("JOIN artists ON artists.id = artist_releases.artist_id").
			Where("artist_releases.release_id IN ?", releaseIDs).
			Order("artist_releases.position ASC").
			Find(&rows)
		for _, row := range rows {
			slug := ""
			if row.Slug != nil {
				slug = *row.Slug
			}
			releaseArtists[row.ReleaseID] = append(releaseArtists[row.ReleaseID], contracts.ReleaseListArtist{
				ID:   row.ArtistID,
				Name: row.Name,
				Slug: slug,
			})
		}
	}

	// Batch-load primary label (first label) per release via release_labels + labels join
	type LabelInfo struct {
		Name string
		Slug *string
	}
	releaseLabels := make(map[uint]*LabelInfo)
	{
		type LabelRow struct {
			ReleaseID uint
			Name      string
			Slug      *string
		}
		var rows []LabelRow
		s.db.Table("release_labels").
			Select("release_labels.release_id, labels.name, labels.slug").
			Joins("JOIN labels ON labels.id = release_labels.label_id").
			Where("release_labels.release_id IN ?", releaseIDs).
			Find(&rows)
		for _, row := range rows {
			// Use the first label found for each release
			if _, exists := releaseLabels[row.ReleaseID]; !exists {
				releaseLabels[row.ReleaseID] = &LabelInfo{
					Name: row.Name,
					Slug: row.Slug,
				}
			}
		}
	}

	// Build responses
	responses := make([]*contracts.ReleaseListResponse, len(releases))
	for i, release := range releases {
		slug := ""
		if release.Slug != nil {
			slug = *release.Slug
		}

		resp := &contracts.ReleaseListResponse{
			ID:          release.ID,
			Title:       release.Title,
			Slug:        slug,
			ReleaseType: string(release.ReleaseType),
			ReleaseYear: release.ReleaseYear,
			CoverArtURL: release.CoverArtURL,
			ArtistCount: artistCounts[release.ID],
			Artists:     releaseArtists[release.ID],
		}

		// Ensure Artists is never nil (always an empty slice for JSON)
		if resp.Artists == nil {
			resp.Artists = []contracts.ReleaseListArtist{}
		}

		if label, ok := releaseLabels[release.ID]; ok {
			resp.LabelName = &label.Name
			if label.Slug != nil {
				resp.LabelSlug = label.Slug
			}
		}

		responses[i] = resp
	}

	return responses, nil
}

// buildDetailResponse converts a Release model to contracts.ReleaseDetailResponse
func (s *ReleaseService) buildDetailResponse(release *models.Release) (*contracts.ReleaseDetailResponse, error) {
	slug := ""
	if release.Slug != nil {
		slug = *release.Slug
	}

	// Load artist_releases with artist data
	var artistReleases []models.ArtistRelease
	s.db.Where("release_id = ?", release.ID).Order("position ASC").Find(&artistReleases)

	// Batch-load artist models
	artistIDs := make([]uint, len(artistReleases))
	for i, ar := range artistReleases {
		artistIDs[i] = ar.ArtistID
	}

	artistMap := make(map[uint]*models.Artist)
	if len(artistIDs) > 0 {
		var artists []models.Artist
		s.db.Where("id IN ?", artistIDs).Find(&artists)
		for i := range artists {
			artistMap[artists[i].ID] = &artists[i]
		}
	}

	// Build artist responses
	artistResponses := make([]contracts.ReleaseArtistResponse, 0, len(artistReleases))
	for _, ar := range artistReleases {
		if artistModel, ok := artistMap[ar.ArtistID]; ok {
			artistSlug := ""
			if artistModel.Slug != nil {
				artistSlug = *artistModel.Slug
			}
			artistResponses = append(artistResponses, contracts.ReleaseArtistResponse{
				ID:   artistModel.ID,
				Slug: artistSlug,
				Name: artistModel.Name,
				Role: string(ar.Role),
			})
		}
	}

	// Build external link responses
	linkResponses := make([]contracts.ReleaseExternalLinkResponse, len(release.ExternalLinks))
	for i, link := range release.ExternalLinks {
		linkResponses[i] = contracts.ReleaseExternalLinkResponse{
			ID:       link.ID,
			Platform: link.Platform,
			URL:      link.URL,
		}
	}

	// Load labels via release_labels junction table
	var releaseLabels []models.ReleaseLabel
	s.db.Where("release_id = ?", release.ID).Find(&releaseLabels)

	labelIDs := make([]uint, len(releaseLabels))
	for i, rl := range releaseLabels {
		labelIDs[i] = rl.LabelID
	}

	labelResponses := make([]contracts.ReleaseLabelResponse, 0, len(releaseLabels))
	if len(labelIDs) > 0 {
		var labels []models.Label
		s.db.Where("id IN ?", labelIDs).Find(&labels)

		labelMap := make(map[uint]*models.Label)
		for i := range labels {
			labelMap[labels[i].ID] = &labels[i]
		}

		// Build label responses preserving junction table order
		for _, rl := range releaseLabels {
			if labelModel, ok := labelMap[rl.LabelID]; ok {
				labelSlug := ""
				if labelModel.Slug != nil {
					labelSlug = *labelModel.Slug
				}
				labelResponses = append(labelResponses, contracts.ReleaseLabelResponse{
					ID:            labelModel.ID,
					Name:          labelModel.Name,
					Slug:          labelSlug,
					CatalogNumber: rl.CatalogNumber,
				})
			}
		}
	}

	return &contracts.ReleaseDetailResponse{
		ID:            release.ID,
		Title:         release.Title,
		Slug:          slug,
		ReleaseType:   string(release.ReleaseType),
		ReleaseYear:   release.ReleaseYear,
		ReleaseDate:   release.ReleaseDate,
		CoverArtURL:   release.CoverArtURL,
		Description:   release.Description,
		Artists:       artistResponses,
		Labels:        labelResponses,
		ExternalLinks: linkResponses,
		CreatedAt:     release.CreatedAt,
		UpdatedAt:     release.UpdatedAt,
	}, nil
}
