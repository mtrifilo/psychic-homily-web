package services

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// ScraperService handles importing scraped event data into the database
type ScraperService struct {
	db           *gorm.DB
	venueService *VenueService
}

// NewScraperService creates a new scraper service
func NewScraperService() *ScraperService {
	return &ScraperService{
		db:           db.GetDB(),
		venueService: NewVenueService(),
	}
}

// ScrapedEvent represents an event from the Node.js scraper JSON output
type ScrapedEvent struct {
	ID         string   `json:"id"`         // External event ID (from the venue's system)
	Title      string   `json:"title"`      // Event title (typically artist names)
	Date       string   `json:"date"`       // Event date in ISO format (e.g., "2026-01-25")
	Venue      string   `json:"venue"`      // Venue name
	VenueSlug  string   `json:"venueSlug"`  // Venue identifier (e.g., "valley-bar")
	ImageURL   *string  `json:"imageUrl"`   // Event image URL (optional)
	DoorsTime  *string  `json:"doorsTime"`  // Doors time (e.g., "6:30 pm")
	ShowTime   *string  `json:"showTime"`   // Show time (e.g., "7:00 pm")
	TicketURL  *string  `json:"ticketUrl"`  // Ticket purchase URL (optional)
	Artists    []string `json:"artists"`    // List of artists (from event detail page)
	ScrapedAt  string   `json:"scrapedAt"`  // When the event was scraped (ISO timestamp)
}

// ImportResult contains statistics about the import operation
type ImportResult struct {
	Total      int      `json:"total"`       // Total events processed
	Imported   int      `json:"imported"`    // Successfully imported
	Duplicates int      `json:"duplicates"`  // Skipped due to deduplication
	Rejected   int      `json:"rejected"`    // Skipped due to matching rejected shows
	Errors     int      `json:"errors"`      // Failed to import
	Messages   []string `json:"messages"`    // Detailed messages for each event
}

// VenueConfig maps venue slugs to their database info
// This should be configured based on known venues
var VenueConfig = map[string]struct {
	Name    string
	City    string
	State   string
	Address string
}{
	"valley-bar": {
		Name:    "Valley Bar",
		City:    "Phoenix",
		State:   "AZ",
		Address: "130 N Central Ave",
	},
	"crescent-ballroom": {
		Name:    "Crescent Ballroom",
		City:    "Phoenix",
		State:   "AZ",
		Address: "308 N 2nd Ave",
	},
}

// ImportFromJSON imports events from a JSON file
func (s *ScraperService) ImportFromJSON(filepath string, dryRun bool) (*ImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Read the JSON file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON - could be a single venue's events or multiple venues
	var events []ScrapedEvent

	// Try parsing as array first
	if err := json.Unmarshal(data, &events); err != nil {
		// Try parsing as object with venue keys
		var venueEvents map[string][]ScrapedEvent
		if err := json.Unmarshal(data, &venueEvents); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		// Flatten into single array
		for _, ve := range venueEvents {
			events = append(events, ve...)
		}
	}

	result := &ImportResult{
		Total:    len(events),
		Messages: make([]string, 0),
	}

	for _, event := range events {
		msg, status := s.importEvent(&event, dryRun)
		result.Messages = append(result.Messages, msg)

		switch status {
		case "imported":
			result.Imported++
		case "duplicate":
			result.Duplicates++
		case "rejected":
			result.Rejected++
		case "error":
			result.Errors++
		}
	}

	return result, nil
}

