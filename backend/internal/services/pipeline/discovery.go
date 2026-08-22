package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// enrichmentQueuer is the subset of EnrichmentService used by DiscoveryService.
type enrichmentQueuer interface {
	QueueShowForEnrichment(showID uint, enrichmentType string) error
}

// DiscoveryService handles importing discovered event data into the database
type DiscoveryService struct {
	db                *gorm.DB
	venueService      venueFinderCreator
	enrichmentService enrichmentQueuer
}

// venueFinderCreator is the subset of VenueService used by DiscoveryService.
type venueFinderCreator interface {
	FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*catalogm.Venue, bool, error)
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService(database *gorm.DB, venueSvc venueFinderCreator) *DiscoveryService {
	if database == nil {
		database = db.GetDB()
	}
	return &DiscoveryService{
		db:           database,
		venueService: venueSvc,
	}
}

// SetEnrichmentService sets the enrichment service for post-import queuing.
// Called after container construction to avoid circular dependencies.
func (s *DiscoveryService) SetEnrichmentService(enrichment enrichmentQueuer) {
	s.enrichmentService = enrichment
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
	// Phoenix, AZ - SeeTickets venues
	"the-rebel-lounge": {
		Name:    "The Rebel Lounge",
		City:    "Phoenix",
		State:   "AZ",
		Address: "2303 E Indian School Rd",
	},

	// Chicago, IL
	"empty-bottle": {
		Name:    "Empty Bottle",
		City:    "Chicago",
		State:   "IL",
		Address: "1035 N Western Ave",
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
func (s *DiscoveryService) ImportFromJSON(filepath string, dryRun bool) (*contracts.ImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Read the JSON file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON - could be a single venue's events or multiple venues
	var events []contracts.DiscoveredEvent

	// Try parsing as array first
	if err := json.Unmarshal(data, &events); err != nil {
		// Try parsing as object with venue keys
		var venueEvents map[string][]contracts.DiscoveredEvent
		if err := json.Unmarshal(data, &venueEvents); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		// Flatten into single array
		for _, ve := range venueEvents {
			events = append(events, ve...)
		}
	}

	result := &contracts.ImportResult{
		Total:    len(events),
		Messages: make([]string, 0),
	}

	for _, event := range events {
		msg, status := s.importEvent(&event, dryRun, false, catalogm.ShowStatusApproved)
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
		case "updated":
			result.Updated++
		case "error":
			result.Errors++
		}
	}

	return result, nil
}

// checkHeadlinerDuplicate checks if there's an existing non-rejected show with the same
// headliner at the same venue at the same exact event_date. Returns the matching show or nil.
// The dedup key is the FULL event_date timestamp (PSY-559) — matinee and evening sets at
// the same venue are distinct shows, not duplicates.
func (s *DiscoveryService) checkHeadlinerDuplicate(headlinerName, venueName string, eventDate time.Time) *catalogm.Show {
	var existingShow catalogm.Show
	err := s.db.
		Joins("JOIN show_artists ON shows.id = show_artists.show_id").
		Joins("JOIN artists ON show_artists.artist_id = artists.id").
		Joins("JOIN show_venues ON shows.id = show_venues.show_id").
		Joins("JOIN venues ON show_venues.venue_id = venues.id").
		Where("LOWER(artists.name) = LOWER(?)", headlinerName).
		Where("(show_artists.set_type = ? OR show_artists.position = 0)", "headliner").
		Where("LOWER(venues.name) = LOWER(?)", venueName).
		Where("shows.event_date = ?", eventDate).
		Where("shows.status NOT IN ?", []catalogm.ShowStatus{catalogm.ShowStatusRejected, catalogm.ShowStatusPrivate}).
		First(&existingShow).Error
	if err != nil {
		return nil
	}
	return &existingShow
}

// resolveHeadlinerName determines the headliner artist name for duplicate checking.
// Prefers BillingArtists data (with explicit set_type/billing_order) over the plain Artists list.
// Falls back to the first artist in the list, which the import logic treats as headliner.
func (s *DiscoveryService) resolveHeadlinerName(event *contracts.DiscoveredEvent) string {
	// Check BillingArtists first — these have explicit set_type from AI extraction
	if len(event.BillingArtists) > 0 {
		// Look for an explicit headliner
		for _, ba := range event.BillingArtists {
			if contracts.NormalizeSetType(ba.SetType) == contracts.SetTypeHeadliner {
				return ba.Name
			}
		}
		// If no explicit headliner, use billing_order=1 or first entry
		lowest := event.BillingArtists[0]
		for _, ba := range event.BillingArtists[1:] {
			if ba.BillingOrder > 0 && (lowest.BillingOrder == 0 || ba.BillingOrder < lowest.BillingOrder) {
				lowest = ba
			}
		}
		return lowest.Name
	}

	// Fall back to plain Artists list — first entry is treated as headliner during import
	if len(event.Artists) > 0 {
		return event.Artists[0]
	}

	// No artist info — can't check for headliner duplicates
	return ""
}

// importEvent imports a single scraped event
// Returns a message, status ("imported", "duplicate", "rejected", "updated", "error"), and show ID (if created)
func (s *DiscoveryService) importEvent(event *contracts.DiscoveredEvent, dryRun bool, allowUpdates bool, initialStatus catalogm.ShowStatus) (string, string) {
	// Validate required fields
	if event.ID == "" || event.VenueSlug == "" {
		return fmt.Sprintf("SKIP: Missing required fields (id=%s, venueSlug=%s)", event.ID, event.VenueSlug), "error"
	}

	// Check for duplicate (same source_venue + source_event_id)
	var existing catalogm.Show
	err := s.db.Where("source_venue = ? AND source_event_id = ?", event.VenueSlug, event.ID).First(&existing).Error
	if err == nil {
		if allowUpdates {
			return s.updateShowFromEvent(&existing, event, dryRun)
		}
		return fmt.Sprintf("DUPLICATE: %s (ID: %s) already imported as show #%d", event.Title, event.ID, existing.ID), "duplicate"
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
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

	// Check if there's a rejected show at the same venue at the same exact
	// event_date. This prevents re-importing events that were previously
	// rejected. Keyed on the FULL event_date timestamp (PSY-559) so a rejected
	// matinee does not block a legitimate evening import at the same venue.
	var rejectedShow catalogm.Show
	err = s.db.Joins("JOIN show_venues ON shows.id = show_venues.show_id").
		Joins("JOIN venues ON show_venues.venue_id = venues.id").
		Where("LOWER(venues.name) = LOWER(?) AND shows.event_date = ? AND shows.status = ?",
			venueConfig.Name, eventDate, catalogm.ShowStatusRejected).
		First(&rejectedShow).Error
	if err == nil {
		return fmt.Sprintf("REJECTED: %s matches previously rejected show #%d at %s on %s",
			event.Title, rejectedShow.ID, venueConfig.Name, eventDate.Format("2006-01-02 15:04")), "rejected"
	}

	// Check for duplicate: same headliner + venue + date as an existing show.
	// Determine the headliner name from billing data (preferred) or artist list.
	headlinerName := s.resolveHeadlinerName(event)
	if headlinerName != "" {
		if dupShow := s.checkHeadlinerDuplicate(headlinerName, venueConfig.Name, eventDate); dupShow != nil {
			return fmt.Sprintf("DUPLICATE: %s at %s on %s (matches existing show #%d: %s)",
				event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04"), dupShow.ID, dupShow.Title), "duplicate"
		}
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: %s at %s on %s", event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04")), "imported"
	}

	// Create the show
	err = s.createShowFromEvent(event, eventDate, venueConfig, nil, initialStatus)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to create show: %v", err), "error"
	}

	return fmt.Sprintf("IMPORTED: %s at %s on %s", event.Title, venueConfig.Name, eventDate.Format("2006-01-02 15:04")), "imported"
}

