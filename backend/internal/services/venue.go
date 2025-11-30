package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// VenueService handles venue-related business logic
type VenueService struct {
	db *gorm.DB
}

// NewVenueService creates a new venue service
func NewVenueService() *VenueService {
	return &VenueService{
		db: db.GetDB(),
	}
}

// CreateVenueRequest represents the data needed to create a new venue
type CreateVenueRequest struct {
	Name       string  `json:"name" validate:"required"`
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

// VenueDetailResponse represents the venue data returned to clients
type VenueDetailResponse struct {
	ID        uint           `json:"id"`
	Name      string         `json:"name"`
	Address   *string        `json:"address"`
	City      *string        `json:"city"`
	State     *string        `json:"state"`
	Zipcode   *string        `json:"zipcode"`
	Social    SocialResponse `json:"social"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// CreateVenue creates a new venue
func (s *VenueService) CreateVenue(req *CreateVenueRequest) (*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if venue already exists
	var existingVenue models.Venue
	err := s.db.Where("LOWER(name) = LOWER(?)", req.Name).First(&existingVenue).Error
	if err == nil {
		return nil, fmt.Errorf("venue with name '%s' already exists", req.Name)
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Create the venue
	venue := &models.Venue{
		Name:    req.Name,
		Address: req.Address,
		City:    req.City,
		State:   req.State,
		Zipcode: req.Zipcode,
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
			return nil, fmt.Errorf("venue not found")
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	return s.buildVenueResponse(&venue), nil
}

// GetVenueByName retrieves a venue by name (case-insensitive)
func (s *VenueService) GetVenueByName(name string) (*VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var venue models.Venue
	err := s.db.Where("LOWER(name) = LOWER(?)", name).First(&venue).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("venue not found")
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

	// Check if name is being updated and if it conflicts with existing venue
	if name, ok := updates["name"].(string); ok {
		var existingVenue models.Venue
		err := s.db.Where("LOWER(name) = LOWER(?) AND id != ?", name, venueID).First(&existingVenue).Error
		if err == nil {
			return nil, fmt.Errorf("venue with name '%s' already exists", name)
		} else if err != gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("failed to check existing venue: %w", err)
		}
	}

	err := s.db.Model(&models.Venue{}).Where("id = ?", venueID).Updates(updates).Error
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
			return fmt.Errorf("venue not found")
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
		return fmt.Errorf("cannot delete venue: associated with %d shows", count)
	}

	// Delete the venue
	err = s.db.Delete(&venue).Error
	if err != nil {
		return fmt.Errorf("failed to delete venue: %w", err)
	}

	return nil
}

// buildVenueResponse converts a Venue model to VenueDetailResponse
func (s *VenueService) buildVenueResponse(venue *models.Venue) *VenueDetailResponse {
	return &VenueDetailResponse{
		ID:      venue.ID,
		Name:    venue.Name,
		Address: venue.Address,
		City:    venue.City,
		State:   venue.State,
		Zipcode: venue.Zipcode,
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
