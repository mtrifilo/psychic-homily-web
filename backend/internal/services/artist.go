package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// ArtistService handles artist-related business logic
type ArtistService struct {
	db *gorm.DB
}

// NewArtistService creates a new artist service
func NewArtistService() *ArtistService {
	return &ArtistService{
		db: db.GetDB(),
	}
}

// CreateArtistRequest represents the data needed to create a new artist
type CreateArtistRequest struct {
	Name       string  `json:"name" validate:"required"`
	State      *string `json:"state"`
	City       *string `json:"city"`
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// ArtistDetailResponse represents the artist data returned to clients
type ArtistDetailResponse struct {
	ID        uint           `json:"id"`
	Name      string         `json:"name"`
	State     *string        `json:"state"`
	City      *string        `json:"city"`
	Social    SocialResponse `json:"social"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// SocialResponse represents social media links
type SocialResponse struct {
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// CreateArtist creates a new artist
func (s *ArtistService) CreateArtist(req *CreateArtistRequest) (*ArtistDetailResponse, error) {
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

	// Create the artist
	artist := &models.Artist{
		Name:  req.Name,
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
func (s *ArtistService) GetArtist(artistID uint) (*ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var artist models.Artist
	err := s.db.First(&artist, artistID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("artist not found")
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	return s.buildArtistResponse(&artist), nil
}

// GetArtistByName retrieves an artist by name (case-insensitive)
func (s *ArtistService) GetArtistByName(name string) (*ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var artist models.Artist
	err := s.db.Where("LOWER(name) = LOWER(?)", name).First(&artist).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("artist not found")
		}
		return nil, fmt.Errorf("failed to get artist: %w", err)
	}

	return s.buildArtistResponse(&artist), nil
}

// GetArtists retrieves artists with optional filtering
func (s *ArtistService) GetArtists(filters map[string]interface{}) ([]*ArtistDetailResponse, error) {
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

	var artists []models.Artist
	err := query.Find(&artists).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get artists: %w", err)
	}

	// Build responses
	responses := make([]*ArtistDetailResponse, len(artists))
	for i, artist := range artists {
		responses[i] = s.buildArtistResponse(&artist)
	}

	return responses, nil
}

// UpdateArtist updates an existing artist
func (s *ArtistService) UpdateArtist(artistID uint, updates map[string]interface{}) (*ArtistDetailResponse, error) {
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
			return fmt.Errorf("artist not found")
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
		return fmt.Errorf("cannot delete artist: associated with %d shows", count)
	}

	// Delete the artist
	err = s.db.Delete(&artist).Error
	if err != nil {
		return fmt.Errorf("failed to delete artist: %w", err)
	}

	return nil
}

// buildArtistResponse converts an Artist model to ArtistDetailResponse
func (s *ArtistService) buildArtistResponse(artist *models.Artist) *ArtistDetailResponse {
	return &ArtistDetailResponse{
		ID:    artist.ID,
		Name:  artist.Name,
		State: artist.State,
		City:  artist.City,
		Social: SocialResponse{
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