// createShowFromEvent creates a show record from a scraped event
func (s *DiscoveryService) createShowFromEvent(event *contracts.DiscoveredEvent, eventDate time.Time, venueConfig struct {
	Name    string
	City    string
	State   string
	Address string
}, duplicateOfShowID *uint, initialStatus catalogm.ShowStatus) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Parse scraped_at timestamp
		scrapedAt, err := time.Parse(time.RFC3339, event.ScrapedAt)
		if err != nil {
			scrapedAt = time.Now().UTC()
		}

		// Resolve the doors/music instants the source stated. Nil means the
		// source said nothing readable, and the column stays empty.
		doorsAt, musicAt := resolveShowTimes(event.Date, event.DoorsTime, event.ShowTime, venueConfig.State)

		// Build description from available info.
		//
		// A time that reached its own column is NOT repeated here: the status
		// stripe renders doors_at/music_at, so keeping the string as well shows
		// the same fact twice. A time the parser could not read still goes in,
		// because then the string is the only record that survives the import.
		var descParts []string
		if doorsAt == nil && event.DoorsTime != nil && *event.DoorsTime != "" {
			descParts = append(descParts, fmt.Sprintf("Doors: %s", *event.DoorsTime))
		}
		if musicAt == nil && event.ShowTime != nil && *event.ShowTime != "" {
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

		// Create the show — use initialStatus, but always override to pending for potential duplicates
		status := initialStatus
		if duplicateOfShowID != nil {
			status = catalogm.ShowStatusPending
		}

		// Set data provenance fields for AI-extracted shows
		aiSource := catalogm.DataSourceAIExtraction
		aiConfidence := 0.8
		now := time.Now()

		show := &catalogm.Show{
			Title:             event.Title,
			EventDate:         eventDate.UTC(),
			DoorsAt:           doorsAt,
			MusicAt:           musicAt,
			City:              &venueConfig.City,
			State:             &venueConfig.State,
			Description:       description,
			Status:            status,
			Source:            catalogm.ShowSourceDiscovery,
			SourceVenue:       &event.VenueSlug,
			SourceEventID:     &event.ID,
			ScrapedAt:         &scrapedAt,
			DataSource:        &aiSource,
			SourceConfidence:  &aiConfidence,
			LastVerifiedAt:    &now,
			DuplicateOfShowID: duplicateOfShowID,
			Price:             parsePriceString(ptrStr(event.Price)),
			AgeRequirement:    event.AgeRestriction,
		}

		if event.IsSoldOut != nil && *event.IsSoldOut {
			show.IsSoldOut = true
		}
		// Recover the flag when the venue encoded it in the title instead of a
		// field ("*SOLD OUT* Audrey Hobert"). Without this the marker is
		// stripped off the artist name below and the signal is lost entirely.
		if _, titleSoldOut := stripStatusMarkers(event.Title); titleSoldOut {
			show.IsSoldOut = true
		}
		if event.IsCancelled != nil && *event.IsCancelled {
			show.IsCancelled = true
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
		showVenue := catalogm.ShowVenue{
			ShowID:  show.ID,
			VenueID: venue.ID,
		}
		if err := tx.Create(&showVenue).Error; err != nil {
			return fmt.Errorf("failed to create show-venue association: %w", err)
		}

		// Build billing-aware artist list
		type artistEntry struct {
			Name         string
			SetType      string
			BillingOrder int
		}
		var artistEntries []artistEntry

		if len(event.BillingArtists) > 0 {
			// Use richer billing data from AI extraction
			for _, ba := range event.BillingArtists {
				artistEntries = append(artistEntries, artistEntry{
					Name:         ba.Name,
					SetType:      ba.SetType,
					BillingOrder: ba.BillingOrder,
				})
			}
		} else if len(event.Artists) > 0 {
			// Fall back to simple artist list
			for _, name := range event.Artists {
				artistEntries = append(artistEntries, artistEntry{Name: name})
			}
		} else {
			// Fall back to parsing from title
			for _, name := range parseArtistsFromTitle(event.Title) {
				artistEntries = append(artistEntries, artistEntry{Name: name})
			}
		}

		for idx, entry := range artistEntries {
			// Sanitize at the boundary — this covers all three sources above
			// (billing data, artist list, title fallback) with one rule, so a
			// status marker can't reach the catalog through whichever path the
			// venue's feed happens to use.
			artistName, _ := stripStatusMarkers(entry.Name)
			if artistName == "" {
				continue
			}

			// Single artist write path (PSY-1254): dedup + unique slug + insert.
			foundArtist, _, err := catalog.FindOrCreateArtistTx(tx, artistName, nil)
			if err != nil {
				return fmt.Errorf("artist %s: %w", artistName, err)
			}
			artist := *foundArtist

			// Determine position: use billing_order if provided, otherwise array index
			position := idx
			if entry.BillingOrder > 0 {
				position = entry.BillingOrder - 1 // billing_order is 1-based, position is 0-based
			}

			// Determine set type from AI extraction, with fallback logic.
			//
			// The fallback infers the HEADLINER only from a first position the
			// source said NOTHING about. Two distinct silences have to be told
			// apart here:
			//
			//   - the source stated no slot at all -> position 0 is the usual
			//     billing convention, and headliner is a fair reading
			//   - the source stated a slot the vocabulary cannot model (a
			//     "host", say) -> it already told us this act is not the
			//     headliner, so inferring one from position would assert
			//     something the source contradicts
			//
			// Everything else the source did not state resolves to the neutral
			// default: a scrape listing four names in order is evidence of
			// billing order, not evidence that acts two through four opened,
			// and stamping a role on that inference is what made this column
			// unreadable before PSY-1673.
			setType := contracts.NormalizeSetType(entry.SetType)
			if setType == "" {
				statedSomeSlot := strings.TrimSpace(entry.SetType) != ""
				if idx == 0 && !statedSomeSlot {
					setType = contracts.SetTypeHeadliner
				} else {
					setType = contracts.SetTypeDefault
				}
			}

			// Create show-artist association. EventDate + VenueID
			// denormalize the show dedup key so the partial unique index
			// `shows_artist_venue_eventdate_uniq` covers discovery-imported
			// rows (PSY-576). Discovery imports a single venue per scrape
			// (see venueConfig above), so primary-venue resolution is
			// unambiguous.
			discoveryEventDate := show.EventDate
			discoveryVenueID := venue.ID
			showArtist := catalogm.ShowArtist{
				ShowID:    show.ID,
				ArtistID:  artist.ID,
				Position:  position,
				SetType:   setType,
				EventDate: &discoveryEventDate,
				VenueID:   &discoveryVenueID,
			}
			if err := tx.Create(&showArtist).Error; err != nil {
				return fmt.Errorf("failed to create show-artist association: %w", err)
			}
		}

		// Generate slug for the show
		headlinerName := ""
		if len(artistEntries) > 0 {
			headlinerName = artistEntries[0].Name
		}
		baseShowSlug := utils.GenerateShowSlug(show.EventDate, headlinerName, venueConfig.Name, venueConfig.State)
		showSlug := utils.GenerateUniqueSlug(baseShowSlug, func(candidate string) bool {
			var count int64
			tx.Model(&catalogm.Show{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})
		tx.Model(show).Update("slug", showSlug)

		// Enqueue the follower-notification outbox job (PSY-1894). Discovery is the
		// highest-volume ingest path and, until now, notified nobody: shows land
		// here already `approved` and never enter the admin moderation queue that
		// owns the only MatchAndNotify call sites.
		//
		// The whole `show` is passed rather than a hardcoded "approved", because
		// catalogm.ShowAnnounceable — the one predicate the poller re-runs at
		// delivery — reads more than the status. It matters most here: this function
		// sets IsCancelled straight from the scrape (both the feed's flag and a
		// "*CANCELLED*" title marker) and still writes the row `approved`, so
		// without that predicate an automated calendar import would email "New show"
		// for an event the venue has already called off.
		//
		// Reading show.Status rather than assuming it also keeps the duplicate
		// branch above honest. That branch (`status = pending` when
		// duplicateOfShowID is set) is unreachable from the only caller today, which
		// passes nil — real duplicates are rejected before a show is created at all.
		// Should it be revived, the enqueue already does the right thing instead of
		// announcing a flagged duplicate.
		catalog.EnqueueShowNotify(tx, show)

		return nil
	})
}

// updateShowFromEvent compares scraped event data against an existing show and updates changed fields.
// Returns a message and status ("updated" or "duplicate").
func (s *DiscoveryService) updateShowFromEvent(existing *catalogm.Show, event *contracts.DiscoveredEvent, dryRun bool) (string, string) {
	updates := make(map[string]interface{})
	var changes []string

	// Compare price
	if event.Price != nil {
		newPrice := parsePriceString(*event.Price)
		if newPrice != nil {
			if existing.Price == nil || *existing.Price != *newPrice {
				updates["price"] = *newPrice
				oldStr := "nil"
				if existing.Price != nil {
					oldStr = fmt.Sprintf("$%.2f", *existing.Price)
				}
				changes = append(changes, fmt.Sprintf("price: %s -> $%.2f", oldStr, *newPrice))
			}
		}
	}

	// Compare age restriction
	if event.AgeRestriction != nil {
		if existing.AgeRequirement == nil || *existing.AgeRequirement != *event.AgeRestriction {
			updates["age_requirement"] = *event.AgeRestriction
			oldStr := "nil"
			if existing.AgeRequirement != nil {
				oldStr = *existing.AgeRequirement
			}
			changes = append(changes, fmt.Sprintf("age: %s -> %s", oldStr, *event.AgeRestriction))
		}
	}

	// Compare sold out status
	if event.IsSoldOut != nil && *event.IsSoldOut != existing.IsSoldOut {
		updates["is_sold_out"] = *event.IsSoldOut
		changes = append(changes, fmt.Sprintf("soldOut: %v -> %v", existing.IsSoldOut, *event.IsSoldOut))
	}

	// Compare cancelled status
	if event.IsCancelled != nil && *event.IsCancelled != existing.IsCancelled {
		updates["is_cancelled"] = *event.IsCancelled
		changes = append(changes, fmt.Sprintf("cancelled: %v -> %v", existing.IsCancelled, *event.IsCancelled))
	}

	// doors_at / music_at are deliberately NOT updated here.
	//
	// This path never moves event_date, and music_at has to keep naming the
	// same instant. Writing a time onto a row whose event_date stays put
	// produces a show whose stripe and date disagree -- by 27 hours when the
	// original scrape stated no time at all, since event_date is then a
	// midnight-UTC placeholder. It also breaks the doors <= music invariant the
	// API enforces over stored-plus-incoming values (validateShowTimeOrder in
	// internal/api/handlers/catalog/show.go), because a scrape stating only one
	// of the two cannot see the stored other half: a later scrape stating
	// "12:00 AM" music against a stored 11:00 PM doors would write a row that
	// 422s the next admin edit.
	//
	// Filling times on already-imported shows is a backfill, and a backfill has
	// to reconcile event_date at the same time. Until something does, times
	// land on the create path only.

	if len(updates) == 0 {
		return fmt.Sprintf("DUPLICATE: %s (ID: %s) already imported as show #%d (no changes)", event.Title, event.ID, existing.ID), "duplicate"
	}

	changeStr := strings.Join(changes, ", ")

	if dryRun {
		return fmt.Sprintf("WOULD UPDATE: %s (show #%d) -- %s", event.Title, existing.ID, changeStr), "updated"
	}

	if err := s.db.Model(existing).Updates(updates).Error; err != nil {
		return fmt.Sprintf("ERROR: Failed to update show #%d: %v", existing.ID, err), "error"
	}

	return fmt.Sprintf("UPDATED: %s (show #%d) -- %s", event.Title, existing.ID, changeStr), "updated"
}

// getTimezoneForState delegates to the shared utils.GetTimezoneForState.
func getTimezoneForState(state string) string {
	return utils.GetTimezoneForState(state)
}

// parseCalendarDate reads the calendar date out of the date string a scrape
// reports. The returned time carries whatever zone the input expressed, so
// callers that want the stated Y/M/D read it off directly.
//
// dateOnly reports that the input was a bare YYYY-MM-DD, i.e. that it stated no
// time of day at all. That is NOT the same as "the time is midnight", and the
// difference is the whole of PSY-1861: a bare date has to be widened onto a real
// instant by the writer, whereas a full timestamp already named one and must be
// stored verbatim however unusual it looks. Reported here because this is the
// only point at which the original representation still exists.
func parseCalendarDate(dateStr string) (date time.Time, dateOnly bool, err error) {
	const dateOnlyLayout = "2006-01-02"
	for _, layout := range []string{dateOnlyLayout, "2006-01-02T15:04:05Z", time.RFC3339} {
		if parsed, perr := time.Parse(layout, dateStr); perr == nil {
			return parsed, layout == dateOnlyLayout, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("unable to parse date: %s", dateStr)
}

// clockTimeWithMeridiem matches the 12-hour wall clock every scraper in
// discovery/src/server/providers emits ("7:00 pm", "6:30PM"). The trailing
// periods cover the "7:00 p.m." a raw passthrough can leak.
var clockTimeWithMeridiem = regexp.MustCompile(`^(\d{1,2}):(\d{2})([ap])\.?m\.?$`)

// clockTime24Hour matches the 24-hour wall clock a feed can hand through
// unformatted ("19:00"). Only hours that CANNOT be a 12-hour reading count:
// "19:00" is unambiguous, "7:00" is not. emptybottle.formatTime returns its
// input unchanged when its AM/PM regex misses, so a ".start-time" cell reading
// "7:00" arrives here verbatim from a 7 PM show -- reading it as 7 AM would
// fabricate exactly the kind of time this parser exists to refuse.
var clockTime24Hour = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)

// parseClockTime reads a wall-clock time of day out of the free text a venue
// calendar states. It reports ok only when the string is unambiguously a time,
// so a caller never has to decide what a half-readable value meant.
//
// Deliberately strict. "doors at 7" and "7ish" are not times. Neither is
// "19:00 pm", a 24-hour clock wearing a meridiem, which the previous Sscanf
// parser silently turned into hour 31 and rolled into the next day. Neither is
// a bare "7:00", which could name either half of the day. Rejecting them leaves
// the caller with "the source did not state a time", which is a truthful
// answer; accepting them would publish a fabricated one.
func parseClockTime(raw string) (hour, minute int, ok bool) {
	// Fold case and drop ALL whitespace so "7:00 PM" and "7:00pm" are one
	// shape. Unicode-aware on purpose: venue calendars are scraped HTML, and a
	// "7:00&nbsp;PM" reaches ticketweb.parseTime, whose capture group hands the
	// U+00A0 through verbatim. Stripping only the ASCII space would drop that
	// listing's times on the floor.
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, raw)

	if m := clockTimeWithMeridiem.FindStringSubmatch(normalized); m != nil {
		hour, _ = strconv.Atoi(m[1])
		minute, _ = strconv.Atoi(m[2])
		if hour < 1 || hour > 12 || minute > 59 {
			return 0, 0, false
		}
		switch {
		case m[3] == "p" && hour != 12:
			hour += 12
		case m[3] == "a" && hour == 12:
			hour = 0
		}
		return hour, minute, true
	}

	if m := clockTime24Hour.FindStringSubmatch(normalized); m != nil {
		hour, _ = strconv.Atoi(m[1])
		minute, _ = strconv.Atoi(m[2])
		// Hours 1 through 12 without a meridiem could mean either half of the
		// day, so they are not a stated time. Hour 0 and 13 through 23 can only
		// be a 24-hour clock.
		if hour > 23 || minute > 59 || (hour >= 1 && hour <= 12) {
			return 0, 0, false
		}
		return hour, minute, true
	}

	return 0, 0, false
}

// venueLocalInstant anchors a wall-clock time to a stated calendar date, read
// in the venue's local zone, and returns the UTC instant it names.
func venueLocalInstant(date time.Time, hour, minute int, state string) time.Time {
	loc, err := time.LoadLocation(getTimezoneForState(state))
	if err != nil {
		loc = time.UTC
	}
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, loc).UTC()
}

