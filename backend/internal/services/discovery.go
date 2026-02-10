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
	"psychic-homily-backend/internal/utils"
)

// DiscoveryService handles importing discovered event data into the database
type DiscoveryService struct {
	db           *gorm.DB
	venueService *VenueService
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService(database *gorm.DB) *DiscoveryService {
	if database == nil {
		database = db.GetDB()
	}
	return &DiscoveryService{
		db:           database,
		venueService: NewVenueService(database),
	}
}

// DiscoveredEvent represents an event from the Node.js discovery app JSON output
type DiscoveredEvent struct {
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
	Total         int      `json:"total"`          // Total events processed
	Imported      int      `json:"imported"`       // Successfully imported
	Duplicates    int      `json:"duplicates"`     // Skipped due to deduplication
	Rejected      int      `json:"rejected"`       // Skipped due to matching rejected shows
	PendingReview int      `json:"pending_review"` // Flagged as potential duplicates for admin review
	Errors        int      `json:"errors"`         // Failed to import
	Messages      []string `json:"messages"`       // Detailed messages for each event
}

// VenueConfig maps venue slugs to their database info
// NOTE: When adding venues, also update:
//   - discovery/src/lib/config.ts (frontend config)
//   - discovery/src/server/index.ts (discovery server config)
var VenueConfig = map[string]struct {
	Name    string
	City    string
	State   string
	Address string
}{
	// Phoenix, AZ - Stateside Presents venues
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
	"the-van-buren": {
		Name:    "The Van Buren",
		City:    "Phoenix",
		State:   "AZ",
		Address: "401 W Van Buren St",
	},
	"celebrity-theatre": {
		Name:    "Celebrity Theatre",
		City:    "Phoenix",
		State:   "AZ",
		Address: "440 N 32nd St",
	},
	"arizona-financial-theatre": {
		Name:    "Arizona Financial Theatre",
		City:    "Phoenix",
		State:   "AZ",
		Address: "400 W Washington St",
	},

	// NOTE: Add more venues here as you implement providers for them.
	// Example venues from other cities:
	//
	// Denver, CO
	// "gothic-theatre": { Name: "Gothic Theatre", City: "Denver", State: "CO", Address: "3263 S Broadway" },
	// "bluebird-theater": { Name: "Bluebird Theater", City: "Denver", State: "CO", Address: "3317 E Colfax Ave" },
	//
	// Austin, TX
	// "mohawk": { Name: "Mohawk", City: "Austin", State: "TX", Address: "912 Red River St" },
}

// ImportFromJSON imports events from a JSON file
func (s *DiscoveryService) ImportFromJSON(filepath string, dryRun bool) (*ImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Read the JSON file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON - could be a single venue's events or multiple venues
	var events []DiscoveredEvent

	// Try parsing as array first
	if err := json.Unmarshal(data, &events); err != nil {
		// Try parsing as object with venue keys
		var venueEvents map[string][]DiscoveredEvent
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
		case "pending_review":
			result.PendingReview++
		case "error":
			result.Errors++
		}
	}

	return result, nil
}

// checkHeadlinerDuplicate checks if there's an existing non-rejected show with the same
// headliner at the same venue on the same date. Returns the matching show or nil.
func (s *DiscoveryService) checkHeadlinerDuplicate(headlinerName, venueName string, eventDate time.Time) *models.Show {
	startOfDay := time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var existingShow models.Show
	err := s.db.
		Joins("JOIN show_artists ON shows.id = show_artists.show_id").
		Joins("JOIN artists ON show_artists.artist_id = artists.id").
		Joins("JOIN show_venues ON shows.id = show_venues.show_id").
		Joins("JOIN venues ON show_venues.venue_id = venues.id").
		Where("LOWER(artists.name) = LOWER(?) AND show_artists.set_type = ?", headlinerName, "headliner").
		Where("LOWER(venues.name) = LOWER(?)", venueName).
		Where("shows.event_date >= ? AND shows.event_date < ?", startOfDay, endOfDay).
		Where("shows.status NOT IN ?", []models.ShowStatus{models.ShowStatusRejected, models.ShowStatusPrivate}).
		First(&existingShow).Error
	if err != nil {
		return nil
	}
	return &existingShow
}