// importEvent imports a single scraped event
// Returns a message and status ("imported", "duplicate", "rejected", "error")
func (s *ScraperService) importEvent(event *ScrapedEvent, dryRun bool) (string, string) {
	// Validate required fields
	if event.ID == "" || event.VenueSlug == "" {
		return fmt.Sprintf("SKIP: Missing required fields (id=%s, venueSlug=%s)", event.ID, event.VenueSlug), "error"
	}

	// Check for duplicate (same source_venue + source_event_id)
	var existing models.Show
	err := s.db.Where("source_venue = ? AND source_event_id = ?", event.VenueSlug, event.ID).First(&existing).Error
	if err == nil {
		return fmt.Sprintf("DUPLICATE: %s (ID: %s) already imported as show #%d", event.Title, event.ID, existing.ID), "duplicate"
	} else if err != gorm.ErrRecordNotFound {
		return fmt.Sprintf("ERROR: Failed to check duplicate: %v", err), "error"
	}

	// Parse event date
	eventDate, err := parseEventDate(event.Date, event.ShowTime)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to parse date for %s: %v", event.Title, err), "error"
	}

	// Look up venue configuration
	venueConfig, ok := VenueConfig[event.VenueSlug]
	if !ok {
		return fmt.Sprintf("ERROR: Unknown venue slug: %s", event.VenueSlug), "error"
	}

	// Check if there's a rejected show at the same venue on the same date
	// This prevents re-importing events that were previously rejected
	startOfDay := time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var rejectedShow models.Show
	err = s.db.Joins("JOIN show_venues ON shows.id = show_venues.show_id").
		Joins("JOIN venues ON show_venues.venue_id = venues.id").
		Where("LOWER(venues.name) = LOWER(?) AND shows.event_date >= ? AND shows.event_date < ? AND shows.status = ?",
			venueConfig.Name, startOfDay, endOfDay, models.ShowStatusRejected).
		First(&rejectedShow).Error
	if err == nil {
		return fmt.Sprintf("REJECTED: %s matches previously rejected show #%d at %s on %s",
			event.Title, rejectedShow.ID, venueConfig.Name, eventDate.Format("2006-01-02")), "rejected"
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: %s at %s on %s", event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04")), "imported"
	}

	// Create the show
	err = s.createShowFromEvent(event, eventDate, venueConfig)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to create show: %v", err), "error"
	}

	return fmt.Sprintf("IMPORTED: %s at %s on %s", event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04")), "imported"
}

// createShowFromEvent creates a show record from a scraped event
func (s *ScraperService) createShowFromEvent(event *ScrapedEvent, eventDate time.Time, venueConfig struct {
	Name    string
	City    string
	State   string
	Address string
}) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Parse scraped_at timestamp
		scrapedAt, err := time.Parse(time.RFC3339, event.ScrapedAt)
		if err != nil {
			scrapedAt = time.Now().UTC()
		}

		// Build description from available info
		var descParts []string
		if event.DoorsTime != nil && *event.DoorsTime != "" {
			descParts = append(descParts, fmt.Sprintf("Doors: %s", *event.DoorsTime))
		}
		if event.ShowTime != nil && *event.ShowTime != "" {
			descParts = append(descParts, fmt.Sprintf("Show: %s", *event.ShowTime))
		}
		if event.TicketURL != nil && *event.TicketURL != "" {
			descParts = append(descParts, fmt.Sprintf("Tickets: %s", *event.TicketURL))
		}
		var description *string
		if len(descParts) > 0 {
			desc := strings.Join(descParts, " | ")
			description = &desc
		}

		// Create the show
		show := &models.Show{
			Title:         event.Title,
			EventDate:     eventDate.UTC(),
			City:          &venueConfig.City,
			State:         &venueConfig.State,
			Description:   description,
			Status:        models.ShowStatusPending,
			Source:        models.ShowSourceScraper,
			SourceVenue:   &event.VenueSlug,
			SourceEventID: &event.ID,
			ScrapedAt:     &scrapedAt,
		}

		if err := tx.Create(show).Error; err != nil {
			return fmt.Errorf("failed to create show: %w", err)
		}

		// Find or create the venue
		address := venueConfig.Address
		venue, _, err := s.venueService.FindOrCreateVenue(
			venueConfig.Name,
			venueConfig.City,
			venueConfig.State,
			&address,
			nil,   // zipcode
			tx,    // use transaction
			false, // not admin - venue needs verification
		)
		if err != nil {
			return fmt.Errorf("failed to find/create venue: %w", err)
		}

		// Create show-venue association
		showVenue := models.ShowVenue{
			ShowID:  show.ID,
			VenueID: venue.ID,
		}
		if err := tx.Create(&showVenue).Error; err != nil {
			return fmt.Errorf("failed to create show-venue association: %w", err)
		}

		// Use artists array from scraper if available, otherwise parse from title
		var artistNames []string
		if len(event.Artists) > 0 {
			artistNames = event.Artists
		} else {
			// Fall back to parsing from title
			artistNames = parseArtistsFromTitle(event.Title)
		}

		for position, artistName := range artistNames {
			artistName = strings.TrimSpace(artistName)
			if artistName == "" {
				continue
			}

			// Find or create artist
			var artist models.Artist
			err := tx.Where("LOWER(name) = LOWER(?)", artistName).First(&artist).Error
			if err == gorm.ErrRecordNotFound {
				artist = models.Artist{Name: artistName}
				if err := tx.Create(&artist).Error; err != nil {
					return fmt.Errorf("failed to create artist %s: %w", artistName, err)
				}
			} else if err != nil {
				return fmt.Errorf("failed to find artist %s: %w", artistName, err)
			}

			// Determine set type (first artist is headliner)
			setType := "opener"
			if position == 0 {
				setType = "headliner"
			}

			// Create show-artist association
			showArtist := models.ShowArtist{
				ShowID:   show.ID,
				ArtistID: artist.ID,
				Position: position,
				SetType:  setType,
			}
			if err := tx.Create(&showArtist).Error; err != nil {
				return fmt.Errorf("failed to create show-artist association: %w", err)
			}
		}

		return nil
	})
}