// parseEventDate parses the event date and optional show time into a time.Time (UTC).
// The state parameter is used to interpret the time of day in the venue's local
// timezone — for a stated show time, and for the date-only anchor below.
func parseEventDate(dateStr string, showTime *string, state string) (time.Time, error) {
	date, dateOnly, err := parseCalendarDate(dateStr)
	if err != nil {
		return time.Time{}, err
	}

	if showTime != nil {
		if hour, minute, ok := parseClockTime(*showTime); ok {
			return venueLocalInstant(date, hour, minute, state), nil
		}
	}

	// No readable time of day. A full timestamp already stated its own, so only
	// a date-only input needs anchoring — and it must be anchored rather than
	// left on the bare calendar date, because that bare date becomes UTC
	// midnight, which is the PREVIOUS evening in every US zone. Stored that way
	// the show rendered and sorted a day early and dropped out of every "still
	// upcoming" bound from 17:00 the day before in Phoenix. See
	// utils.DateOnlyEventHour.
	//
	// A show time that was PRESENT but unreadable lands here too, and takes the
	// same anchor. That is deliberate: an unreadable time means the scrape did
	// not state one this parser trusts, which is the same state as not stating
	// one at all. Anchoring is wrong by hours; the bare date was wrong by a day.
	if !dateOnly {
		return date.UTC(), nil
	}
	return venueLocalInstant(date, utils.DateOnlyEventHour, 0, state), nil
}

