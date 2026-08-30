package admin

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// DataSyncService handles exporting and importing data between environments
type DataSyncService struct {
	db *gorm.DB
}

// NewDataSyncService creates a new data sync service
func NewDataSyncService(database *gorm.DB) *DataSyncService {
	if database == nil {
		database = db.GetDB()
	}
	return &DataSyncService{
		db: database,
	}
}

// formatOptionalRFC3339 renders a nullable instant for the export payload,
// preserving "unset" as an absent key rather than a zero date.
func formatOptionalRFC3339(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

// parseOptionalRFC3339 is its inverse. An absent key stays nil; a malformed one
// is an error rather than a silent drop, so a bad payload fails the import
// instead of quietly landing a show with no doors time.
func parseOptionalRFC3339(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, err
	}
	utc := parsed.UTC()
	return &utc, nil
}

// ExportShows exports shows with their artists and venues
func (s *DataSyncService) ExportShows(params contracts.ExportShowsParams) (*contracts.ExportShowsResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}

	// Build query
	query := s.db.Model(&catalogm.Show{}).
		Preload("Venues").
		Preload("Artists")

	// Apply status filter
	switch params.Status {
	case "approved":
		query = query.Where("status = ?", catalogm.ShowStatusApproved)
	case "pending":
		query = query.Where("status = ?", catalogm.ShowStatusPending)
	case "rejected":
		query = query.Where("status = ?", catalogm.ShowStatusRejected)
	case "all", "":
		// No filter - include all
	default:
		query = query.Where("status = ?", params.Status)
	}

	// Apply date filter
	if params.FromDate != nil {
		query = query.Where("event_date >= ?", params.FromDate)
	}

	// Apply location filters
	if params.City != "" {
		query = query.Where("city = ?", params.City)
	}
	if params.State != "" {
		query = query.Where("state = ?", params.State)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count shows: %w", err)
	}

	// Get shows with pagination
	var shows []catalogm.Show
	if err := query.Order("event_date DESC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&shows).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch shows: %w", err)
	}

	// Get show artists with position info
	showIDs := make([]uint, len(shows))
	for i, show := range shows {
		showIDs[i] = show.ID
	}

	var showArtists []catalogm.ShowArtist
	if len(showIDs) > 0 {
		if err := s.db.Where("show_id IN ?", showIDs).Find(&showArtists).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch show artists: %w", err)
		}
	}

	// Build show artist map
	showArtistMap := make(map[uint][]catalogm.ShowArtist)
	for _, sa := range showArtists {
		showArtistMap[sa.ShowID] = append(showArtistMap[sa.ShowID], sa)
	}

	// Convert to exported format
	exported := make([]contracts.ExportedShow, len(shows))
	for i, show := range shows {
		exported[i] = contracts.ExportedShow{
			Title:          show.Title,
			EventDate:      show.EventDate.Format(time.RFC3339),
			DoorsAt:        formatOptionalRFC3339(show.DoorsAt),
			MusicAt:        formatOptionalRFC3339(show.MusicAt),
			City:           show.City,
			State:          show.State,
			Price:          show.Price,
			DoorPrice:      show.DoorPrice,
			AgeRequirement: show.AgeRequirement,
			Description:    show.Description,
			Status:         string(show.Status),
			IsSoldOut:      show.IsSoldOut,
			IsCancelled:    show.IsCancelled,
			Venues:         make([]contracts.ExportedVenue, len(show.Venues)),
			Artists:        make([]contracts.ExportedShowArtist, 0),
		}

		// Convert venues
		for j, venue := range show.Venues {
			exported[i].Venues[j] = contracts.ExportedVenue{
				Name:       venue.Name,
				Address:    venue.Address,
				City:       venue.City,
				State:      venue.State,
				Zipcode:    venue.Zipcode,
				Verified:   venue.Verified,
				Instagram:  venue.Social.Instagram,
				Facebook:   venue.Social.Facebook,
				Twitter:    venue.Social.Twitter,
				YouTube:    venue.Social.YouTube,
				Spotify:    venue.Social.Spotify,
				SoundCloud: venue.Social.SoundCloud,
				Bandcamp:   venue.Social.Bandcamp,
				Website:    venue.Social.Website,
			}
		}

		// Convert artists with position info
		for _, artist := range show.Artists {
			// Find position info from showArtistMap
			position := 0
			setType := contracts.SetTypeDefault
			for _, sa := range showArtistMap[show.ID] {
				if sa.ArtistID == artist.ID {
					position = sa.Position
					setType = sa.SetType
					break
				}
			}

			exported[i].Artists = append(exported[i].Artists, contracts.ExportedShowArtist{
				Name:     artist.Name,
				Position: position,
				SetType:  setType,
			})
		}
	}

	return &contracts.ExportShowsResult{
		Shows: exported,
		Total: total,
	}, nil
}