// importEvent imports a single scraped event
// Returns a message and status ("imported", "duplicate", "rejected", "error")
func (s *DiscoveryService) importEvent(event *DiscoveredEvent, dryRun bool) (string, string) {
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

	// Look up venue configuration (needed for timezone before parsing date)
	venueConfig, ok := VenueConfig[event.VenueSlug]
	if !ok {
		return fmt.Sprintf("ERROR: Unknown venue slug: %s", event.VenueSlug), "error"
	}

	// Parse event date using the venue's state for timezone context
	eventDate, err := parseEventDate(event.Date, event.ShowTime, venueConfig.State)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to parse date for %s: %v", event.Title, err), "error"
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

	// Check for potential duplicate (same headliner + venue + date as an existing show)
	var duplicateOfShowID *uint
	if len(event.Artists) > 0 {
		if dupShow := s.checkHeadlinerDuplicate(event.Artists[0], venueConfig.Name, eventDate); dupShow != nil {
			duplicateOfShowID = &dupShow.ID
			if dryRun {
				return fmt.Sprintf("WOULD FLAG FOR REVIEW: %s at %s on %s (potential duplicate of show #%d: %s)",
					event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04"), dupShow.ID, dupShow.Title), "pending_review"
			}
		}
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: %s at %s on %s", event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04")), "imported"
	}

	// Create the show
	err = s.createShowFromEvent(event, eventDate, venueConfig, duplicateOfShowID)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to create show: %v", err), "error"
	}

	if duplicateOfShowID != nil {
		return fmt.Sprintf("FLAGGED FOR REVIEW: %s at %s on %s", event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04")), "pending_review"
	}

	return fmt.Sprintf("IMPORTED: %s at %s on %s", event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04")), "imported"
}

// createShowFromEvent creates a show record from a scraped event
func (s *DiscoveryService) createShowFromEvent(event *DiscoveredEvent, eventDate time.Time, venueConfig struct {
	Name    string
	City    string
	State   string
	Address string
}, duplicateOfShowID *uint) error {
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
		status := models.ShowStatusApproved
		if duplicateOfShowID != nil {
			status = models.ShowStatusPending
		}

		show := &models.Show{
			Title:             event.Title,
			EventDate:         eventDate.UTC(),
			City:              &venueConfig.City,
			State:             &venueConfig.State,
			Description:       description,
			Status:            status,
			Source:            models.ShowSourceDiscovery,
			SourceVenue:       &event.VenueSlug,
			SourceEventID:     &event.ID,
			ScrapedAt:         &scrapedAt,
			DuplicateOfShowID: duplicateOfShowID,
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

		// Use artists array from discovery app if available, otherwise parse from title
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
				// Generate slug for the new artist
				baseSlug := utils.GenerateArtistSlug(artist.Name)
				artistSlug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
					var count int64
					tx.Model(&models.Artist{}).Where("slug = ?", candidate).Count(&count)
					return count > 0
				})
				tx.Model(&artist).Update("slug", artistSlug)
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

		// Generate slug for the show
		headlinerName := ""
		if len(artistNames) > 0 {
			headlinerName = artistNames[0]
		}
		baseShowSlug := utils.GenerateShowSlug(show.EventDate, headlinerName, venueConfig.Name)
		showSlug := utils.GenerateUniqueSlug(baseShowSlug, func(candidate string) bool {
			var count int64
			tx.Model(&models.Show{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})
		tx.Model(show).Update("slug", showSlug)

		return nil
	})
}

// stateTimezones maps US state abbreviations to IANA timezone names
var stateTimezones = map[string]string{
	"AZ": "America/Phoenix",
	"CA": "America/Los_Angeles",
	"NV": "America/Los_Angeles",
	"CO": "America/Denver",
	"NM": "America/Denver",
	"TX": "America/Chicago",
	"NY": "America/New_York",
}

// getTimezoneForState returns the IANA timezone for a US state abbreviation.
// Defaults to "America/Phoenix" if the state is not found.
func getTimezoneForState(state string) string {
	if tz, ok := stateTimezones[strings.ToUpper(state)]; ok {
		return tz
	}
	return "America/Phoenix"
}

// parseEventDate parses the event date and optional show time into a time.Time (UTC).
// The state parameter is used to interpret the show time in the venue's local timezone.
func parseEventDate(dateStr string, showTime *string, state string) (time.Time, error) {
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
			// Interpret the time in the venue's local timezone
			loc, locErr := time.LoadLocation(getTimezoneForState(state))
			if locErr != nil {
				loc = time.UTC
			}
			date = time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, loc)
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
func (s *DiscoveryService) ImportFromJSONWithDB(filepath string, dryRun bool, database *gorm.DB) (*ImportResult, error) {
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

// CheckEventInput represents a single event to check for import status
type CheckEventInput struct {
	ID        string `json:"id"`
	VenueSlug string `json:"venueSlug"`
}

// CheckEventStatus represents the import status of a single event
type CheckEventStatus struct {
	Exists bool   `json:"exists"`
	ShowID uint   `json:"showId,omitempty"`
	Status string `json:"status,omitempty"`
}

// CheckEventsResult contains the import status of multiple events
type CheckEventsResult struct {
	Events map[string]CheckEventStatus `json:"events"`
}

// CheckEvents checks whether scraped events already exist in the database
func (s *DiscoveryService) CheckEvents(events []CheckEventInput) (*CheckEventsResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &CheckEventsResult{
		Events: make(map[string]CheckEventStatus),
	}

	if len(events) == 0 {
		return result, nil
	}

	// Build WHERE clause for batch lookup
	// WHERE (source_venue, source_event_id) IN (('valley-bar','evt1'), ('crescent-ballroom','evt2'))
	pairs := make([][]interface{}, 0, len(events))
	for _, e := range events {
		if e.ID != "" && e.VenueSlug != "" {
			pairs = append(pairs, []interface{}{e.VenueSlug, e.ID})
		}
	}

	if len(pairs) == 0 {
		return result, nil
	}

	var shows []models.Show
	err := s.db.Where("(source_venue, source_event_id) IN ?", pairs).
		Select("id, source_venue, source_event_id, status").
		Find(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to check events: %w", err)
	}

	for _, show := range shows {
		if show.SourceEventID != nil {
			result.Events[*show.SourceEventID] = CheckEventStatus{
				Exists: true,
				ShowID: show.ID,
				Status: string(show.Status),
			}
		}
	}

	return result, nil
}

// ImportEvents imports events from an array of DiscoveredEvent objects
// This is used by the HTTP API endpoint for importing scraped data directly
func (s *DiscoveryService) ImportEvents(events []DiscoveredEvent, dryRun bool) (*ImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
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
		case "pending_review":
			result.PendingReview++
		case "error":
			result.Errors++
		}
	}

	return result, nil
}