// resolveShowTimes maps a scraped event's free-text doors/show times onto the
// shows.doors_at / shows.music_at instants.
//
// Explicit only. A time the source did not state, or stated in a form
// parseClockTime cannot read unambiguously, comes back nil so the column stays
// empty. The show page would rather render a date alone than a door time the
// pipeline invented.
//
// Both instants anchor to the calendar date the source stated, read in the
// venue's timezone -- the same anchoring parseEventDate uses.
//
// A readable SHOW time is required before either column is written. The reason
// is no longer that event_date would otherwise be a midnight-UTC placeholder a
// calendar day away from the stripe — PSY-1861 removed that placeholder, and a
// timeless import now anchors on the stated day's evening like everything else.
// What remains is narrower and still holds: doors_at and music_at are a PAIR
// describing one evening's schedule, and this function's whole contract is that
// it never invents a time. With no readable show time there is nothing to
// order a doors time against, and a lone doors_at would publish half a schedule
// as if it were the whole one.
//
// A stated pair whose show time lands before its doors time is contradictory,
// and the usual cause is a listing that crossed midnight and dropped the day
// ("Doors 11:00 PM / Show 12:00 AM"). Recovering that intent would mean
// inferring a day rollover the source never stated, so neither time is written.
func resolveShowTimes(dateStr string, doorsTime, showTime *string, state string) (doorsAt, musicAt *time.Time) {
	date, _, err := parseCalendarDate(dateStr)
	if err != nil {
		return nil, nil
	}

	if showTime == nil {
		return nil, nil
	}
	showHour, showMinute, ok := parseClockTime(*showTime)
	if !ok {
		return nil, nil
	}
	music := venueLocalInstant(date, showHour, showMinute, state)
	musicAt = &music

	if doorsTime != nil {
		if hour, minute, ok := parseClockTime(*doorsTime); ok {
			instant := venueLocalInstant(date, hour, minute, state)
			doorsAt = &instant
		}
	}

	if doorsAt != nil && musicAt.Before(*doorsAt) {
		return nil, nil
	}

	return doorsAt, musicAt
}