// contracts.ExportArtistsParams contains filters for artist export
// ExportArtists exports artists
func (s *DataSyncService) ExportArtists(params contracts.ExportArtistsParams) (*contracts.ExportArtistsResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}

	query := s.db.Model(&catalogm.Artist{})

	if params.Search != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", shared.LikePattern(params.Search))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists: %w", err)
	}

	var artists []catalogm.Artist
	if err := query.Order("name ASC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&artists).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch artists: %w", err)
	}

	exported := make([]contracts.ExportedArtist, len(artists))
	for i, artist := range artists {
		exported[i] = contracts.ExportedArtist{
			Name:             artist.Name,
			City:             artist.City,
			State:            artist.State,
			BandcampEmbedURL: artist.BandcampEmbedURL,
			Instagram:        artist.Social.Instagram,
			Facebook:         artist.Social.Facebook,
			Twitter:          artist.Social.Twitter,
			YouTube:          artist.Social.YouTube,
			Spotify:          artist.Social.Spotify,
			SoundCloud:       artist.Social.SoundCloud,
			Bandcamp:         artist.Social.Bandcamp,
			Website:          artist.Social.Website,
		}
	}

	return &contracts.ExportArtistsResult{
		Artists: exported,
		Total:   total,
	}, nil
}

// contracts.ExportVenuesParams contains filters for venue export
// ExportVenues exports venues
func (s *DataSyncService) ExportVenues(params contracts.ExportVenuesParams) (*contracts.ExportVenuesResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}

	query := s.db.Model(&catalogm.Venue{})

	if params.Search != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", shared.LikePattern(params.Search))
	}
	if params.Verified != nil {
		query = query.Where("verified = ?", *params.Verified)
	}
	if params.City != "" {
		query = query.Where("city = ?", params.City)
	}
	if params.State != "" {
		query = query.Where("state = ?", params.State)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	}

	var venues []catalogm.Venue
	if err := query.Order("name ASC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&venues).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch venues: %w", err)
	}

	exported := make([]contracts.ExportedVenue, len(venues))
	for i, venue := range venues {
		exported[i] = contracts.ExportedVenue{
			Name:       venue.Name,
			Address:    venue.Address,
			City:       venue.City,
			State:      venue.State,
			Zipcode:    venue.Zipcode,
			Verified:   venue.Verified,
			Instagram:  venue.Social.Instagram,
			Facebook:   venue.Social.Facebook,
			Twitter:    venue.Social.Twitter,
			YouTube:    venue.Social.YouTube,
			Spotify:    venue.Social.Spotify,
			SoundCloud: venue.Social.SoundCloud,
			Bandcamp:   venue.Social.Bandcamp,
			Website:    venue.Social.Website,
		}
	}

	return &contracts.ExportVenuesResult{
		Venues: exported,
		Total:  total,
	}, nil
}

