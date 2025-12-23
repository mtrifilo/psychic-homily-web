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
	ID        uint           `json:"id"`
	Name      string         `json:"name"`
	Address   *string        `json:"address"`
	City      string         `json:"city"`
	State     string         `json:"state"`
	Zipcode   *string        `json:"zipcode"`
	Verified  bool           `json:"verified"` // Admin-verified as legitimate venue
	Social    SocialResponse `json:"social"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
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

	// Create the venue - verified if created by admin, unverified otherwise
	venue := &models.Venue{
		Name:     req.Name,
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

	// Check if name/city is being updated and if it conflicts with existing venue
	// Get current venue to check its city
	var currentVenue models.Venue
	err := s.db.First(&currentVenue, venueID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("venue not found")
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
			Where("name ILIKE ?", "%"+query+"%").
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
func (s *VenueService) FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*models.Venue, error) {
	// Use provided db or fall back to service's db
	query := db
	if query == nil {
		query = s.db
	}

	if query == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate required fields
	if name == "" {
		return nil, fmt.Errorf("venue name is required")
	}
	if city == "" {
		return nil, fmt.Errorf("venue city is required")
	}
	if state == "" {
		return nil, fmt.Errorf("venue state is required")
	}

	// Check if venue already exists by name and city
	var venue models.Venue
	err := query.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", name, city).First(&venue).Error

	if err == nil {
		// Venue exists, return it
		return &venue, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Venue doesn't exist, create it - verified if created by admin, unverified otherwise
	venue = models.Venue{
		Name:     name,
		Address:  address,
		City:     city,
		State:    state,
		Zipcode:  zipcode,
		Verified: isAdmin, // Admins create verified venues, non-admins require approval
		Social:   models.Social{}, // Empty social fields
	}

	if err := query.Create(&venue).Error; err != nil {
		return nil, fmt.Errorf("failed to create venue: %w", err)
	}

	return &venue, nil
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
			return nil, fmt.Errorf("venue not found")
		}
		return nil, fmt.Errorf("failed to get venue: %w", err)
	}

	// Check if already verified
	if venue.Verified {
		return s.buildVenueResponse(&venue), nil
	}

	// Update verified status
	if err := s.db.Model(&venue).Update("verified", true).Error; err != nil {
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
	return &VenueDetailResponse{
		ID:       venue.ID,
		Name:     venue.Name,
		Address:  venue.Address,
		City:     venue.City,
		State:    venue.State,
		Zipcode:  venue.Zipcode,
		Verified: venue.Verified,
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