// parseEventDate parses the event date and optional show time into a time.Time
func parseEventDate(dateStr string, showTime *string) (time.Time, error) {
	// Try parsing ISO date format (2026-01-25)
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		// Try other common formats
		date, err = time.Parse("2006-01-02T15:04:05Z", dateStr)
		if err != nil {
			date, err = time.Parse(time.RFC3339, dateStr)
			if err != nil {
				return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
			}
		}
	}

	// If show time is provided, try to parse and combine with date
	if showTime != nil && *showTime != "" {
		// Parse time like "7:00 pm" or "7:00 PM"
		timeStr := strings.ToLower(strings.TrimSpace(*showTime))
		timeStr = strings.ReplaceAll(timeStr, " ", "")

		var hour, minute int
		var period string

		_, err := fmt.Sscanf(timeStr, "%d:%d%s", &hour, &minute, &period)
		if err == nil {
			if strings.HasPrefix(period, "pm") && hour != 12 {
				hour += 12
			} else if strings.HasPrefix(period, "am") && hour == 12 {
				hour = 0
			}
			date = time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, time.UTC)
		}
	}

	return date.UTC(), nil
}

// parseArtistsFromTitle extracts artist names from event title
func parseArtistsFromTitle(title string) []string {
	// Common separators in event titles
	// Try comma first
	if strings.Contains(title, ",") {
		return splitAndTrim(title, ",")
	}

	// Try " with " (case insensitive)
	if idx := strings.Index(strings.ToLower(title), " with "); idx > 0 {
		first := title[:idx]
		rest := title[idx+6:] // len(" with ") = 6
		artists := []string{first}
		artists = append(artists, splitAndTrim(rest, ",")...)
		return artists
	}

	// Try " / " or " | "
	for _, sep := range []string{" / ", " | ", " + "} {
		if strings.Contains(title, sep) {
			return splitAndTrim(title, sep)
		}
	}

	// Try " & " but be careful about names like "Tom & Jerry"
	if strings.Contains(title, " & ") {
		parts := strings.Split(title, " & ")
		// Only split if we have clearly distinct artists (parts have reasonable length)
		if len(parts) == 2 && len(parts[0]) > 10 && len(parts[1]) > 10 {
			return splitAndTrim(title, " & ")
		}
	}

	// No separator found, treat entire title as single artist
	return []string{strings.TrimSpace(title)}
}

// splitAndTrim splits a string by separator and trims whitespace from each part
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ImportFromJSONWithDB imports events using a provided database connection
// Useful for testing or CLI tools that manage their own DB connection
func (s *ScraperService) ImportFromJSONWithDB(filepath string, dryRun bool, database *gorm.DB) (*ImportResult, error) {
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Temporarily swap the db connection
	originalDB := s.db
	s.db = database
	defer func() {
		s.db = originalDB
	}()

	return s.ImportFromJSON(filepath, dryRun)
}