// contracts.DataImportRequest represents a batch import request
// ImportData imports shows, artists, and venues with deduplication
func (s *DataSyncService) ImportData(req contracts.DataImportRequest) (*contracts.DataImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &contracts.DataImportResult{}
	result.Shows.Messages = make([]string, 0)
	result.Artists.Messages = make([]string, 0)
	result.Venues.Messages = make([]string, 0)

	// Import artists first (shows depend on them)
	result.Artists.Total = len(req.Artists)
	for _, artist := range req.Artists {
		msg, status := s.importArtist(&artist, req.DryRun)
		result.Artists.Messages = append(result.Artists.Messages, msg)
		switch status {
		case "imported":
			result.Artists.Imported++
		case "duplicate":
			result.Artists.Duplicates++
		case "updated":
			result.Artists.Updated++
		case "error":
			result.Artists.Errors++
		}
	}

	// Import venues second (shows depend on them)
	result.Venues.Total = len(req.Venues)
	for _, venue := range req.Venues {
		msg, status := s.importVenue(&venue, req.DryRun)
		result.Venues.Messages = append(result.Venues.Messages, msg)
		switch status {
		case "imported":
			result.Venues.Imported++
		case "duplicate":
			result.Venues.Duplicates++
		case "updated":
			result.Venues.Updated++
		case "error":
			result.Venues.Errors++
		}
	}

	// Import shows last
	result.Shows.Total = len(req.Shows)
	for _, show := range req.Shows {
		msg, status := s.importShow(&show, req.DryRun)
		result.Shows.Messages = append(result.Shows.Messages, msg)
		switch status {
		case "imported":
			result.Shows.Imported++
		case "duplicate":
			result.Shows.Duplicates++
		case "error":
			result.Shows.Errors++
		}
	}

	return result, nil
}

// importArtist imports a single artist with deduplication
func (s *DataSyncService) importArtist(artist *contracts.ExportedArtist, dryRun bool) (string, string) {
	if artist.Name == "" {
		return "SKIP: Artist name is required", "error"
	}

	// Probe first so the DUPLICATE / WOULD IMPORT / IMPORTED message + dry-run gate
	// can be decided before any write; the actual create + slug-backfill then route
	// through the single artist funnel (PSY-1254).
	var existing catalogm.Artist
	err := s.db.Where("LOWER(name) = LOWER(?)", artist.Name).First(&existing).Error
	switch {
	case err == nil:
		if !dryRun {
			// Backfill a missing slug via the funnel (a no-op when already set).
			if _, _, ferr := catalog.FindOrCreateArtistTx(s.db, artist.Name, nil); ferr != nil {
				return fmt.Sprintf("ERROR: Failed to backfill artist '%s': %v", artist.Name, ferr), "error"
			}
		}
		return fmt.Sprintf("DUPLICATE: Artist '%s' already exists (ID: %d)", artist.Name, existing.ID), "duplicate"
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return fmt.Sprintf("ERROR: Failed to check artist '%s': %v", artist.Name, err), "error"
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: Artist '%s'", artist.Name), "imported"
	}

	newArtist, _, ferr := catalog.FindOrCreateArtistTx(s.db, artist.Name, func(a *catalogm.Artist) {
		a.City = artist.City
		a.State = artist.State
		a.BandcampEmbedURL = artist.BandcampEmbedURL
		a.Social = catalogm.Social{
			Instagram:  artist.Instagram,
			Facebook:   artist.Facebook,
			Twitter:    artist.Twitter,
			YouTube:    artist.YouTube,
			Spotify:    artist.Spotify,
			SoundCloud: artist.SoundCloud,
			Bandcamp:   artist.Bandcamp,
			Website:    artist.Website,
		}
	})
	if ferr != nil {
		return fmt.Sprintf("ERROR: Failed to create artist '%s': %v", artist.Name, ferr), "error"
	}

	return fmt.Sprintf("IMPORTED: Artist '%s' (ID: %d)", artist.Name, newArtist.ID), "imported"
}

