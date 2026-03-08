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

// CreateReleaseRequest represents the data needed to create a new release
type CreateReleaseRequest struct {
	Title       string                      `json:"title" validate:"required"`
	ReleaseType string                      `json:"release_type"`
	ReleaseYear *int                        `json:"release_year"`
	ReleaseDate *string                     `json:"release_date"`
	CoverArtURL *string                     `json:"cover_art_url"`
	Description *string                     `json:"description"`
	Artists     []CreateReleaseArtistEntry  `json:"artists"`
	ExternalLinks []CreateReleaseLinkEntry  `json:"external_links"`
}

// CreateReleaseArtistEntry represents an artist-role pair for release creation
type CreateReleaseArtistEntry struct {
	ArtistID uint   `json:"artist_id"`
	Role     string `json:"role"`
}

// CreateReleaseLinkEntry represents an external link for release creation
type CreateReleaseLinkEntry struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

// UpdateReleaseRequest represents the data that can be updated on a release
type UpdateReleaseRequest struct {
	Title       *string `json:"title"`
	ReleaseType *string `json:"release_type"`
	ReleaseYear *int    `json:"release_year"`
	ReleaseDate *string `json:"release_date"`
	CoverArtURL *string `json:"cover_art_url"`
	Description *string `json:"description"`
}

// ReleaseDetailResponse represents the release data returned to clients
type ReleaseDetailResponse struct {
	ID            uint                         `json:"id"`
	Title         string                       `json:"title"`
	Slug          string                       `json:"slug"`
	ReleaseType   string                       `json:"release_type"`
	ReleaseYear   *int                         `json:"release_year"`
	ReleaseDate   *string                      `json:"release_date"`
	CoverArtURL   *string                      `json:"cover_art_url"`
	Description   *string                      `json:"description"`
	Artists       []ReleaseArtistResponse      `json:"artists"`
	ExternalLinks []ReleaseExternalLinkResponse `json:"external_links"`
	CreatedAt     time.Time                    `json:"created_at"`
	UpdatedAt     time.Time                    `json:"updated_at"`
}