// parseArtistsFromTitle extracts artist names from event title
// eventStatusMarkers are listing-status prefixes venues put on a calendar entry.
// They describe the EVENT, not a band, and the show model already has columns
// for them — so they must never survive into an artist name.
//
// Sold-out markers are listed first and reported back to the caller so the
// signal is preserved on `shows.is_sold_out` rather than thrown away.
var eventSoldOutMarkers = []string{"*sold out*", "sold out", "*soldout*", "soldout"}

var eventOtherMarkers = []string{
	"*cancelled*", "*canceled*", "cancelled:", "canceled:",
	"*postponed*", "postponed:",
	"*free*", "*free show*", "*21+*", "*18+*", "*all ages*",
}

// stripStatusMarkers removes listing-status decoration from a name and reports
// whether a sold-out marker was present.
//
// Without this, `parseArtistsFromTitle`'s final fallback ("no separator found,
// treat the entire title as one artist") mints artist records like
// `*SOLD OUT* Audrey Hobert`. That is not just cosmetic: show dedup keys on
// (artist_id, venue_id, event_date), so a decorated name produces a DIFFERENT
// artist_id and the same event gets stored twice. Verified on production —
// Thalia Hall 2026-07-29 carried both the real Red Vox show and a duplicate
// under `*SOLD OUT* Red Vox ft. special guests Super Guitar Bros.`
func stripStatusMarkers(name string) (string, bool) {
	cleaned := strings.TrimSpace(name)
	soldOut := false

	// Loop: a listing can stack markers, e.g. "*SOLD OUT* *21+* Band".
	for changed := true; changed; {
		changed = false
		lower := strings.ToLower(cleaned)
		for _, m := range eventSoldOutMarkers {
			if strings.HasPrefix(lower, m) {
				cleaned = strings.TrimSpace(cleaned[len(m):])
				soldOut, changed = true, true
				break
			}
		}
		if changed {
			continue
		}
		for _, m := range eventOtherMarkers {
			if strings.HasPrefix(lower, m) {
				cleaned = strings.TrimSpace(cleaned[len(m):])
				changed = true
				break
			}
		}
	}

	// Leading punctuation left behind by a stripped marker (":", "-", "–").
	cleaned = strings.TrimSpace(strings.TrimLeft(cleaned, ":-–—|"))

	// Never return empty — a name that was ONLY a marker is better kept intact
	// than silently dropped, so a human can see and fix it.
	if cleaned == "" {
		return strings.TrimSpace(name), soldOut
	}
	return cleaned, soldOut
}