// importVenue imports a single venue with deduplication
func (s *DataSyncService) importVenue(venue *contracts.ExportedVenue, dryRun bool) (string, string) {
	if venue.Name == "" || venue.City == "" || venue.State == "" {
		return "SKIP: Venue name, city, and state are required", "error"
	}

	// Check for existing venue by name + city (case insensitive)
	var existing catalogm.Venue
	err := s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", venue.Name, venue.City).First(&existing).Error
	if err == nil {
		// Venue exists — backfill slug if missing
		if existing.Slug == nil && !dryRun {
			baseSlug := utils.GenerateVenueSlug(existing.Name, existing.City, existing.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				s.db.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			s.db.Model(&existing).Update("slug", slug)
		}
		return fmt.Sprintf("DUPLICATE: Venue '%s' in %s already exists (ID: %d)", venue.Name, venue.City, existing.ID), "duplicate"
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Sprintf("ERROR: Failed to check venue '%s': %v", venue.Name, err), "error"
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: Venue '%s' in %s, %s", venue.Name, venue.City, venue.State), "imported"
	}

	// Create new venue with slug
	baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	newVenue := catalogm.Venue{
		Name:     venue.Name,
		Slug:     &slug,
		Address:  venue.Address,
		City:     venue.City,
		State:    venue.State,
		Zipcode:  venue.Zipcode,
		Verified: venue.Verified,
		Social: catalogm.Social{
			Instagram:  venue.Instagram,
			Facebook:   venue.Facebook,
			Twitter:    venue.Twitter,
			YouTube:    venue.YouTube,
			Spotify:    venue.Spotify,
			SoundCloud: venue.SoundCloud,
			Bandcamp:   venue.Bandcamp,
			Website:    venue.Website,
		},
	}

	// PSY-985: geocode imported venues so timezone/coordinates are populated like
	// the VenueService create path (nil on a miss → legacy state->tz fallback).
	// Since PSY-1747 it is literally the same derivation, not a parallel one.
	// Street-level geocoding (PSY-1536) is deliberately skipped on this bulk
	// import seam — new rows start with NULL street fields and the
	// geocode-venue-addresses backfill CLI resolves them afterwards.
	//
	// ExportedVenue carries no country field, so newVenue.Country is nil here and
	// the location resolves with a blank country exactly as the hardcoded "" this
	// replaced did — see shared.VenueLocation for why reading the column is the
	// right shape anyway.
	shared.DeriveVenueLocation(s.db, geo.Default(), shared.VenueLocation(&newVenue),
		"venue_name", newVenue.Name, "city", newVenue.City, "state", newVenue.State).
		ApplyTo(&newVenue)

	if err := s.db.Create(&newVenue).Error; err != nil {
		return fmt.Sprintf("ERROR: Failed to create venue '%s': %v", venue.Name, err), "error"
	}

	return fmt.Sprintf("IMPORTED: Venue '%s' in %s (ID: %d)", venue.Name, venue.City, newVenue.ID), "imported"
}

// venueTimezoneForShow reads the IANA zone already stored on the show's primary
// venue, so utils.GenerateShowSlug can date the slug in the venue's real zone
// rather than the US state map's Phoenix default (PSY-1873).
//
// The export format carries no timezone (ExportedVenue is name/address/socials
// only), and the zone is derived data the importer geocodes onto the venue row
// itself, so the authority is the DB rather than the payload.
//
// A missing venue is not an error: an import that creates the venue and the
// show in one pass still slugs from the state, and the venue's real zone
// reaches the slug on the next re-import or via cmd/dedup-shows'
// RecanonicaliseShowSlug.
//
// A failed QUERY is returned, not swallowed. Callers inside a transaction MUST
// abort on it: the failed statement has already put Postgres into an aborted
// transaction, so continuing only produces "current transaction is aborted" on
// some later, innocent statement. venueTimezoneForShowBestEffort is the
// non-transactional variant for callers that genuinely can degrade.
func venueTimezoneForShow(db *gorm.DB, venues []contracts.ExportedVenue) (*string, error) {
	if len(venues) == 0 {
		return nil, nil
	}
	return shared.VenueTimezoneByNameCity(db, venues[0].Name, venues[0].City)
}

// venueTimezoneForShowBestEffort is venueTimezoneForShow for a caller running
// OUTSIDE a transaction, where a lookup failure costs one slug's accuracy
// rather than the correctness of everything after it. It logs and falls back to
// the state map, which is what the slug used before this existed.
func venueTimezoneForShowBestEffort(db *gorm.DB, venues []contracts.ExportedVenue) *string {
	tz, err := venueTimezoneForShow(db, venues)
	if err != nil {
		name, city := "", ""
		if len(venues) > 0 {
			name, city = venues[0].Name, venues[0].City
		}
		slog.Error("could not read the venue timezone for a show slug; falling back to the state map",
			"venue_name", name, "city", city, "error", err)
		return nil
	}
	return tz
}

// importShow imports a single show with deduplication
func (s *DataSyncService) importShow(show *contracts.ExportedShow, dryRun bool) (string, string) {
	if show.Title == "" || show.EventDate == "" {
		return "SKIP: Show title and event date are required", "error"
	}

	// Parse event date
	eventDate, err := time.Parse(time.RFC3339, show.EventDate)
	if err != nil {
		return fmt.Sprintf("ERROR: Invalid event date '%s': %v", show.EventDate, err), "error"
	}

	// Show times are optional; a malformed one is an error rather than a silent
	// drop, so a bad payload is counted and reported instead of landing a show
	// that quietly lost its doors time.
	doorsAt, err := parseOptionalRFC3339(show.DoorsAt)
	if err != nil {
		return fmt.Sprintf("ERROR: Invalid doorsAt '%s': %v", *show.DoorsAt, err), "error"
	}
	musicAt, err := parseOptionalRFC3339(show.MusicAt)
	if err != nil {
		return fmt.Sprintf("ERROR: Invalid musicAt '%s': %v", *show.MusicAt, err), "error"
	}

	// Get venue name for deduplication
	venueName := ""
	if len(show.Venues) > 0 {
		venueName = show.Venues[0].Name
	}

	// Check for duplicate: same title + venue + event_date. The dedup key is the
	// full event_date timestamp (not the calendar day), so a matinee and an evening
	// show at the same title/venue are distinct rows rather than a false duplicate.
	// EventDate round-trips through RFC3339 on export/import, preserving time-of-day.
	if venueName != "" {
		var existingShow catalogm.Show
		err := s.db.Joins("JOIN show_venues ON shows.id = show_venues.show_id").
			Joins("JOIN venues ON show_venues.venue_id = venues.id").
			Where("LOWER(shows.title) = LOWER(?) AND LOWER(venues.name) = LOWER(?) AND shows.event_date = ?",
				show.Title, venueName, eventDate).
			First(&existingShow).Error
		if err == nil {
			// Backfill slugs for the existing show and its associated entities
			if !dryRun {
				s.backfillShowSlugs(&existingShow, show, eventDate, venueName)
			}
			return fmt.Sprintf("DUPLICATE: Show '%s' at %s on %s already exists (ID: %d)",
				show.Title, venueName, eventDate.Format("2006-01-02"), existingShow.ID), "duplicate"
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Sprintf("ERROR: Failed to check show '%s': %v", show.Title, err), "error"
		}
	}

	if dryRun {
		return fmt.Sprintf("WOULD IMPORT: Show '%s' at %s on %s", show.Title, venueName, eventDate.Format("2006-01-02")), "imported"
	}

	// Create show in a transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Parse status
		status := catalogm.ShowStatusApproved
		switch strings.ToLower(show.Status) {
		case "pending":
			status = catalogm.ShowStatusPending
		case "rejected":
			status = catalogm.ShowStatusRejected
		case "private":
			status = catalogm.ShowStatusPrivate
		}

		// Determine headliner name for show slug
		headlinerName := ""
		for _, a := range show.Artists {
			if a.Position == 0 || headlinerName == "" {
				headlinerName = a.Name
			}
		}

		// Determine state for timezone-aware slug
		showState := ""
		if show.State != nil {
			showState = *show.State
		} else if len(show.Venues) > 0 {
			showState = show.Venues[0].State
		}

		// Generate show slug. The zone lookup runs on tx, so a failure has
		// already aborted this transaction: every statement after it would fail
		// with "current transaction is aborted" and the operator would be
		// debugging the wrong one. Fail here, where the cause is named.
		venueTZ, err := venueTimezoneForShow(tx, show.Venues)
		if err != nil {
			return err
		}
		baseShowSlug := utils.GenerateShowSlug(eventDate.UTC(), headlinerName, venueName,
			venueTZ, showState)
		showSlug := utils.GenerateUniqueSlug(baseShowSlug, func(candidate string) bool {
			var count int64
			tx.Model(&catalogm.Show{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})

		newShow := catalogm.Show{
			Title:          show.Title,
			Slug:           &showSlug,
			EventDate:      eventDate.UTC(),
			DoorsAt:        doorsAt,
			MusicAt:        musicAt,
			City:           show.City,
			State:          show.State,
			Price:          show.Price,
			DoorPrice:      show.DoorPrice,
			AgeRequirement: show.AgeRequirement,
			Description:    show.Description,
			Status:         status,
			Source:         catalogm.ShowSourceUser,
			IsSoldOut:      show.IsSoldOut,
			IsCancelled:    show.IsCancelled,
		}

		if err := tx.Create(&newShow).Error; err != nil {
			return fmt.Errorf("failed to create show: %w", err)
		}

		// Link venues. Track the lowest venue.ID for the denormalized
		// show_artists.venue_id below — matches the 20260512023704
		// backfill migration's LATERAL tiebreaker (PSY-576).
		var primaryVenueID *uint
		for _, exportedVenue := range show.Venues {
			var venue catalogm.Venue
			err := tx.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)",
				exportedVenue.Name, exportedVenue.City).First(&venue).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Create venue with slug
				venueBaseSlug := utils.GenerateVenueSlug(exportedVenue.Name, exportedVenue.City, exportedVenue.State)
				venueSlug := utils.GenerateUniqueSlug(venueBaseSlug, func(candidate string) bool {
					var count int64
					tx.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
					return count > 0
				})
				venue = catalogm.Venue{
					Name:     exportedVenue.Name,
					Slug:     &venueSlug,
					Address:  exportedVenue.Address,
					City:     exportedVenue.City,
					State:    exportedVenue.State,
					Zipcode:  exportedVenue.Zipcode,
					Verified: exportedVenue.Verified,
				}
				// PSY-985: geocode imported venues (see importVenue). Validates
				// through tx, not s.db, so the write-boundary guard runs inside the
				// transaction carrying the write it guards.
				shared.DeriveVenueLocation(tx, geo.Default(), shared.VenueLocation(&venue),
					"venue_name", venue.Name, "city", venue.City, "state", venue.State).
					ApplyTo(&venue)
				if err := tx.Create(&venue).Error; err != nil {
					return fmt.Errorf("failed to create venue: %w", err)
				}
			} else if err != nil {
				return fmt.Errorf("failed to find venue: %w", err)
			} else if venue.Slug == nil {
				// Backfill slug for existing venue
				venueBaseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
				venueSlug := utils.GenerateUniqueSlug(venueBaseSlug, func(candidate string) bool {
					var count int64
					tx.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
					return count > 0
				})
				tx.Model(&venue).Update("slug", venueSlug)
			}

			// Create show-venue association
			showVenue := catalogm.ShowVenue{
				ShowID:  newShow.ID,
				VenueID: venue.ID,
			}
			if err := tx.Create(&showVenue).Error; err != nil {
				return fmt.Errorf("failed to link venue: %w", err)
			}

			if primaryVenueID == nil || venue.ID < *primaryVenueID {
				vid := venue.ID
				primaryVenueID = &vid
			}
		}

		// Link artists. EventDate + VenueID denormalize the show dedup
		// key so the partial unique index
		// `shows_artist_venue_eventdate_uniq` covers data-synced rows
		// (PSY-576).
		syncEventDate := newShow.EventDate
		for _, exportedArtist := range show.Artists {
			// Single artist write path (PSY-1254): dedup, unique slug, insert, and
			// slug-backfill of an existing slug-less artist — all in the funnel.
			foundArtist, _, err := catalog.FindOrCreateArtistTx(tx, exportedArtist.Name, nil)
			if err != nil {
				return fmt.Errorf("artist %s: %w", exportedArtist.Name, err)
			}
			artist := *foundArtist

			// Create show-artist association.
			//
			// The set_type is normalized rather than trusted: this is the one
			// write path that reaches show_artists without passing the show
			// service's validator or the OpenAPI enum, and the payload is an
			// export file from another environment that may predate the
			// vocabulary. Anything unmappable becomes the neutral default, so
			// a stale export cannot seed a role nobody asserted.
			syncSetType := contracts.NormalizeSetType(exportedArtist.SetType)
			if syncSetType == "" {
				syncSetType = contracts.SetTypeDefault
			}
			showArtist := catalogm.ShowArtist{
				ShowID:    newShow.ID,
				ArtistID:  artist.ID,
				Position:  exportedArtist.Position,
				SetType:   syncSetType,
				EventDate: &syncEventDate,
				VenueID:   primaryVenueID,
			}
			if err := tx.Create(&showArtist).Error; err != nil {
				return fmt.Errorf("failed to link artist: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Sprintf("ERROR: Failed to import show '%s': %v", show.Title, err), "error"
	}

	return fmt.Sprintf("IMPORTED: Show '%s' at %s on %s", show.Title, venueName, eventDate.Format("2006-01-02")), "imported"
}

// backfillShowSlugs generates slugs for an existing show and its associated artists/venues if missing.
func (s *DataSyncService) backfillShowSlugs(existingShow *catalogm.Show, show *contracts.ExportedShow, eventDate time.Time, venueName string) {
	// Backfill show slug
	if existingShow.Slug == nil {
		headlinerName := ""
		for _, a := range show.Artists {
			if a.Position == 0 || headlinerName == "" {
				headlinerName = a.Name
			}
		}
		showState := ""
		if show.State != nil {
			showState = *show.State
		} else if len(show.Venues) > 0 {
			showState = show.Venues[0].State
		}
		baseSlug := utils.GenerateShowSlug(eventDate.UTC(), headlinerName, venueName,
			venueTimezoneForShowBestEffort(s.db, show.Venues), showState)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&catalogm.Show{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})
		s.db.Model(existingShow).Update("slug", slug)
	}

	// Backfill artist slugs
	for _, exportedArtist := range show.Artists {
		var artist catalogm.Artist
		if err := s.db.Where("LOWER(name) = LOWER(?)", exportedArtist.Name).First(&artist).Error; err != nil {
			continue
		}
		if artist.Slug == nil {
			baseSlug := utils.GenerateArtistSlug(artist.Name)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				s.db.Model(&catalogm.Artist{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			s.db.Model(&artist).Update("slug", slug)
		}
	}

	// Backfill venue slugs
	for _, exportedVenue := range show.Venues {
		var venue catalogm.Venue
		if err := s.db.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", exportedVenue.Name, exportedVenue.City).First(&venue).Error; err != nil {
			continue
		}
		if venue.Slug == nil {
			baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				s.db.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			s.db.Model(&venue).Update("slug", slug)
		}
	}
}