// ReleaseArtistResponse represents an artist on a release
type ReleaseArtistResponse struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// ReleaseExternalLinkResponse represents an external link for a release
type ReleaseExternalLinkResponse struct {
	ID       uint   `json:"id"`
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

// ReleaseListResponse represents a release in list views
type ReleaseListResponse struct {
	ID          uint    `json:"id"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	ReleaseType string  `json:"release_type"`
	ReleaseYear *int    `json:"release_year"`
	CoverArtURL *string `json:"cover_art_url"`
	ArtistCount int     `json:"artist_count"`
}

// ArtistReleaseListResponse extends ReleaseListResponse with the artist's role on that release
type ArtistReleaseListResponse struct {
	ReleaseListResponse
	Role string `json:"role"`
}

// CreateRelease creates a new release
func (s *ReleaseService) CreateRelease(req *CreateReleaseRequest) (*ReleaseDetailResponse, error) {
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
func (s *ReleaseService) GetRelease(releaseID uint) (*ReleaseDetailResponse, error) {
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
func (s *ReleaseService) GetReleaseBySlug(slug string) (*ReleaseDetailResponse, error) {
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

// ListReleases retrieves releases with optional filtering
func (s *ReleaseService) ListReleases(filters map[string]interface{}) ([]*ReleaseListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.Release{})

	// Apply filters
	if artistID, ok := filters["artist_id"].(uint); ok && artistID > 0 {
		query = query.Where("id IN (?)",
			s.db.Table("artist_releases").Select("release_id").Where("artist_id = ?", artistID),
		)
	}
	if releaseType, ok := filters["release_type"].(string); ok && releaseType != "" {
		query = query.Where("release_type = ?", releaseType)
	}
	if year, ok := filters["year"].(int); ok && year > 0 {
		query = query.Where("release_year = ?", year)
	}

	// Order by release_year DESC, title ASC
	query = query.Order("release_year DESC NULLS LAST, title ASC")

	var releases []models.Release
	err := query.Find(&releases).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	// Batch-load artist counts
	releaseIDs := make([]uint, len(releases))
	for i, r := range releases {
		releaseIDs[i] = r.ID
	}

	artistCounts := make(map[uint]int)
	if len(releaseIDs) > 0 {
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

	// Build responses
	responses := make([]*ReleaseListResponse, len(releases))
	for i, release := range releases {
		slug := ""
		if release.Slug != nil {
			slug = *release.Slug
		}
		responses[i] = &ReleaseListResponse{
			ID:          release.ID,
			Title:       release.Title,
			Slug:        slug,
			ReleaseType: string(release.ReleaseType),
			ReleaseYear: release.ReleaseYear,
			CoverArtURL: release.CoverArtURL,
			ArtistCount: artistCounts[release.ID],
		}
	}

	return responses, nil
}

// UpdateRelease updates an existing release
func (s *ReleaseService) UpdateRelease(releaseID uint, req *UpdateReleaseRequest) (*ReleaseDetailResponse, error) {
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
func (s *ReleaseService) GetReleasesForArtist(artistID uint) ([]*ReleaseListResponse, error) {
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

	return s.ListReleases(map[string]interface{}{"artist_id": artistID})
}

// GetReleasesForArtistWithRoles retrieves all releases for an artist, including their role on each release
func (s *ReleaseService) GetReleasesForArtistWithRoles(artistID uint) ([]*ArtistReleaseListResponse, error) {
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
		return []*ArtistReleaseListResponse{}, nil
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

	// Batch-load artist counts
	artistCounts := make(map[uint]int)
	if len(releaseIDs) > 0 {
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

	// Build responses
	responses := make([]*ArtistReleaseListResponse, len(releases))
	for i, release := range releases {
		slug := ""
		if release.Slug != nil {
			slug = *release.Slug
		}
		responses[i] = &ArtistReleaseListResponse{
			ReleaseListResponse: ReleaseListResponse{
				ID:          release.ID,
				Title:       release.Title,
				Slug:        slug,
				ReleaseType: string(release.ReleaseType),
				ReleaseYear: release.ReleaseYear,
				CoverArtURL: release.CoverArtURL,
				ArtistCount: artistCounts[release.ID],
			},
			Role: roleMap[release.ID],
		}
	}

	return responses, nil
}

// AddExternalLink adds an external link to a release
func (s *ReleaseService) AddExternalLink(releaseID uint, platform, url string) (*ReleaseExternalLinkResponse, error) {
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

	return &ReleaseExternalLinkResponse{
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

// buildDetailResponse converts a Release model to ReleaseDetailResponse
func (s *ReleaseService) buildDetailResponse(release *models.Release) (*ReleaseDetailResponse, error) {
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
	artistResponses := make([]ReleaseArtistResponse, 0, len(artistReleases))
	for _, ar := range artistReleases {
		if artistModel, ok := artistMap[ar.ArtistID]; ok {
			artistSlug := ""
			if artistModel.Slug != nil {
				artistSlug = *artistModel.Slug
			}
			artistResponses = append(artistResponses, ReleaseArtistResponse{
				ID:   artistModel.ID,
				Slug: artistSlug,
				Name: artistModel.Name,
				Role: string(ar.Role),
			})
		}
	}

	// Build external link responses
	linkResponses := make([]ReleaseExternalLinkResponse, len(release.ExternalLinks))
	for i, link := range release.ExternalLinks {
		linkResponses[i] = ReleaseExternalLinkResponse{
			ID:       link.ID,
			Platform: link.Platform,
			URL:      link.URL,
		}
	}

	return &ReleaseDetailResponse{
		ID:            release.ID,
		Title:         release.Title,
		Slug:          slug,
		ReleaseType:   string(release.ReleaseType),
		ReleaseYear:   release.ReleaseYear,
		ReleaseDate:   release.ReleaseDate,
		CoverArtURL:   release.CoverArtURL,
		Description:   release.Description,
		Artists:       artistResponses,
		ExternalLinks: linkResponses,
		CreatedAt:     release.CreatedAt,
		UpdatedAt:     release.UpdatedAt,
	}, nil
}