func parseArtistsFromTitle(title string) []string {
	// Featuring / support separators are checked BEFORE comma, because they
	// bind more strongly: in "Headliner featuring A, B" the comma separates the
	// SUPPORT acts, so splitting on it first yields "Headliner featuring A" —
	// another billing string masquerading as a band name, which is the exact
	// defect this function is being hardened against.
	//
	// The extraction prompt already classifies ft./feat./w/ as billing markers
	// ("special_guest", "support"); the title fallback just wasn't splitting on
	// them.
	lower := strings.ToLower(title)
	for _, sep := range []string{" ft. ", " feat. ", " featuring ", " w/ "} {
		if idx := strings.Index(lower, sep); idx > 0 {
			head := strings.TrimSpace(title[:idx])
			rest := strings.TrimSpace(title[idx+len(sep):])
			// Drop the "special guests" connective the venue wrote for humans.
			for _, filler := range []string{"special guests ", "special guest "} {
				if strings.HasPrefix(strings.ToLower(rest), filler) {
					rest = strings.TrimSpace(rest[len(filler):])
					break
				}
			}
			artists := []string{head}
			artists = append(artists, splitAndTrim(rest, ",")...)
			return artists
		}
	}

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

// ptrStr safely dereferences a string pointer, returning "" if nil
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parsePriceString parses a price string like "$18", "$23.81", "Free" into a float64
func parsePriceString(s string) *float64 {
	if s == "" {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "free" || lower == "no cover" {
		val := 0.0
		return &val
	}
	// Strip "$" prefix
	cleaned := strings.TrimPrefix(lower, "$")
	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return nil
	}
	return &val
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
func (s *DiscoveryService) ImportFromJSONWithDB(filepath string, dryRun bool, database *gorm.DB) (*contracts.ImportResult, error) {
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
// CheckEvents checks whether scraped events already exist in the database
func (s *DiscoveryService) CheckEvents(events []contracts.CheckEventInput) (*contracts.CheckEventsResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &contracts.CheckEventsResult{
		Events: make(map[string]contracts.CheckEventStatus),
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

	var shows []catalogm.Show
	err := s.db.Where("(source_venue, source_event_id) IN ?", pairs).
		Select("id, source_venue, source_event_id, status, price, age_requirement, description, event_date, is_sold_out, is_cancelled").
		Find(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to check events: %w", err)
	}

	// Index the source-key matches by their event ID for O(1) "already matched?"
	// lookups in the fallback loop below.
	matchedByEventID := make(map[string]bool, len(shows))
	for _, show := range shows {
		if show.SourceEventID != nil {
			matchedByEventID[*show.SourceEventID] = true
		}
	}

	// Fallback: for events not found by source key, try venue + date match.
	// This catches manually-created shows that don't have source_venue/source_event_id.
	// Collect the matched fallback shows here (keyed by the input event ID) so
	// their artist names can be batched into the single fetch below alongside
	// the source-key matches — keeping CheckEvents at O(1) artist queries.
	fallbackByEventID := make(map[string]catalogm.Show)
	// One zone lookup per distinct STATE rather than one per event:
	// time.LoadLocation re-reads the zoneinfo on every call, and this endpoint
	// accepts up to 200 events that in practice span a handful of states. Mirrors
	// the same cache in engagement.upcomingShowsForScene.
	zones := make(map[string]*time.Location)
	for _, e := range events {
		if matchedByEventID[e.ID] {
			continue // Already matched by source key
		}
		if e.Date == "" || e.VenueSlug == "" {
			continue
		}

		venueConfig, ok := VenueConfig[e.VenueSlug]
		if !ok {
			continue
		}

		// This is a read-only "does a show already exist at this venue that
		// day" status lookup for the scraper UI — NOT a dedup-rejection gate.
		// Unlike the dedup checks above, the CheckEventInput here carries only
		// a date (YYYY-MM-DD), with no time-of-day, so a full-timestamp
		// equality match is impossible. The same-day range is the correct
		// (and only feasible) comparison for a date-only input. Surfacing a
		// same-day match here never rejects an import; importEvent's
		// full-timestamp dedup checks remain the authoritative gate.
		//
		// The day is the VENUE'S day, not a UTC day, and that distinction is the
		// whole correctness of this branch. Shows are stored on the venue's
		// evening — an explicit scraped time, or parseEventDate's date-only
		// anchor — so a US venue's listings sit on the UTC day AFTER their own
		// calendar date. A UTC midnight-to-midnight window therefore matched the
		// wrong night: it missed every show on the date asked for and reported
		// the PREVIOUS night's show under it.
		loc, cached := zones[venueConfig.State]
		if !cached {
			loc = utils.EventLocation(nil, venueConfig.State)
			zones[venueConfig.State] = loc
		}
		startOfDay, err := time.ParseInLocation("2006-01-02", e.Date, loc)
		if err != nil {
			continue
		}
		// Walked forward by a CALENDAR day rather than 24 hours, so a day
		// containing a DST transition still closes at local midnight instead of
		// an hour either side of it.
		endOfDay := startOfDay.AddDate(0, 0, 1)

		var matchedShow catalogm.Show
		err = s.db.Joins("JOIN show_venues ON show_venues.show_id = shows.id").
			Joins("JOIN venues ON show_venues.venue_id = venues.id").
			Where("LOWER(venues.name) = LOWER(?) AND shows.event_date >= ? AND shows.event_date < ?",
				venueConfig.Name, startOfDay, endOfDay).
			Select("shows.id, shows.source_venue, shows.source_event_id, shows.status, shows.price, shows.age_requirement, shows.description, shows.event_date, shows.is_sold_out, shows.is_cancelled").
			First(&matchedShow).Error
		if err != nil {
			continue // No match found — that's fine
		}

		fallbackByEventID[e.ID] = matchedShow
	}

	// Batch-fetch artist names for every matched show (source-key + fallback) in
	// a single query, keyed by show ID. Avoids the per-show artist query that
	// previously ran inside buildCheckEventStatus — the admin handler allows up
	// to 200 events per call, so the per-show query was up to 200 round-trips.
	showIDs := make([]uint, 0, len(shows)+len(fallbackByEventID))
	for _, show := range shows {
		showIDs = append(showIDs, show.ID)
	}
	for _, show := range fallbackByEventID {
		showIDs = append(showIDs, show.ID)
	}
	artistsByShowID, err := shared.BatchResolveShowArtistNames(s.db, showIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch show artists: %w", err)
	}

	for _, show := range shows {
		if show.SourceEventID != nil {
			result.Events[*show.SourceEventID] = buildCheckEventStatus(show, artistsByShowID[show.ID])
		}
	}
	for eventID, show := range fallbackByEventID {
		result.Events[eventID] = buildCheckEventStatus(show, artistsByShowID[show.ID])
	}

	return result, nil
}

// buildCheckEventStatus creates a CheckEventStatus from a Show model and its
// pre-fetched artist names (see shared.BatchResolveShowArtistNames — names must
// not be queried per-show on the discovery hot path).
func buildCheckEventStatus(show catalogm.Show, artistNames []string) contracts.CheckEventStatus {
	return contracts.CheckEventStatus{
		Exists: true,
		ShowID: show.ID,
		Status: string(show.Status),
		CurrentData: &contracts.ShowCurrentData{
			Price:          show.Price,
			AgeRequirement: show.AgeRequirement,
			Description:    show.Description,
			EventDate:      show.EventDate.Format(time.RFC3339),
			IsSoldOut:      show.IsSoldOut,
			IsCancelled:    show.IsCancelled,
			Artists:        artistNames,
		},
	}
}

// ImportEvents imports events from an array of DiscoveredEvent objects
// This is used by the HTTP API endpoint for importing scraped data directly
func (s *DiscoveryService) ImportEvents(events []contracts.DiscoveredEvent, dryRun bool, allowUpdates bool, initialStatus catalogm.ShowStatus) (*contracts.ImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &contracts.ImportResult{
		Total:    len(events),
		Messages: make([]string, 0),
	}

	// Track event IDs that were imported so we can queue them for enrichment
	var importedEventIDs []string

	for _, event := range events {
		msg, status := s.importEvent(&event, dryRun, allowUpdates, initialStatus)
		result.Messages = append(result.Messages, msg)

		switch status {
		case "imported":
			result.Imported++
			importedEventIDs = append(importedEventIDs, event.ID)
		case "duplicate":
			result.Duplicates++
		case "rejected":
			result.Rejected++
		case "pending_review":
			result.PendingReview++
			importedEventIDs = append(importedEventIDs, event.ID)
		case "updated":
			result.Updated++
		case "error":
			result.Errors++
		}
	}

	// Fire-and-forget: queue newly imported shows for enrichment
	if !dryRun && s.enrichmentService != nil && len(importedEventIDs) > 0 {
		shared.GoSafe(context.Background(), "queue_imported_shows_enrichment", func() {
			s.queueImportedShowsForEnrichment(importedEventIDs)
		})
	}

	return result, nil
}

// queueImportedShowsForEnrichment looks up shows by source_event_id and queues them for enrichment.
func (s *DiscoveryService) queueImportedShowsForEnrichment(eventIDs []string) {
	for _, eventID := range eventIDs {
		var show catalogm.Show
		if err := s.db.Where("source_event_id = ?", eventID).First(&show).Error; err != nil {
			continue
		}
		if err := s.enrichmentService.QueueShowForEnrichment(show.ID, adminm.EnrichmentTypeAll); err != nil {
			// Fire-and-forget: log but don't fail
			fmt.Printf("warning: failed to queue show %d for enrichment: %v\n", show.ID, err)
		}
	}
}
