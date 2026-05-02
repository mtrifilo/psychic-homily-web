package engagement

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

const (
	// CalendarTokenPrefix is prepended to calendar tokens for identification
	CalendarTokenPrefix = "phcal_"

	// calendarTokenLength is the length of the generated token in bytes (32 bytes = 64 hex chars)
	calendarTokenLength = 32
)

// CalendarService handles calendar feed token and ICS generation
type CalendarService struct {
	db           *gorm.DB
	savedShowSvc contracts.SavedShowServiceInterface
}

// NewCalendarService creates a new calendar service
func NewCalendarService(database *gorm.DB, savedShowSvc contracts.SavedShowServiceInterface) *CalendarService {
	if database == nil {
		database = db.GetDB()
	}
	return &CalendarService{
		db:           database,
		savedShowSvc: savedShowSvc,
	}
}

// generateCalendarToken creates a cryptographically secure random calendar token
func generateCalendarToken() (string, error) {
	bytes := make([]byte, calendarTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return CalendarTokenPrefix + hex.EncodeToString(bytes), nil
}

// hashCalendarToken creates a SHA-256 hash of a token for storage
func hashCalendarToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// CreateToken generates a new calendar token for a user, replacing any existing token
func (s *CalendarService) CreateToken(userID uint, apiBaseURL string) (*contracts.CalendarTokenCreateResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	plainToken, err := generateCalendarToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	tokenHash := hashCalendarToken(plainToken)

	// Delete existing + insert new in a transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete any existing token for this user
		if err := tx.Where("user_id = ?", userID).Delete(&engagementm.CalendarToken{}).Error; err != nil {
			return fmt.Errorf("failed to delete existing token: %w", err)
		}

		token := &engagementm.CalendarToken{
			UserID:    userID,
			TokenHash: tokenHash,
		}
		if err := tx.Create(token).Error; err != nil {
			return fmt.Errorf("failed to create token: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Fetch the created token to get the server-set created_at
	var created engagementm.CalendarToken
	if err := s.db.Where("user_id = ?", userID).First(&created).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch created token: %w", err)
	}

	feedURL := fmt.Sprintf("%s/calendar/%s", apiBaseURL, plainToken)

	return &contracts.CalendarTokenCreateResponse{
		Token:     plainToken,
		FeedURL:   feedURL,
		CreatedAt: created.CreatedAt,
	}, nil
}

// GetTokenStatus checks whether a user has a calendar token
func (s *CalendarService) GetTokenStatus(userID uint) (*contracts.CalendarTokenStatusResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var token engagementm.CalendarToken
	err := s.db.Where("user_id = ?", userID).First(&token).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &contracts.CalendarTokenStatusResponse{HasToken: false}, nil
		}
		return nil, fmt.Errorf("failed to check token status: %w", err)
	}

	return &contracts.CalendarTokenStatusResponse{
		HasToken:  true,
		CreatedAt: &token.CreatedAt,
	}, nil
}

// DeleteToken removes a user's calendar token
func (s *CalendarService) DeleteToken(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("user_id = ?", userID).Delete(&engagementm.CalendarToken{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no calendar token found")
	}
	return nil
}

// ValidateCalendarToken validates a plaintext calendar token and returns the associated user
func (s *CalendarService) ValidateCalendarToken(plainToken string) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	tokenHash := hashCalendarToken(plainToken)

	var token engagementm.CalendarToken
	err := s.db.Preload("User").Where("token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("invalid calendar token")
		}
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	if !token.User.IsActive {
		return nil, fmt.Errorf("user account is not active")
	}

	return &token.User, nil
}

// GenerateICSFeed creates an ICS calendar feed for a user's saved shows
func (s *CalendarService) GenerateICSFeed(userID uint, frontendURL string) ([]byte, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Fetch saved shows (large limit to get all relevant shows)
	shows, _, err := s.savedShowSvc.GetUserSavedShows(userID, 500, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch saved shows: %w", err)
	}

	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	cal.SetProductId("-//Psychic Homily//Calendar Feed//EN")
	cal.SetName("Psychic Homily - My Shows")
	cal.SetDescription("Your saved shows from Psychic Homily")
	cal.SetXWRCalName("Psychic Homily - My Shows")

	now := time.Now()
	cutoff := now.AddDate(0, 0, -30) // 30 days in the past

	for _, show := range shows {
		// Filter: approved, not cancelled, event_date within range
		if show.Status != "approved" {
			continue
		}
		if show.IsCancelled {
			continue
		}
		if show.EventDate.Before(cutoff) {
			continue
		}

		event := cal.AddEvent(fmt.Sprintf("show-%d@psychichomily.com", show.ID))
		event.SetCreatedTime(show.CreatedAt)
		event.SetModifiedAt(show.UpdatedAt)
		event.SetStartAt(show.EventDate)
		// Default to 3-hour duration for shows
		event.SetEndAt(show.EventDate.Add(3 * time.Hour))

		// Summary
		summary := show.Title
		if show.IsSoldOut {
			summary += " [SOLD OUT]"
		}
		event.SetSummary(summary)

		// Location
		if len(show.Venues) > 0 {
			venue := show.Venues[0]
			location := venue.Name
			if venue.City != "" {
				location += ", " + venue.City
			}
			if venue.State != "" {
				location += ", " + venue.State
			}
			event.SetLocation(location)
		}

		// Description
		var descParts []string

		// Artist lineup
		if len(show.Artists) > 0 {
			names := make([]string, len(show.Artists))
			for i, a := range show.Artists {
				names[i] = a.Name
			}
			descParts = append(descParts, "Artists: "+strings.Join(names, ", "))
		}

		if show.Price != nil {
			descParts = append(descParts, fmt.Sprintf("Price: $%.0f", *show.Price))
		}
		if show.AgeRequirement != nil && *show.AgeRequirement != "" {
			descParts = append(descParts, "Ages: "+*show.AgeRequirement)
		}

		// Show URL
		slug := show.Slug
		if slug == "" {
			slug = fmt.Sprintf("%d", show.ID)
		}
		showURL := fmt.Sprintf("%s/shows/%s", frontendURL, slug)
		descParts = append(descParts, showURL)

		event.SetDescription(strings.Join(descParts, "\n"))
		event.SetURL(showURL)
	}

	return []byte(cal.Serialize()), nil
}
