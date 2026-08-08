package catalog

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// fnvHash produces a stable int64 advisory-lock key (xact- or session-scoped):
// show.go uses it for pg_advisory_xact_lock; radio_sync.go for a session lock.
func fnvHash(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64())
}

// utcOrNil normalizes an optional instant to UTC, preserving nil.
//
// This does not affect what lands in the column: TIMESTAMPTZ stores an instant
// and no offset, so the write is identical either way. What it buys is that
// the in-memory response CreateShow builds inside the transaction renders in
// the same zone as a later read, instead of echoing back whatever offset the
// request happened to use.
func utcOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

// ShowService handles show-related business logic
type ShowService struct {
	db *gorm.DB
	// geocoder resolves a (city, state) to its centroid coordinates using the
	// same offline GeoNames dataset PSY-985 uses for venues. GetShowCities
	// surfaces the centroid per show-city so the frontend can pick the nearest
	// has-shows city for a new visitor (PSY-981). It's process-wide and
	// stateless, so sharing geo.Default() is safe.
	geocoder geo.Geocoder
}

// NewShowService creates a new show service
func NewShowService(database *gorm.DB) *ShowService {
	if database == nil {
		database = db.GetDB()
	}
	return &ShowService{
		db:       database,
		geocoder: geo.Default(),
	}
}

// CreateShow creates a new show with associated venues and artists.
// Prevents duplicate headliners at the same venue on the same date/time.
// Prevents duplicate venues with the same name in the same city.
// Status is determined based on venue verification and submitter admin status.
func (s *ShowService) CreateShow(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Re-check the show-time order here, mirroring how instagram_handle is
	// handled: the handler's Resolve gives the common path a field-located 422,
	// and this backstops CreateShow callers that never run it. In practice that
	// is the entity-request fulfiller, which validates the same rule earlier at
	// queue-create; this is defense in depth, not the primary gate.
	//
	// Note this is NOT every write path: DataSyncService.importShow builds the
	// row and calls tx.Create directly, below this function, so an import can
	// still land a disordered pair. That is tolerated on purpose, same as a
	// revision rollback restoring one, and the update guard keeps such a row
	// editable.
	//
	// Deliberately NOT mirrored on the update path. There, the equivalent guard
	// runs in the handler and only when the request touches a time, because a
	// row can hold a disordered pair legitimately: revision Rollback restores
	// whatever state was recorded, and a restore must never be refused. Blocking
	// writes on stored disorder would make such a row uneditable.
	if req.DoorsAt != nil && req.MusicAt != nil && req.MusicAt.Before(*req.DoorsAt) {
		return nil, apperrors.ErrShowValidationFailed("music_at cannot be before doors_at")
	}

	// Same defense-in-depth shape as above: the handler's OpenAPI enum rejects a
	// bad set_type with a field-located 422, and this backstops in-process
	// callers (the entity-request fulfiller, the show importer) that never see
	// that schema.
	if err := validateShowArtistSetTypes(req.Artists); err != nil {
		return nil, err
	}

	// Use transaction for data consistency
	var response *contracts.ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Check for duplicate headliner-venue-date conflicts
		if err := s.checkDuplicateHeadlinerConflicts(tx, req); err != nil {
			return err
		}

		// Determine show status based on venue verification and privacy preference
		status := s.determineShowStatus(tx, req.Venues, req.SubmitterIsAdmin, req.IsPrivate)

		// Create the show
		show := &catalogm.Show{
			Title:          req.Title,
			EventDate:      req.EventDate.UTC(), // Ensure UTC storage
			DoorsAt:        utcOrNil(req.DoorsAt),
			MusicAt:        utcOrNil(req.MusicAt),
			City:           &req.City,
			State:          &req.State,
			Price:          req.Price,
			AgeRequirement: &req.AgeRequirement,
			Description:    &req.Description,
			ImageURL:       req.ImageURL,
			Status:         status,
			SubmittedBy:    req.SubmittedByUserID,
		}
		if req.TicketURL != "" {
			show.TicketURL = &req.TicketURL
		}

		if err := tx.Create(show).Error; err != nil {
			return fmt.Errorf("failed to create show: %w", err)
		}

		// Associate venues (pass admin status for venue verification)
		venues, err := s.associateVenues(tx, show.ID, req.Venues, req.SubmitterIsAdmin)
		if err != nil {
			return fmt.Errorf("failed to associate venues: %w", err)
		}

		// Associate artists
		artists, err := s.associateArtists(tx, show.ID, req.Artists)
		if err != nil {
			return fmt.Errorf("failed to associate artists: %w", err)
		}

		// Stamp the denormalized (event_date, venue_id) columns on the
		// just-created show_artists rows so the partial unique index
		// `shows_artist_venue_eventdate_uniq` covers them (PSY-576).
		if err := syncShowArtistDedupColumns(tx, show.ID); err != nil {
			return fmt.Errorf("failed to sync show_artists dedup columns: %w", err)
		}

		// Generate slug after artists and venues are associated
		headlinerName := "unknown"
		venueName := "unknown"
		for _, a := range artists {
			if a.IsHeadliner != nil && *a.IsHeadliner {
				headlinerName = a.Name
				break
			}
		}
		if len(artists) > 0 && headlinerName == "unknown" {
			headlinerName = artists[0].Name
		}
		if len(venues) > 0 {
			venueName = venues[0].Name
		}

		// Use show state for timezone-aware slug date
		showState := ""
		if show.State != nil {
			showState = *show.State
		}
		baseSlug := utils.GenerateShowSlug(show.EventDate, headlinerName, venueName, showState)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			tx.Model(&catalogm.Show{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})

		// Update show with slug
		if err := tx.Model(show).Update("slug", slug).Error; err != nil {
			return fmt.Errorf("failed to update show slug: %w", err)
		}

		// Build response
		response = &contracts.ShowResponse{
			ID:              show.ID,
			Slug:            slug,
			Title:           show.Title,
			EventDate:       show.EventDate,
			DoorsAt:         show.DoorsAt,
			MusicAt:         show.MusicAt,
			City:            show.City,
			State:           show.State,
			Price:           show.Price,
			AgeRequirement:  show.AgeRequirement,
			Description:     show.Description,
			TicketURL:       show.TicketURL,
			ImageURL:        show.ImageURL,
			Status:          string(show.Status),
			SubmittedBy:     show.SubmittedBy,
			RejectionReason: show.RejectionReason,
			Venues:          venues,
			Artists:         artists,
			CreatedAt:       show.CreatedAt,
			UpdatedAt:       show.UpdatedAt,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// determineShowStatus determines whether a show should be approved or private.
// Shows from unverified venues are now approved but display city-only until venue is verified.
// Private shows remain private (user's list only).
func (s *ShowService) determineShowStatus(tx *gorm.DB, venues []contracts.CreateShowVenue, isAdmin bool, isPrivate bool) catalogm.ShowStatus {
	// Private shows stay private regardless of venue status
	if isPrivate {
		return catalogm.ShowStatusPrivate
	}

	// All other shows are approved - unverified venues will show city-only on frontend
	return catalogm.ShowStatusApproved
}

// checkDuplicateHeadlinerConflicts checks if any headliners are already performing
// at the same venue on the same date/time.
// Uses pg_advisory_xact_lock to prevent race conditions where two concurrent
// requests could both pass the check before either commits.
func (s *ShowService) checkDuplicateHeadlinerConflicts(tx *gorm.DB, req *contracts.CreateShowRequest) error {
	// Get all headliners from the request.
	//
	// Resolved with the same function and the same positions associateArtists
	// will use, so the names locked and probed here are exactly the rows about
	// to be written as headliners. Reading a different signal -- or the same
	// signal without position -- would let an artist be written into the
	// headliner slot without ever being duplicate-checked.
	var headlinerNames []string
	for position, artist := range req.Artists {
		if _, isHeadliner := resolveArtistRole(artist, position); isHeadliner {
			headlinerNames = append(headlinerNames, artist.Name)
		}
	}

	// If no headliners marked, fall back to first-billed artist
	if len(headlinerNames) == 0 {
		if len(req.Artists) > 0 {
			headlinerNames = []string{req.Artists[0].Name}
		} else {
			return nil
		}
	}

	// Get all venue names from the request
	var venueNames []string
	for _, venue := range req.Venues {
		venueNames = append(venueNames, venue.Name)
	}

	// The dedup key is the FULL event_date timestamp (PSY-559): a matinee and
	// an evening set at the same venue with the same headliner are distinct
	// shows, not duplicates. Match on exact-timestamp equality, mirroring the
	// shows_artist_venue_eventdate_uniq unique index.
	eventDate := req.EventDate.UTC()

	// Acquire advisory lock keyed on (headliner, venue, exact timestamp) to
	// serialize concurrent inserts of the SAME show. Keying on the full
	// timestamp (not the calendar day) lets matinee + evening inserts proceed
	// in parallel. Uses FNV hash for a stable int64 lock key; auto-released on
	// transaction commit/rollback.
	for _, headlinerName := range headlinerNames {
		for _, venueName := range venueNames {
			lockKey := fnvHash(strings.ToLower(headlinerName) + "|" + strings.ToLower(venueName) + "|" + eventDate.Format(time.RFC3339Nano))
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", lockKey).Error; err != nil {
				return fmt.Errorf("failed to acquire advisory lock: %w", err)
			}
		}
	}

	// Check for conflicts: same headliner + same venue + same exact timestamp
	// (case-insensitive). Matches headliner by explicit set_type='headliner'
	// OR position=0 (first billed artist).
	for _, headlinerName := range headlinerNames {
		for _, venueName := range venueNames {
			var existingShows []catalogm.Show

			err := tx.Table("shows").
				Joins("JOIN show_artists ON shows.id = show_artists.show_id").
				Joins("JOIN artists ON show_artists.artist_id = artists.id").
				Joins("JOIN show_venues ON shows.id = show_venues.show_id").
				Joins("JOIN venues ON show_venues.venue_id = venues.id").
				Where("LOWER(artists.name) = LOWER(?) AND LOWER(venues.name) = LOWER(?)",
					headlinerName, venueName).
				Where("(show_artists.set_type = ? OR show_artists.position = 0)", "headliner").
				Where("shows.event_date = ?", eventDate).
				Find(&existingShows).Error

			if err != nil {
				return fmt.Errorf("failed to check for duplicate headliner conflicts: %w", err)
			}

			if len(existingShows) > 0 {
				return fmt.Errorf("headliner '%s' is already performing at venue '%s' on %s",
					headlinerName, venueName, req.EventDate.Format("2006-01-02 15:04:05 UTC"))
			}
		}
	}

	return nil
}

// GetShow retrieves a show by ID with all associations
func (s *ShowService) GetShow(showID uint) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var show catalogm.Show
	err := s.db.Preload("Venues").Preload("Artists").First(&show, showID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrShowNotFound(showID)
		}
		return nil, fmt.Errorf("failed to get show: %w", err)
	}

	resp := s.buildShowResponse(&show)
	s.attachBillLabels(resp)
	return resp, nil
}

// GetShowBySlug retrieves a show by slug with all associations
func (s *ShowService) GetShowBySlug(slug string) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var show catalogm.Show
	err := s.db.Preload("Venues").Preload("Artists").Where("slug = ?", slug).First(&show).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrShowNotFound(0)
		}
		return nil, fmt.Errorf("failed to get show: %w", err)
	}

	resp := s.buildShowResponse(&show)
	s.attachBillLabels(resp)
	return resp, nil
}

// GetShows retrieves shows with optional filtering
func (s *ShowService) GetShows(filters map[string]interface{}) ([]*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Preload("Venues").Preload("Artists").
		Where("status = ?", catalogm.ShowStatusApproved)

	// Apply filters
	if city, ok := filters["city"].(string); ok && city != "" {
		query = query.Where("city = ?", city)
	}
	if state, ok := filters["state"].(string); ok && state != "" {
		query = query.Where("state = ?", state)
	}
	// Timezone note (PSY-987): from_date/to_date are matched against the stored
	// UTC event_date verbatim — this endpoint does NOT reinterpret a bare
	// calendar date into a venue-local day window. Callers that want a
	// "venue-local calendar day" window must convert the boundaries to UTC
	// themselves before calling, the way the ph CLI's showDedupWindow does
	// (PSY-999): a date-only show is stored at 20:00 venue-local -> UTC, which
	// lands on the next UTC day for US zones, so a naive midnight-UTC window
	// would miss it.
	if fromDate, ok := filters["from_date"].(time.Time); ok {
		query = query.Where("event_date >= ?", fromDate.UTC())
	}
	if toDate, ok := filters["to_date"].(time.Time); ok {
		query = query.Where("event_date <= ?", toDate.UTC())
	}
	if tf, ok := filters["tag_filter"].(TagFilter); ok {
		// PSY-499: Shows are not directly tagged with genre/locale tags — they
		// inherit meaning from the billed artists. Filter shows whose lineup
		// includes artists matching the tag filter.
		query = ApplyTransitiveArtistTagFilter(
			query, s.db,
			"show_artists", "show_id", "artist_id",
			"shows.id", tf,
		)
	}

	// Default ordering by event date
	query = query.Order("event_date ASC")

	var shows []catalogm.Show
	err := query.Find(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get shows: %w", err)
	}

	// Build responses
	responses := make([]*contracts.ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, nil
}

// GetUserSubmissions returns all shows submitted by a specific user
func (s *ShowService) GetUserSubmissions(userID uint, limit, offset int) ([]contracts.ShowResponse, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Get total count first
	var total int64
	if err := s.db.Model(&catalogm.Show{}).Where("submitted_by = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user submissions: %w", err)
	}

	// Query shows with pagination
	var shows []catalogm.Show
	err := s.db.Preload("Venues").Preload("Artists").
		Where("submitted_by = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user submissions: %w", err)
	}

	// Build responses
	responses := make([]contracts.ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = *s.buildShowResponse(&show)
	}

	return responses, int(total), nil
}

// UpdateShow updates an existing show (basic fields only)
// showUpdatesToMap translates a typed UpdateShowRequest into a GORM update
// map. Only non-nil fields are written so omitted fields stay unchanged.
// EventDate is normalized to UTC before storing so the denormalized
// show_artists.event_date stays in sync downstream; ImageURL is nullable and
// normalizes empty input to SQL NULL. The remaining fields are written
// verbatim to preserve prior handler behavior. A nil req yields an empty map
// (used by the relations path when only associations change).
func showUpdatesToMap(req *contracts.UpdateShowRequest) map[string]interface{} {
	updates := map[string]interface{}{}
	if req == nil {
		return updates
	}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.EventDate != nil {
		updates["event_date"] = req.EventDate.UTC()
	}
	if req.DoorsAt != nil {
		updates["doors_at"] = req.DoorsAt.UTC()
	}
	if req.MusicAt != nil {
		updates["music_at"] = req.MusicAt.UTC()
	}
	if req.City != nil {
		updates["city"] = *req.City
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.Price != nil {
		updates["price"] = *req.Price
	}
	if req.AgeRequirement != nil {
		updates["age_requirement"] = *req.AgeRequirement
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.TicketURL != nil {
		updates["ticket_url"] = *req.TicketURL
	}
	if req.ImageURL != nil {
		updates["image_url"] = utils.NilIfEmpty(*req.ImageURL)
	}
	return updates
}

func (s *ShowService) UpdateShow(showID uint, req *contracts.UpdateShowRequest) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	updates := showUpdatesToMap(req)

	_, eventDateChanged := updates["event_date"]
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&catalogm.Show{}).Where("id = ?", showID).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update show: %w", err)
		}
		// If the event_date moved, cascade onto the denormalized
		// show_artists.event_date so the partial unique index stays in
		// sync (PSY-576).
		if eventDateChanged {
			if err := syncShowArtistDedupColumns(tx, showID); err != nil {
				return fmt.Errorf("failed to sync show_artists dedup columns: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetShow(showID)
}

// contracts.OrphanedArtist represents an artist that has no remaining show associations
// after a show update replaced its artist list

// UpdateShowWithRelations updates a show including its artist and venue associations.
// If venues or artists slices are provided (non-nil), they replace the existing associations.
// If nil, the existing associations are preserved.
// If isAdmin is true, new venues created during update are automatically verified.
// Returns the updated show response and any artists that became orphaned (0 shows).
func (s *ShowService) UpdateShowWithRelations(
	showID uint,
	req *contracts.UpdateShowRequest,
	venues []contracts.CreateShowVenue,
	artists []contracts.CreateShowArtist,
	isAdmin bool,
) (*contracts.ShowResponse, []contracts.OrphanedArtist, error) {
	if s.db == nil {
		return nil, nil, fmt.Errorf("database not initialized")
	}

	// Validate before the transaction so a bad set_type never partially
	// rebuilds the bill (replaceShowArtists tears the old rows down first).
	if err := validateShowArtistSetTypes(artists); err != nil {
		return nil, nil, err
	}

	updates := showUpdatesToMap(req)
	_, eventDateChanged := updates["event_date"]

	var response *contracts.ShowResponse
	var orphanedArtists []contracts.OrphanedArtist
	err := s.db.Transaction(func(tx *gorm.DB) error {
		show, err := s.updateShowFields(tx, showID, updates)
		if err != nil {
			return err
		}

		venueResponses, err := s.replaceShowVenues(tx, showID, venues, isAdmin)
		if err != nil {
			return err
		}

		artistResponses, orphaned, err := s.replaceShowArtists(tx, showID, artists)
		if err != nil {
			return err
		}
		orphanedArtists = orphaned

		// Re-stamp denormalized (event_date, venue_id) on show_artists
		// whenever the show's event_date, venue set, or artist set may
		// have changed. Idempotent — safe to call when nothing changed.
		// Keeps the partial unique index `shows_artist_venue_eventdate_uniq`
		// in sync with the parent rows (PSY-576).
		if venues != nil || artists != nil || eventDateChanged {
			if err := syncShowArtistDedupColumns(tx, showID); err != nil {
				return fmt.Errorf("failed to sync show_artists dedup columns: %w", err)
			}
		}

		response, err = s.buildUpdatedShowResponse(tx, show, venues, artists, venueResponses, artistResponses)
		return err
	})

	if err != nil {
		return nil, nil, err
	}

	return response, orphanedArtists, nil
}

// updateShowFields verifies the show exists, applies any scalar-field updates,
// and returns the (reloaded) show row. An empty updates map is a no-op write
// (associations-only edits take this path); the show is still loaded so the
// caller can build the response. Returns ErrShowNotFound when the row is
// missing so callers map it to a 404/422.
func (s *ShowService) updateShowFields(tx *gorm.DB, showID uint, updates map[string]interface{}) (*catalogm.Show, error) {
	var show catalogm.Show
	if err := tx.First(&show, showID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrShowNotFound(showID)
		}
		return nil, fmt.Errorf("failed to find show: %w", err)
	}

	if len(updates) > 0 {
		if err := tx.Model(&show).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update show: %w", err)
		}
		// Reload show to get updated values
		if err := tx.First(&show, showID).Error; err != nil {
			return nil, fmt.Errorf("failed to reload show: %w", err)
		}
	}

	return &show, nil
}

// replaceShowVenues teardown-and-rebuilds the show's venue associations when
// venues is non-nil, returning the rebuilt venue responses. A nil venues slice
// means "leave associations untouched" and returns (nil, nil) — the caller
// lazy-loads the existing venues for the response in that case.
func (s *ShowService) replaceShowVenues(tx *gorm.DB, showID uint, venues []contracts.CreateShowVenue, isAdmin bool) ([]contracts.VenueResponse, error) {
	if venues == nil {
		return nil, nil
	}

	// Delete existing show-venue associations
	if err := tx.Where("show_id = ?", showID).Delete(&catalogm.ShowVenue{}).Error; err != nil {
		return nil, fmt.Errorf("failed to delete existing show venues: %w", err)
	}

	// Create new associations (pass admin status for venue verification)
	venueResponses, err := s.associateVenues(tx, showID, venues, isAdmin)
	if err != nil {
		return nil, fmt.Errorf("failed to associate venues: %w", err)
	}
	return venueResponses, nil
}

// replaceShowArtists teardown-and-rebuilds the show's artist associations when
// artists is non-nil, returning the rebuilt artist responses plus any artists
// that became orphaned (left with zero show associations). A nil artists slice
// means "leave associations untouched" and returns (nil, nil, nil) — the caller
// lazy-loads the existing artists for the response in that case.
func (s *ShowService) replaceShowArtists(tx *gorm.DB, showID uint, artists []contracts.CreateShowArtist) ([]contracts.ArtistResponse, []contracts.OrphanedArtist, error) {
	if artists == nil {
		return nil, nil, nil
	}

	// Capture old artist IDs before deleting associations
	var oldShowArtists []catalogm.ShowArtist
	if err := tx.Where("show_id = ?", showID).Find(&oldShowArtists).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to fetch old show artists: %w", err)
	}
	oldArtistIDs := make(map[uint]bool)
	for _, sa := range oldShowArtists {
		oldArtistIDs[sa.ArtistID] = true
	}

	// Delete existing show-artist associations
	if err := tx.Where("show_id = ?", showID).Delete(&catalogm.ShowArtist{}).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to delete existing show artists: %w", err)
	}

	// Create new associations
	artistResponses, err := s.associateArtists(tx, showID, artists)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to associate artists: %w", err)
	}

	// Build set of new artist IDs
	newArtistIDs := make(map[uint]bool)
	for _, ar := range artistResponses {
		newArtistIDs[ar.ID] = true
	}

	// Check which old artists are no longer associated with ANY show
	var orphanedArtists []contracts.OrphanedArtist
	for oldID := range oldArtistIDs {
		if newArtistIDs[oldID] {
			continue // still associated with this show
		}
		var count int64
		if err := tx.Model(&catalogm.ShowArtist{}).Where("artist_id = ?", oldID).Count(&count).Error; err != nil {
			return nil, nil, fmt.Errorf("failed to count artist associations: %w", err)
		}
		if count == 0 {
			var artist catalogm.Artist
			if err := tx.First(&artist, oldID).Error; err == nil {
				slug := ""
				if artist.Slug != nil {
					slug = *artist.Slug
				}
				orphanedArtists = append(orphanedArtists, contracts.OrphanedArtist{
					ID:   artist.ID,
					Name: artist.Name,
					Slug: slug,
				})
			}
		}
	}

	return artistResponses, orphanedArtists, nil
}

// buildUpdatedShowResponse assembles the ShowResponse from the updated show
// row and its associations. venueResponses / artistResponses carry the rebuilt
// associations when the corresponding slice was provided; when it was nil the
// existing associations are lazy-loaded here so the response is always
// complete.
func (s *ShowService) buildUpdatedShowResponse(
	tx *gorm.DB,
	show *catalogm.Show,
	venues []contracts.CreateShowVenue,
	artists []contracts.CreateShowArtist,
	venueResponses []contracts.VenueResponse,
	artistResponses []contracts.ArtistResponse,
) (*contracts.ShowResponse, error) {
	if venues == nil {
		loaded, err := s.loadShowVenueResponses(tx, show.ID)
		if err != nil {
			return nil, err
		}
		venueResponses = loaded
	}

	if artists == nil {
		loaded, err := s.loadShowArtistResponses(tx, show.ID)
		if err != nil {
			return nil, err
		}
		artistResponses = loaded
	}

	return &contracts.ShowResponse{
		ID:              show.ID,
		Title:           show.Title,
		EventDate:       show.EventDate,
		DoorsAt:         show.DoorsAt,
		MusicAt:         show.MusicAt,
		City:            show.City,
		State:           show.State,
		Price:           show.Price,
		AgeRequirement:  show.AgeRequirement,
		Description:     show.Description,
		TicketURL:       show.TicketURL,
		ImageURL:        show.ImageURL,
		Status:          string(show.Status),
		SubmittedBy:     show.SubmittedBy,
		RejectionReason: show.RejectionReason,
		Venues:          venueResponses,
		Artists:         artistResponses,
		CreatedAt:       show.CreatedAt,
		UpdatedAt:       show.UpdatedAt,
	}, nil
}

// loadShowVenueResponses batch-loads the existing venue associations for a show
// and maps them to VenueResponse, hiding the address of unverified venues.
func (s *ShowService) loadShowVenueResponses(tx *gorm.DB, showID uint) ([]contracts.VenueResponse, error) {
	var showVenues []catalogm.ShowVenue
	if err := tx.Where("show_id = ?", showID).Find(&showVenues).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch show venues: %w", err)
	}
	if len(showVenues) == 0 {
		return nil, nil
	}

	venueIDs := make([]uint, len(showVenues))
	for i, sv := range showVenues {
		venueIDs[i] = sv.VenueID
	}
	var venueModels []catalogm.Venue
	if err := tx.Where("id IN ?", venueIDs).Find(&venueModels).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch venues: %w", err)
	}
	venueMap := make(map[uint]catalogm.Venue, len(venueModels))
	for _, v := range venueModels {
		venueMap[v.ID] = v
	}

	var venueResponses []contracts.VenueResponse
	for _, sv := range showVenues {
		if venue, ok := venueMap[sv.VenueID]; ok {
			// Slug was omitted here while both peer builders resolve it, so an
			// update response carried venues[].slug == "" against a contract
			// that declares it non-nullable, degrading venue links to plain
			// text for consumers of that response.
			venueSlug := ""
			if venue.Slug != nil {
				venueSlug = *venue.Slug
			}
			venueResponses = append(venueResponses, contracts.VenueResponse{
				ID:        venue.ID,
				Slug:      venueSlug,
				Name:      venue.Name,
				Address:   venue.PublicAddress(),
				City:      venue.City,
				State:     venue.State,
				Timezone:  venue.Timezone,
				Capacity:  venue.Capacity,
				AgePolicy: venue.AgePolicy,
				Verified:  venue.Verified,
			})
		}
	}
	return venueResponses, nil
}

// loadShowArtistResponses batch-loads the existing artist associations for a
// show in position order and maps them to ArtistResponse.
func (s *ShowService) loadShowArtistResponses(tx *gorm.DB, showID uint) ([]contracts.ArtistResponse, error) {
	var showArtists []catalogm.ShowArtist
	if err := tx.Where("show_id = ?", showID).Order("position ASC").Find(&showArtists).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch show artists: %w", err)
	}
	if len(showArtists) == 0 {
		return nil, nil
	}

	artistIDs := make([]uint, len(showArtists))
	for i, sa := range showArtists {
		artistIDs[i] = sa.ArtistID
	}
	var artistModels []catalogm.Artist
	if err := tx.Where("id IN ?", artistIDs).Find(&artistModels).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch artists: %w", err)
	}
	artistMap := make(map[uint]catalogm.Artist, len(artistModels))
	for _, a := range artistModels {
		artistMap[a.ID] = a
	}

	var artistResponses []contracts.ArtistResponse
	for _, sa := range showArtists {
		if artist, ok := artistMap[sa.ArtistID]; ok {
			isHeadliner := sa.SetType == "headliner"
			isNewArtist := false
			socials := contracts.ShowArtistSocials{
				Instagram:  artist.Social.Instagram,
				Facebook:   artist.Social.Facebook,
				Twitter:    artist.Social.Twitter,
				YouTube:    artist.Social.YouTube,
				Spotify:    artist.Social.Spotify,
				SoundCloud: artist.Social.SoundCloud,
				Bandcamp:   artist.Social.Bandcamp,
				Website:    artist.Social.Website,
			}
			artistResponses = append(artistResponses, contracts.ArtistResponse{
				ID:               artist.ID,
				Name:             artist.Name,
				State:            artist.State,
				City:             artist.City,
				Country:          artist.Country,
				IsHeadliner:      &isHeadliner,
				SetType:          sa.SetType,
				Position:         sa.Position,
				IsNewArtist:      &isNewArtist,
				BandcampEmbedURL: artist.BandcampEmbedURL,
				Socials:          socials,
			})
		}
	}
	return artistResponses, nil
}

// syncShowArtistDedupColumns stamps the denormalized event_date + venue_id
// on every show_artists row for a show so the partial unique index
// `shows_artist_venue_eventdate_uniq` covers them. Mirrors the
// 20260512023704 backfill migration: picks the lowest venue_id when a
// show has multiple show_venues rows (deterministic, matches the
// migration's LATERAL subquery tiebreaker). Idempotent — safe to call
// after Create as well as after any Update that touches the show's
// event_date or venue associations.
//
// The partial unique index is on (artist_id, venue_id, event_date)
// WHERE event_date IS NOT NULL AND venue_id IS NOT NULL, so a row left
// with NULL denorm columns inserts but is not covered by the
// constraint. The application advisory-lock pre-check in
// checkDuplicateHeadlinerConflicts continues to provide a user-friendly
// duplicate error before the index ever fires (PSY-576).
//
// Package-level so the show-dedup merge path (show_dedup.go) can call
// it too after re-pointing show_artists rows between merged shows.
func syncShowArtistDedupColumns(tx *gorm.DB, showID uint) error {
	return tx.Exec(`
		UPDATE show_artists sa
		SET event_date = s.event_date,
		    venue_id   = pv.venue_id
		FROM shows s
		JOIN LATERAL (
		    SELECT venue_id
		    FROM show_venues
		    WHERE show_id = s.id
		    ORDER BY venue_id
		    LIMIT 1
		) pv ON TRUE
		WHERE sa.show_id = s.id
		  AND s.id = ?
	`, showID).Error
}

// encodeCursor creates a cursor from event_date and show ID
func encodeCursor(eventDate time.Time, id uint) string {
	// Format: base64(timestamp_unix_nano:id)
	cursor := fmt.Sprintf("%d:%d", eventDate.UnixNano(), id)
	return base64.URLEncoding.EncodeToString([]byte(cursor))
}

// decodeCursor parses a cursor into event_date and show ID
func decodeCursor(cursor string) (time.Time, uint, error) {
	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor encoding: %w", err)
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid cursor format")
	}

	unixNano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor timestamp: %w", err)
	}

	id, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid cursor id: %w", err)
	}

	return time.Unix(0, unixNano), uint(id), nil
}

// contracts.CityStateFilter represents a city+state pair for multi-city filtering

// GetUpcomingShows retrieves the shows whose venue-local calendar day has not
// yet passed, with cursor pagination.
//
// "Today" is each show's OWN venue-local day (shared.VenueTZJoin), not one
// boundary shared by the whole page and emphatically not the reader's. A
// Phoenix date therefore stays listed until midnight in Phoenix however far away
// the reader is, and the canonical list is the same list for every visitor —
// which is what lets /shows be server-rendered once and hydrate without a
// discarding refetch (PSY-1678).
//
// If includeNonApproved is false, only approved shows are returned (public
// view). If includeNonApproved is true, all shows are returned including
// pending/rejected (admin view). Optional filters can be provided to filter by
// city, state and tags. Returns shows, next cursor (nil if no more), the
// filter-aware total size of the full matching set (independent of the page
// cursor), and error.
//
// Deprecated parameter: timezone is accepted and ignored, matching
// GetShowsForArtist / GetShowsForVenue. It used to place the boundary in the
// CALLER's zone, which made the same show upcoming for one reader and past for
// another. Kept in the signature because removing it is a breaking change for
// every caller. Do not add new callers that pass a meaningful value.
func (s *ShowService) GetUpcomingShows(timezone string, cursor string, limit int, includeNonApproved bool, filters *contracts.UpcomingShowsFilter) ([]*contracts.ShowResponse, *string, int64, error) {
	if s.db == nil {
		return nil, nil, 0, fmt.Errorf("database not initialized")
	}

	applyUpcomingFilters := func(query *gorm.DB) *gorm.DB {
		// Every predicate below is table-qualified. The venue-local lateral
		// aliases its own columns (shared.VenueTZJoin) so none of them can
		// collide with a `shows` column, which means this is hygiene rather than
		// load-bearing. It is still worth doing: this query now spans two
		// relations, and a reader should not have to know the lateral's
		// projection to tell which one a bare column came from.

		// Filter by status for non-admin users (public view shows only approved)
		if !includeNonApproved {
			query = query.Where("shows.status = ?", catalogm.ShowStatusApproved)
		} else {
			// For admin view, still exclude private shows (those are personal to the submitter)
			query = query.Where("shows.status != ?", catalogm.ShowStatusPrivate)
		}

		// Apply city/state filters if provided
		if filters != nil {
			if len(filters.Cities) > 0 {
				// Multi-city filter: (city = ? AND state = ?) OR ...
				conditions := s.db
				for i, cs := range filters.Cities {
					if i == 0 {
						conditions = conditions.Where("(shows.city = ? AND shows.state = ?)", cs.City, cs.State)
					} else {
						conditions = conditions.Or("(shows.city = ? AND shows.state = ?)", cs.City, cs.State)
					}
				}
				query = query.Where(conditions)
			} else {
				// Legacy single-city filter
				if filters.City != "" {
					query = query.Where("shows.city = ?", filters.City)
				}
				if filters.State != "" {
					query = query.Where("shows.state = ?", filters.State)
				}
			}
			if len(filters.TagSlugs) > 0 {
				// PSY-499: Transitive artist-based tag filtering — shows match when
				// any billed artist has the tag. Direct `entity_type='show'` tags
				// are ignored because shows are not directly tagged with genres.
				query = ApplyTransitiveArtistTagFilter(
					query, s.db,
					"show_artists", "show_id", "artist_id",
					"shows.id",
					TagFilter{
						TagSlugs: filters.TagSlugs,
						MatchAny: filters.TagMatchAny,
					},
				)
			}
		}

		// Partition on each show's own venue-local calendar day.
		return query.
			Joins(shared.VenueTZJoin).
			Where(shared.VenueLocalDateCondition("upcoming"))
	}

	// Filter-aware total for the full matching set (ignores the page cursor).
	var total int64
	countQuery := applyUpcomingFilters(s.db.Model(&catalogm.Show{}))
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, nil, 0, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	// Build page query. `shows.*` is explicit so the lateral's columns cannot
	// widen the projection: shared.VenueTZJoin's aliases match no Show field, so
	// GORM would ignore them anyway, but naming the source relation keeps that
	// true if the lateral ever projects something else.
	query := applyUpcomingFilters(s.db.Preload("Venues").Preload("Artists").Select("shows.*"))

	// Apply cursor filter if provided (narrows the page, not the total)
	if cursor != "" {
		cursorDate, cursorID, err := decodeCursor(cursor)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("invalid cursor: %w", err)
		}
		// Get shows after the cursor position (same date but higher ID, or later date)
		query = query.Where(
			"(shows.event_date = ? AND shows.id > ?) OR (shows.event_date > ?)",
			cursorDate, cursorID, cursorDate,
		)
	}

	// Order by event_date ASC, then by ID ASC for stable ordering
	// Fetch one extra to check if there are more results
	query = query.Order("shows.event_date ASC, shows.id ASC").Limit(limit + 1)

	var shows []catalogm.Show
	if err := query.Find(&shows).Error; err != nil {
		return nil, nil, 0, fmt.Errorf("failed to get upcoming shows: %w", err)
	}

	// Check if there are more results
	var nextCursor *string
	if len(shows) > limit {
		// There are more results, create cursor from the last item we'll return
		shows = shows[:limit] // Trim to requested limit
		lastShow := shows[len(shows)-1]
		encoded := encodeCursor(lastShow.EventDate, lastShow.ID)
		nextCursor = &encoded
	}

	// Build responses
	responses := make([]*contracts.ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, nextCursor, total, nil
}

// GetShowCities retrieves cities that have upcoming approved shows, with counts.
// Returns cities sorted by show count (descending).
//
// "Upcoming" is the SAME venue-local partition GetUpcomingShows lists, which is
// what stops the picker from offering a city whose count then dead-ends at an
// empty list (or hiding one that has shows). The two must be changed together.
//
// Deprecated parameter: timezone is accepted and ignored — see GetUpcomingShows.
func (s *ShowService) GetShowCities(timezone string) ([]contracts.ShowCityResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var results []contracts.ShowCityResponse

	// Table-qualified throughout, and the SELECT aliases back to the bare names
	// `contracts.ShowCityResponse` scans into. The venue-local lateral aliases
	// its own columns, so nothing here is ambiguous; the qualification says
	// which relation each column came from now that the query spans two.
	err := s.db.Model(&catalogm.Show{}).
		Select("shows.city AS city, shows.state AS state, COUNT(*) AS show_count").
		Joins(shared.VenueTZJoin).
		Where("shows.status = ?", catalogm.ShowStatusApproved).
		Where(shared.VenueLocalDateCondition("upcoming")).
		Where("shows.city IS NOT NULL AND shows.city != ''").
		Where("shows.state IS NOT NULL AND shows.state != ''").
		Group("shows.city, shows.state").
		Order("show_count DESC, city ASC").
		Scan(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get show cities: %w", err)
	}

	// Attach each city's geocoded centroid (PSY-981). We resolve from the
	// (city, state) pair via the SAME offline geocoder PSY-985 uses for venue
	// coordinates, rather than joining show_venues -> venues: it's exact to the
	// (city, state) key we already group by (no venue-vs-show city-spelling
	// mismatch), it has no dependency on every venue row being backfilled, and
	// it's an in-memory lookup (no extra query, no N+1). A miss leaves both
	// coords nil — the frontend then falls back to exact city-name matching.
	if s.geocoder != nil {
		for i := range results {
			lat, lng, _ := geo.LookupPointers(s.geocoder, results[i].City, results[i].State, "")
			results[i].Latitude = lat
			results[i].Longitude = lng
		}
	}

	return results, nil
}

// SearchShows returns up to 20 shows whose title or any bill artist name matches
// the query (case-insensitive ILIKE substring), ordered by event_date DESC.
//
// Match target: shows.title OR any artists.name on the bill (joined through
// show_artists). User explicitly chose this permissive shape for PSY-372: users
// think of shows by artist first. PSY-520.
//
// Headliner resolution: there is no `is_headliner` column on show_artists.
// Headliner = the show_artists row with set_type = 'headliner', falling back
// to position = 0. This mirrors checkDuplicateHeadlinerConflicts above.
//
// Venue resolution: a show may have multiple venues; we pick the
// lowest-numbered venue_id deterministically as the "primary" — venues array
// ordering isn't currently tracked in show_venues.
//
// Empty query returns []*ShowSearchResult{}, not all shows.
//
// Mirrors SearchFestivals/SearchReleases: no status filter (those don't
// constrain to approved either), simple LIMIT 20, no relevance ranking.
func (s *ShowService) SearchShows(query string) ([]*contracts.ShowSearchResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Empty / whitespace-only queries return no results — never "all shows".
	// The handler also short-circuits before calling here as a defensive
	// boundary; this keeps the service safe if it's ever called directly
	// (e.g. from tests or future internal callers).
	if strings.TrimSpace(query) == "" {
		return []*contracts.ShowSearchResult{}, nil
	}

	pattern := shared.LikePattern(query)

	// Single query: select shows whose title matches OR any artist on the
	// bill matches, then resolve headliner/venue via correlated subqueries.
	// DISTINCT on shows.id prevents duplicate rows when both title and
	// multiple artists match.
	rows, err := s.db.Raw(`
		SELECT DISTINCT ON (shows.id)
			shows.id,
			shows.slug,
			shows.title,
			shows.event_date,
			COALESCE(
				(SELECT a.name
				 FROM show_artists sa
				 JOIN artists a ON a.id = sa.artist_id
				 WHERE sa.show_id = shows.id
				   AND sa.set_type = 'headliner'
				 ORDER BY sa.position ASC
				 LIMIT 1),
				(SELECT a.name
				 FROM show_artists sa
				 JOIN artists a ON a.id = sa.artist_id
				 WHERE sa.show_id = shows.id
				 ORDER BY sa.position ASC
				 LIMIT 1),
				''
			) AS headliner_name,
			COALESCE(
				(SELECT v.name
				 FROM show_venues sv
				 JOIN venues v ON v.id = sv.venue_id
				 WHERE sv.show_id = shows.id
				 ORDER BY sv.venue_id ASC
				 LIMIT 1),
				''
			) AS venue_name
		FROM shows
		LEFT JOIN show_artists sa_match ON sa_match.show_id = shows.id
		LEFT JOIN artists a_match ON a_match.id = sa_match.artist_id
		WHERE shows.title ILIKE ?
		   OR a_match.name ILIKE ?
	`, pattern, pattern).Rows()

	if err != nil {
		return nil, fmt.Errorf("failed to search shows: %w", err)
	}
	defer rows.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	// Two-stage scan: DISTINCT ON requires its expression(s) to lead the
	// ORDER BY. To order results by event_date DESC across the de-duplicated
	// set, materialize the matched rows first then sort + truncate in Go.
	type matchedRow struct {
		ID            uint
		Slug          *string
		Title         string
		EventDate     time.Time
		HeadlinerName string
		VenueName     string
	}
	var matched []matchedRow
	for rows.Next() {
		var r matchedRow
		if err := rows.Scan(&r.ID, &r.Slug, &r.Title, &r.EventDate, &r.HeadlinerName, &r.VenueName); err != nil {
			return nil, fmt.Errorf("failed to scan show search row: %w", err)
		}
		matched = append(matched, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate show search rows: %w", err)
	}

	// Sort by event_date DESC (most-recent first); ties broken by ID DESC for
	// deterministic ordering.
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].EventDate.Equal(matched[j].EventDate) {
			return matched[i].ID > matched[j].ID
		}
		return matched[i].EventDate.After(matched[j].EventDate)
	})

	limit := 20
	if len(matched) < limit {
		limit = len(matched)
	}

	results := make([]*contracts.ShowSearchResult, 0, limit)
	for i := 0; i < limit; i++ {
		r := matched[i]
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		results = append(results, &contracts.ShowSearchResult{
			ID:            r.ID,
			Slug:          slug,
			Title:         r.Title,
			HeadlinerName: r.HeadlinerName,
			VenueName:     r.VenueName,
			EventDate:     r.EventDate,
		})
	}

	return results, nil
}

// DeleteShow deletes a show and its associations
func (s *ShowService) DeleteShow(showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Polymorphic bookmarks have no FK to shows. Remove every action for
		// this entity inside the same transaction so saved-show totals cannot
		// retain a dangling row after deletion.
		if err := tx.Where(
			"entity_type = ? AND entity_id = ?",
			engagementm.BookmarkEntityShow,
			showID,
		).Delete(&engagementm.UserBookmark{}).Error; err != nil {
			return fmt.Errorf("failed to delete show bookmarks: %w", err)
		}

		// Delete show associations first (cascade will handle this, but being explicit)
		if err := tx.Where("show_id = ?", showID).Delete(&catalogm.ShowVenue{}).Error; err != nil {
			return fmt.Errorf("failed to delete show venues: %w", err)
		}
		if err := tx.Where("show_id = ?", showID).Delete(&catalogm.ShowArtist{}).Error; err != nil {
			return fmt.Errorf("failed to delete show artists: %w", err)
		}

		// Delete the show
		if err := tx.Delete(&catalogm.Show{}, showID).Error; err != nil {
			return fmt.Errorf("failed to delete show: %w", err)
		}

		return nil
	})
}

// GetPendingShows retrieves shows with pending status for admin review.
// Returns shows, total count, and error.
func (s *ShowService) GetPendingShows(limit, offset int, filters *contracts.PendingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Build base query with optional filters
	countQuery := s.db.Model(&catalogm.Show{}).Where("status = ?", catalogm.ShowStatusPending)
	if filters != nil {
		if filters.VenueID != nil {
			countQuery = countQuery.Joins("JOIN show_venues ON shows.id = show_venues.show_id").
				Where("show_venues.venue_id = ?", *filters.VenueID)
		}
		if filters.Source != nil {
			countQuery = countQuery.Where("source = ?", *filters.Source)
		}
	}

	// Get total count
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count pending shows: %w", err)
	}

	// Get pending shows with pagination
	findQuery := s.db.Preload("Venues").Preload("Artists").
		Where("status = ?", catalogm.ShowStatusPending)
	if filters != nil {
		if filters.VenueID != nil {
			findQuery = findQuery.Joins("JOIN show_venues ON shows.id = show_venues.show_id").
				Where("show_venues.venue_id = ?", *filters.VenueID)
		}
		if filters.Source != nil {
			findQuery = findQuery.Where("source = ?", *filters.Source)
		}
	}

	var shows []catalogm.Show
	err := findQuery.
		Order("source_venue ASC NULLS LAST, event_date ASC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get pending shows: %w", err)
	}

	// Build responses
	responses := make([]*contracts.ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, total, nil
}

// GetRejectedShows retrieves shows with rejected status for admin reference.
// Supports optional search by title or rejection reason.
// Returns shows, total count, and error.
func (s *ShowService) GetRejectedShows(limit, offset int, search string) ([]*contracts.ShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Build base query
	baseQuery := s.db.Model(&catalogm.Show{}).Where("status = ?", catalogm.ShowStatusRejected)

	// Add search filter if provided
	if search != "" {
		searchPattern := shared.LikePattern(search)
		baseQuery = baseQuery.Where("title ILIKE ? OR rejection_reason ILIKE ?", searchPattern, searchPattern)
	}

	// Get total count
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count rejected shows: %w", err)
	}

	// Get rejected shows with pagination
	var shows []catalogm.Show
	err := s.db.Preload("Venues").Preload("Artists").
		Where("status = ?", catalogm.ShowStatusRejected).
		Scopes(func(db *gorm.DB) *gorm.DB {
			if search != "" {
				searchPattern := shared.LikePattern(search)
				return db.Where("title ILIKE ? OR rejection_reason ILIKE ?", searchPattern, searchPattern)
			}
			return db
		}).
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get rejected shows: %w", err)
	}

	// Build responses
	responses := make([]*contracts.ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, total, nil
}

// ApproveShow approves a pending show and optionally verifies its venues.
func (s *ShowService) ApproveShow(showID uint, verifyVenues bool) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *contracts.ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show
		var show catalogm.Show
		if err := tx.Preload("Venues").First(&show, showID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrShowNotFound(showID)
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is pending or rejected
		if show.Status != catalogm.ShowStatusPending && show.Status != catalogm.ShowStatusRejected {
			return fmt.Errorf("show cannot be approved (current status: %s)", show.Status)
		}

		// Update show status to approved (clear rejection reason if previously rejected)
		updates := map[string]interface{}{
			"status":           catalogm.ShowStatusApproved,
			"rejection_reason": "",
		}
		if err := tx.Model(&show).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to approve show: %w", err)
		}

		// Optionally verify the venues
		if verifyVenues {
			for _, venue := range show.Venues {
				if !venue.Verified {
					if err := tx.Model(&venue).Update("verified", true).Error; err != nil {
						return fmt.Errorf("failed to verify venue %d: %w", venue.ID, err)
					}
				}
			}
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// RejectShow rejects a pending show with a reason.
func (s *ShowService) RejectShow(showID uint, reason string) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *contracts.ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show
		var show catalogm.Show
		if err := tx.First(&show, showID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrShowNotFound(showID)
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is pending
		if show.Status != catalogm.ShowStatusPending {
			return fmt.Errorf("show is not pending (current status: %s)", show.Status)
		}

		// Update show status to rejected with reason
		updates := map[string]interface{}{
			"status":           catalogm.ShowStatusRejected,
			"rejection_reason": reason,
		}
		if err := tx.Model(&show).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to reject show: %w", err)
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// BatchApproveShows approves multiple pending shows at once.
func (s *ShowService) BatchApproveShows(showIDs []uint) (*contracts.BatchShowResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &contracts.BatchShowResult{
		Succeeded: make([]uint, 0),
		Errors:    make([]contracts.BatchShowError, 0),
	}

	for _, id := range showIDs {
		_, err := s.ApproveShow(id, false)
		if err != nil {
			result.Errors = append(result.Errors, contracts.BatchShowError{ShowID: id, Error: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}

	return result, nil
}

// BatchRejectShows rejects multiple pending shows at once with a reason and category.
func (s *ShowService) BatchRejectShows(showIDs []uint, reason string, category string) (*contracts.BatchShowResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &contracts.BatchShowResult{
		Succeeded: make([]uint, 0),
		Errors:    make([]contracts.BatchShowError, 0),
	}

	for _, id := range showIDs {
		_, err := s.RejectShow(id, reason)
		if err != nil {
			result.Errors = append(result.Errors, contracts.BatchShowError{ShowID: id, Error: err.Error()})
		} else {
			// Update rejection_category separately since RejectShow doesn't handle it
			if category != "" {
				s.db.Model(&catalogm.Show{}).Where("id = ?", id).Update("rejection_category", category)
			}
			result.Succeeded = append(result.Succeeded, id)
		}
	}

	return result, nil
}

// UnpublishShow changes an approved show's status back to pending.
// Only the submitter or an admin can unpublish a show.
func (s *ShowService) UnpublishShow(showID uint, userID uint, isAdmin bool) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *contracts.ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show
		var show catalogm.Show
		if err := tx.First(&show, showID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrShowNotFound(showID)
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is approved (can only unpublish approved shows)
		if show.Status != catalogm.ShowStatusApproved {
			return fmt.Errorf("can only unpublish approved shows (current status: %s)", show.Status)
		}

		// Check authorization: user must be the submitter or an admin
		if !isAdmin {
			if show.SubmittedBy == nil || *show.SubmittedBy != userID {
				return apperrors.ErrShowUnpublishUnauthorized(showID)
			}
		}

		// Update show status to private
		if err := tx.Model(&show).Update("status", catalogm.ShowStatusPrivate).Error; err != nil {
			return fmt.Errorf("failed to unpublish show: %w", err)
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// MakePrivateShow changes a pending show's status to private.
// Only the submitter or an admin can make a show private.
func (s *ShowService) MakePrivateShow(showID uint, userID uint, isAdmin bool) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *contracts.ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show
		var show catalogm.Show
		if err := tx.First(&show, showID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrShowNotFound(showID)
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is pending (can only make private from pending status)
		if show.Status != catalogm.ShowStatusPending {
			return fmt.Errorf("can only make pending shows private (current status: %s)", show.Status)
		}

		// Check authorization: user must be the submitter or an admin
		if !isAdmin {
			if show.SubmittedBy == nil || *show.SubmittedBy != userID {
				return apperrors.ErrShowMakePrivateUnauthorized(showID)
			}
		}

		// Update show status to private
		if err := tx.Model(&show).Update("status", catalogm.ShowStatusPrivate).Error; err != nil {
			return fmt.Errorf("failed to make show private: %w", err)
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// PublishShow changes a private show's status to approved.
// Shows are always approved regardless of venue verification status.
// Unverified venues will display city-only until verified by an admin.
// Only the submitter or an admin can publish a show.
func (s *ShowService) PublishShow(showID uint, userID uint, isAdmin bool) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var response *contracts.ShowResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Get the show with venues preloaded
		var show catalogm.Show
		if err := tx.Preload("Venues").First(&show, showID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrShowNotFound(showID)
			}
			return fmt.Errorf("failed to get show: %w", err)
		}

		// Verify the show is private (can only publish from private status)
		if show.Status != catalogm.ShowStatusPrivate {
			return fmt.Errorf("can only publish private shows (current status: %s)", show.Status)
		}

		// Check authorization: user must be the submitter or an admin
		if !isAdmin {
			if show.SubmittedBy == nil || *show.SubmittedBy != userID {
				return apperrors.ErrShowPublishUnauthorized(showID)
			}
		}

		// Always set status to approved - unverified venues show city-only until verified
		if err := tx.Model(&show).Update("status", catalogm.ShowStatusApproved).Error; err != nil {
			return fmt.Errorf("failed to publish show: %w", err)
		}

		// Reload the show to get updated data
		if err := tx.Preload("Venues").Preload("Artists").First(&show, showID).Error; err != nil {
			return fmt.Errorf("failed to reload show: %w", err)
		}

		response = s.buildShowResponse(&show)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// associateVenues associates venues with a show, creating new venues if needed.
// Uses VenueService to ensure consistent venue creation logic.
// If isAdmin is true, new venues are automatically verified.
func (s *ShowService) associateVenues(tx *gorm.DB, showID uint, requestVenues []contracts.CreateShowVenue, isAdmin bool) ([]contracts.VenueResponse, error) {
	var venues []contracts.VenueResponse

	// Create venue service for venue operations
	venueService := NewVenueService(s.db)

	for _, requestVenue := range requestVenues {
		var venue *catalogm.Venue
		var err error
		isNewVenue := false

		// If ID is provided, try to find existing venue by ID
		if requestVenue.ID != nil {
			var venueModel catalogm.Venue
			err = tx.First(&venueModel, *requestVenue.ID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("venue with ID %d not found", *requestVenue.ID)
			} else if err != nil {
				return nil, fmt.Errorf("failed to find venue with ID %d: %w", *requestVenue.ID, err)
			}
			venue = &venueModel
		} else {
			// No ID provided, use VenueService to find or create venue (name unique per city)
			// VenueService will validate required fields
			var addressPtr *string
			if requestVenue.Address != "" {
				addressPtr = &requestVenue.Address
			}

			venue, isNewVenue, err = venueService.FindOrCreateVenue(
				requestVenue.Name,
				requestVenue.City,
				requestVenue.State,
				addressPtr,
				nil,     // zipcode
				tx,      // use transaction
				isAdmin, // pass admin status for venue verification
			)
			if err != nil {
				return nil, fmt.Errorf("failed to find or create venue: %w", err)
			}
		}

		// Create show-venue association
		showVenue := catalogm.ShowVenue{
			ShowID:  showID,
			VenueID: venue.ID,
		}
		if err := tx.Create(&showVenue).Error; err != nil {
			return nil, fmt.Errorf("failed to create show-venue association: %w", err)
		}

		venueSlug := ""
		if venue.Slug != nil {
			venueSlug = *venue.Slug
		}

		venues = append(venues, contracts.VenueResponse{
			ID:         venue.ID,
			Slug:       venueSlug,
			Name:       venue.Name,
			Address:    venue.PublicAddress(),
			City:       venue.City,
			State:      venue.State,
			Timezone:   venue.Timezone,
			Capacity:   venue.Capacity,
			AgePolicy:  venue.AgePolicy,
			Verified:   venue.Verified,
			IsNewVenue: &isNewVenue,
		})
	}

	return venues, nil
}

// curatedSetType returns the caller's explicitly curated set_type, or "" when
// the caller did not curate this act's slot. Whitespace-only counts as absent.
// The value is returned verbatim, NOT coerced: validateShowArtistSetTypes is
// the single place that decides whether a stated role is acceptable, so this
// helper cannot quietly turn a rejected value into an accepted one.
func curatedSetType(a contracts.CreateShowArtist) string {
	value := derefString(a.SetType)
	if strings.TrimSpace(value) == "" {
		return ""
	}
	// Deliberately NOT trimmed. Only emptiness is forgiving; the value itself
	// is judged exactly as sent, so an in-process caller gets the same verdict
	// on " headliner " that the OpenAPI enum gives an HTTP caller. Trimming
	// here would make the service quietly laxer than its published contract.
	return value
}

// validateShowArtistSetTypes rejects any entry whose curated set_type is
// outside the vocabulary.
//
// Boundary validation: callers run it once, before opening the write
// transaction, so a malformed bill fails whole rather than writing some rows
// and rolling back. Everything downstream may then trust the value.
func validateShowArtistSetTypes(artists []contracts.CreateShowArtist) error {
	for i, a := range artists {
		value := curatedSetType(a)
		if value == "" || contracts.IsValidSetType(value) {
			continue
		}
		return apperrors.ErrShowValidationFailed(fmt.Sprintf(
			"artists[%d].set_type %q is not a valid set type (allowed: %s)",
			i, value, contracts.SetTypeVocabularyCSV(),
		))
	}
	return nil
}

// resolveArtistRole decides the (set_type, is_headliner) pair written for one
// act, in strict precedence order:
//
//  1. A curated set_type wins outright; is_headliner is derived from it.
//  2. Otherwise the legacy is_headliner flag decides headliner vs default.
//  3. Otherwise, with NO signal at all, position 0 is taken as the headliner.
//
// Everything that is not the headliner resolves to SetTypeDefault. It
// deliberately does NOT invent a support role from list order: before PSY-1673
// this function stamped "opener" on every non-headliner, which made the column
// unreadable -- an "(opener)" annotation would have fired on essentially every
// support act on the site regardless of what slot they actually played.
//
// An out-of-vocabulary set_type is treated as absent rather than written
// through; validateShowArtistSetTypes is the enforcement point and runs first.
func resolveArtistRole(a contracts.CreateShowArtist, position int) (setType string, isHeadliner bool) {
	if value := curatedSetType(a); contracts.IsValidSetType(value) {
		return value, value == contracts.SetTypeHeadliner
	}
	if a.IsHeadliner != nil {
		if *a.IsHeadliner {
			return contracts.SetTypeHeadliner, true
		}
		return contracts.SetTypeDefault, false
	}
	if position == 0 {
		return contracts.SetTypeHeadliner, true
	}
	return contracts.SetTypeDefault, false
}

// associateArtists associates artists with a show, creating new artists if needed
func (s *ShowService) associateArtists(tx *gorm.DB, showID uint, requestArtists []contracts.CreateShowArtist) ([]contracts.ArtistResponse, error) {
	var artists []contracts.ArtistResponse

	for position, requestArtist := range requestArtists {
		var artist catalogm.Artist
		var err error
		isNewArtist := false

		// If ID is provided, try to find existing artist by ID
		if requestArtist.ID != nil {
			err = tx.First(&artist, *requestArtist.ID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("artist with ID %d not found", *requestArtist.ID)
			} else if err != nil {
				return nil, fmt.Errorf("failed to find artist with ID %d: %w", *requestArtist.ID, err)
			}
		} else {
			// No ID provided, use name to find or create artist
			if requestArtist.Name == "" {
				return nil, fmt.Errorf("either ID or Name must be provided for artist")
			}

			// PSY-1118: validate + normalize the Instagram handle BEFORE the funnel
			// (apply can't return an error) so it can't reach social.instagram
			// un-anchored — canonical https://instagram.com/<handle>, URL-shaped
			// input rejected. Applied only when a NEW artist is created.
			var igNormalized *string
			if requestArtist.InstagramHandle != nil && *requestArtist.InstagramHandle != "" {
				normalized, nerr := utils.NormalizeInstagramHandle(*requestArtist.InstagramHandle)
				if nerr != nil {
					return nil, fmt.Errorf("artist %s: %w", requestArtist.Name, nerr)
				}
				if normalized != "" {
					igNormalized = &normalized
				}
			}
			// Single artist write path (PSY-1254): dedup + unique slug + insert.
			found, created, ferr := FindOrCreateArtistTx(tx, requestArtist.Name, func(a *catalogm.Artist) {
				a.Social.Instagram = igNormalized
			})
			if ferr != nil {
				return nil, ferr
			}
			artist = *found
			isNewArtist = created
		}

		// Determine set type and IsHeadliner flag
		setType, isHeadliner := resolveArtistRole(requestArtist, position)

		// Create show-artist association with position
		showArtist := catalogm.ShowArtist{
			ShowID:   showID,
			ArtistID: artist.ID,
			Position: position,
			SetType:  setType,
		}
		if err := tx.Create(&showArtist).Error; err != nil {
			return nil, fmt.Errorf("failed to create show-artist association: %w", err)
		}

		// Convert artist socials to response format
		socials := contracts.ShowArtistSocials{
			Instagram:  artist.Social.Instagram,
			Facebook:   artist.Social.Facebook,
			Twitter:    artist.Social.Twitter,
			YouTube:    artist.Social.YouTube,
			Spotify:    artist.Social.Spotify,
			SoundCloud: artist.Social.SoundCloud,
			Bandcamp:   artist.Social.Bandcamp,
			Website:    artist.Social.Website,
		}

		artistSlug := ""
		if artist.Slug != nil {
			artistSlug = *artist.Slug
		}
		artists = append(artists, contracts.ArtistResponse{
			ID:               artist.ID,
			Slug:             artistSlug,
			Name:             artist.Name,
			State:            artist.State,
			City:             artist.City,
			Country:          artist.Country,
			IsHeadliner:      &isHeadliner,
			SetType:          setType,
			Position:         position,
			IsNewArtist:      &isNewArtist,
			BandcampEmbedURL: artist.BandcampEmbedURL,
			Socials:          socials,
		})
	}

	return artists, nil
}

// fetchLabelsForArtists batch-loads the labels for a set of artists, keyed by
// artist ID and ordered by label name ASC to match GetLabelsForArtist. Two
// queries regardless of how many artists are asked for. Artists with no labels
// are absent from the returned map, so callers decide what absence means on
// their wire format.
//
// Returns an error rather than an empty map when a query fails, so callers can
// tell "this artist has no labels" apart from "the lookup broke" and avoid
// publishing the second as the first.
//
// A free function, not a ShowService method: nothing here is show-shaped.
//
// Uses the junction model plus an ID lookup rather than a GORM many2many
// preload: Artist declares no Labels association (it lives on Label), and the
// manual pair keeps the query count fixed and inspectable.
//
// Deliberately does NOT filter labels.status: an inactive or defunct label is
// still a true fact about who put the band's records out, and GetLabelsForArtist
// does not filter either, so the show bill and the artist page agree. Status is
// descriptive metadata here, not a visibility gate.
func fetchLabelsForArtists(db *gorm.DB, artistIDs []uint) (map[uint][]contracts.ShowArtistLabel, error) {
	if len(artistIDs) == 0 {
		return nil, nil
	}

	var artistLabels []catalogm.ArtistLabel
	if err := db.Where("artist_id IN ?", artistIDs).Find(&artistLabels).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch artist_labels: %w", err)
	}
	if len(artistLabels) == 0 {
		return nil, nil
	}

	labelIDs := make([]uint, 0, len(artistLabels))
	seen := make(map[uint]struct{}, len(artistLabels))
	for _, al := range artistLabels {
		if _, dup := seen[al.LabelID]; dup {
			continue
		}
		seen[al.LabelID] = struct{}{}
		labelIDs = append(labelIDs, al.LabelID)
	}

	var labels []catalogm.Label
	if err := db.Where("id IN ?", labelIDs).Order("name ASC, id ASC").Find(&labels).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch labels: %w", err)
	}

	// Keep the name-ASC ordering by walking the sorted label list on the outside
	// and fanning each label out to the artists joined to it.
	artistsByLabel := make(map[uint][]uint, len(labelIDs))
	for _, al := range artistLabels {
		artistsByLabel[al.LabelID] = append(artistsByLabel[al.LabelID], al.ArtistID)
	}

	byArtist := make(map[uint][]contracts.ShowArtistLabel, len(artistIDs))
	for _, label := range labels {
		slug := ""
		if label.Slug != nil {
			slug = *label.Slug
		}
		entry := contracts.ShowArtistLabel{ID: label.ID, Name: label.Name, Slug: slug}
		for _, artistID := range artistsByLabel[label.ID] {
			byArtist[artistID] = append(byArtist[artistID], entry)
		}
	}

	return byArtist, nil
}

// attachBillLabels fills in Labels on an already-built show response, in two
// queries for the whole bill.
//
// Deliberately NOT folded into buildShowResponse: that runs once per show
// inside the list endpoints (GetUpcomingShows serves up to 200 shows per
// request), so doing the join there would add two queries per show to the
// hottest public path for a field only the show-detail bill renders. Only the
// detail reads call this; everyone else leaves Labels nil, so omitempty drops
// the key rather than claiming the bill is unsigned.
//
// The cost this does NOT dodge: the non-bill callers that resolve a show
// through GetShow (the per-show .ics feed, the update/delete ownership
// pre-checks, the admin batch-approve notification loop) each pay the two
// queries for a field they never read. Bounded at two queries per single-show
// request, versus the 400 per request that folding this into buildShowResponse
// would have cost the list path, so it is left alone.
func (s *ShowService) attachBillLabels(resp *contracts.ShowResponse) {
	if resp == nil || len(resp.Artists) == 0 {
		return
	}

	artistIDs := make([]uint, len(resp.Artists))
	for i, artist := range resp.Artists {
		artistIDs[i] = artist.ID
	}

	labelsByArtist, err := fetchLabelsForArtists(s.db, artistIDs)
	if err != nil {
		// Leave Labels nil so the key is omitted. Degrading to [] would tell the
		// client every artist on the bill is unsigned, which is a lie the page
		// would render as fact; an absent key just means "not looked up".
		log.Printf("WARN attachBillLabels: show_id=%d: %v", resp.ID, err)
		return
	}

	for i := range resp.Artists {
		// Every artist gets a non-nil slice: this response HAS looked labels up,
		// so an unsigned artist must read as [] rather than as "not fetched".
		labels := labelsByArtist[resp.Artists[i].ID]
		if labels == nil {
			labels = []contracts.ShowArtistLabel{}
		}
		resp.Artists[i].Labels = &labels
	}
}

// buildShowResponse converts a Show model to contracts.ShowResponse
func (s *ShowService) buildShowResponse(show *catalogm.Show) *contracts.ShowResponse {
	// Build venue responses
	venues := make([]contracts.VenueResponse, len(show.Venues))
	for i, venue := range show.Venues {
		venueSlug := ""
		if venue.Slug != nil {
			venueSlug = *venue.Slug
		}
		venues[i] = contracts.VenueResponse{
			ID:       venue.ID,
			Slug:     venueSlug,
			Name:     venue.Name,
			Address:  venue.PublicAddress(),
			City:     venue.City,
			State:    venue.State,
			Timezone: venue.Timezone,
			// Carried so a show consumer does not need a second fetch of the
			// venue endpoint for two scalars. Neither is sensitive, so unlike
			// Address they are served for unverified venues too. See the
			// VenueResponse contract for the house-default vs per-event
			// distinction that governs AgePolicy.
			Capacity:  venue.Capacity,
			AgePolicy: venue.AgePolicy,
			Verified:  venue.Verified,
		}
	}

	// Build artist responses (need to get ordered artists)
	artists := make([]contracts.ArtistResponse, 0, len(show.Artists))

	// Get ordered artists from show_artists table
	var showArtists []catalogm.ShowArtist
	if err := s.db.Where("show_id = ?", show.ID).Order("position ASC").Find(&showArtists).Error; err != nil {
		log.Printf("WARN buildShowResponse: failed to fetch show_artists for show_id=%d: %v", show.ID, err)
	}

	if len(showArtists) > 0 {
		// Batch-fetch all artists in one query
		artistIDs := make([]uint, len(showArtists))
		for i, sa := range showArtists {
			artistIDs[i] = sa.ArtistID
		}

		var allArtists []catalogm.Artist
		if err := s.db.Where("id IN ?", artistIDs).Find(&allArtists).Error; err != nil {
			log.Printf("WARN buildShowResponse: failed to batch-fetch artists for show_id=%d: %v", show.ID, err)
		}

		// Build lookup map
		artistMap := make(map[uint]*catalogm.Artist, len(allArtists))
		for i := range allArtists {
			artistMap[allArtists[i].ID] = &allArtists[i]
		}

		// Iterate in position order
		for _, sa := range showArtists {
			artist, ok := artistMap[sa.ArtistID]
			if !ok {
				continue
			}

			socials := contracts.ShowArtistSocials{
				Instagram:  artist.Social.Instagram,
				Facebook:   artist.Social.Facebook,
				Twitter:    artist.Social.Twitter,
				YouTube:    artist.Social.YouTube,
				Spotify:    artist.Social.Spotify,
				SoundCloud: artist.Social.SoundCloud,
				Bandcamp:   artist.Social.Bandcamp,
				Website:    artist.Social.Website,
			}

			isHeadliner := sa.SetType == "headliner"
			isNewArtist := false

			artistSlug := ""
			if artist.Slug != nil {
				artistSlug = *artist.Slug
			}
			artists = append(artists, contracts.ArtistResponse{
				ID:               artist.ID,
				Slug:             artistSlug,
				Name:             artist.Name,
				State:            artist.State,
				City:             artist.City,
				Country:          artist.Country,
				IsHeadliner:      &isHeadliner,
				SetType:          sa.SetType,
				Position:         sa.Position,
				IsNewArtist:      &isNewArtist,
				BandcampEmbedURL: artist.BandcampEmbedURL,
				Socials:          socials,
			})
		}
	}

	showSlug := ""
	if show.Slug != nil {
		showSlug = *show.Slug
	}
	return &contracts.ShowResponse{
		ID:                show.ID,
		Slug:              showSlug,
		Title:             show.Title,
		EventDate:         show.EventDate,
		DoorsAt:           show.DoorsAt,
		MusicAt:           show.MusicAt,
		City:              show.City,
		State:             show.State,
		Price:             show.Price,
		AgeRequirement:    show.AgeRequirement,
		Description:       show.Description,
		TicketURL:         show.TicketURL,
		ImageURL:          show.ImageURL,
		Status:            string(show.Status),
		SubmittedBy:       show.SubmittedBy,
		RejectionReason:   show.RejectionReason,
		RejectionCategory: show.RejectionCategory,
		Venues:            venues,
		Artists:           artists,
		CreatedAt:         show.CreatedAt,
		UpdatedAt:         show.UpdatedAt,
		IsSoldOut:         show.IsSoldOut,
		IsCancelled:       show.IsCancelled,
		Source:            string(show.Source),
		SourceVenue:       show.SourceVenue,
		ScrapedAt:         show.ScrapedAt,
		DuplicateOfShowID: show.DuplicateOfShowID,
	}
}

// ============================================================================
// Show Export/Import Feature
// ============================================================================

// contracts.ExportShowData represents the show data in the markdown frontmatter

// ExportShowToMarkdown exports a show to markdown format
// Returns the markdown content, suggested filename, and error
func (s *ShowService) ExportShowToMarkdown(showID uint) ([]byte, string, error) {
	if s.db == nil {
		return nil, "", fmt.Errorf("database not initialized")
	}

	// Fetch show with preloaded venues and artists
	var show catalogm.Show
	err := s.db.Preload("Venues").Preload("Artists").First(&show, showID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", apperrors.ErrShowNotFound(showID)
		}
		return nil, "", fmt.Errorf("failed to get show: %w", err)
	}

	// Get ordered show artists from junction table
	var showArtists []catalogm.ShowArtist
	s.db.Where("show_id = ?", show.ID).Order("position ASC").Find(&showArtists)

	// Build frontmatter data
	frontmatter := contracts.ExportFrontmatter{
		Version:    "1.0",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Show: contracts.ExportShowData{
			Title:     show.Title,
			EventDate: show.EventDate.UTC().Format(time.RFC3339),
			Status:    string(show.Status),
		},
	}

	// Add optional show fields
	if show.City != nil {
		frontmatter.Show.City = *show.City
	}
	if show.State != nil {
		frontmatter.Show.State = *show.State
	}
	if show.Price != nil {
		frontmatter.Show.Price = show.Price
	}
	if show.AgeRequirement != nil && *show.AgeRequirement != "" {
		frontmatter.Show.AgeRequirement = *show.AgeRequirement
	}

	// Build venues
	for _, venue := range show.Venues {
		venueData := contracts.ExportVenueData{
			Name:  venue.Name,
			City:  venue.City,
			State: venue.State,
		}
		if venue.Address != nil {
			venueData.Address = *venue.Address
		}
		if venue.Zipcode != nil {
			venueData.Zipcode = *venue.Zipcode
		}
		// Add social links
		if venue.Social.Instagram != nil {
			venueData.Social.Instagram = *venue.Social.Instagram
		}
		if venue.Social.Facebook != nil {
			venueData.Social.Facebook = *venue.Social.Facebook
		}
		if venue.Social.Twitter != nil {
			venueData.Social.Twitter = *venue.Social.Twitter
		}
		if venue.Social.YouTube != nil {
			venueData.Social.YouTube = *venue.Social.YouTube
		}
		if venue.Social.Spotify != nil {
			venueData.Social.Spotify = *venue.Social.Spotify
		}
		if venue.Social.SoundCloud != nil {
			venueData.Social.SoundCloud = *venue.Social.SoundCloud
		}
		if venue.Social.Bandcamp != nil {
			venueData.Social.Bandcamp = *venue.Social.Bandcamp
		}
		if venue.Social.Website != nil {
			venueData.Social.Website = *venue.Social.Website
		}
		frontmatter.Venues = append(frontmatter.Venues, venueData)
	}

	// Batch-fetch all artists
	artistIDs := make([]uint, len(showArtists))
	for i, sa := range showArtists {
		artistIDs[i] = sa.ArtistID
	}
	var allArtists []catalogm.Artist
	s.db.Where("id IN ?", artistIDs).Find(&allArtists)
	artistMap := make(map[uint]*catalogm.Artist, len(allArtists))
	for i := range allArtists {
		artistMap[allArtists[i].ID] = &allArtists[i]
	}

	// Build artists in order
	for _, sa := range showArtists {
		artist, ok := artistMap[sa.ArtistID]
		if !ok {
			continue
		}

		artistData := contracts.ExportArtistData{
			Name:     artist.Name,
			Position: sa.Position,
			SetType:  sa.SetType,
		}
		if artist.City != nil {
			artistData.City = *artist.City
		}
		if artist.State != nil {
			artistData.State = *artist.State
		}
		// Add social links
		if artist.Social.Instagram != nil {
			artistData.Social.Instagram = *artist.Social.Instagram
		}
		if artist.Social.Facebook != nil {
			artistData.Social.Facebook = *artist.Social.Facebook
		}
		if artist.Social.Twitter != nil {
			artistData.Social.Twitter = *artist.Social.Twitter
		}
		if artist.Social.YouTube != nil {
			artistData.Social.YouTube = *artist.Social.YouTube
		}
		if artist.Social.Spotify != nil {
			artistData.Social.Spotify = *artist.Social.Spotify
		}
		if artist.Social.SoundCloud != nil {
			artistData.Social.SoundCloud = *artist.Social.SoundCloud
		}
		if artist.Social.Bandcamp != nil {
			artistData.Social.Bandcamp = *artist.Social.Bandcamp
		}
		if artist.Social.Website != nil {
			artistData.Social.Website = *artist.Social.Website
		}
		frontmatter.Artists = append(frontmatter.Artists, artistData)
	}

	// Marshal frontmatter to YAML
	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Build markdown content
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlData)
	buf.WriteString("---\n\n")

	// Add description as markdown body
	if show.Description != nil && *show.Description != "" {
		buf.WriteString("## Description\n\n")
		buf.WriteString(*show.Description)
		buf.WriteString("\n")
	}

	// Generate filename
	dateStr := show.EventDate.Format("2006-01-02")
	titleSlug := strings.ReplaceAll(strings.ToLower(show.Title), " ", "-")
	// Remove special characters from slug
	titleSlug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(titleSlug, "")
	filename := fmt.Sprintf("show-%s-%s.md", dateStr, titleSlug)

	return buf.Bytes(), filename, nil
}

// contracts.ParsedShowImport represents the parsed markdown data for import preview

// ParseShowMarkdown parses a markdown file and returns the parsed data
func (s *ShowService) ParseShowMarkdown(content []byte) (*contracts.ParsedShowImport, error) {
	// Split frontmatter and body
	str := string(content)

	// Check for frontmatter delimiters
	if !strings.HasPrefix(str, "---") {
		return nil, fmt.Errorf("invalid markdown: missing frontmatter delimiter")
	}

	// Find the closing delimiter
	parts := strings.SplitN(str[3:], "---", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid markdown: missing closing frontmatter delimiter")
	}

	frontmatterYAML := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])

	// Parse frontmatter
	var frontmatter contracts.ExportFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &frontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Extract description from body (look for ## Description section)
	description := ""
	if body != "" {
		scanner := bufio.NewScanner(strings.NewReader(body))
		inDescription := false
		var descLines []string

		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "## Description") {
				inDescription = true
				continue
			}
			if inDescription {
				// Stop at next heading
				if strings.HasPrefix(line, "##") {
					break
				}
				descLines = append(descLines, line)
			}
		}
		description = strings.TrimSpace(strings.Join(descLines, "\n"))
	}

	return &contracts.ParsedShowImport{
		Frontmatter: frontmatter,
		Description: description,
	}, nil
}

// PreviewShowImport previews the import by checking for existing venues and artists
func (s *ShowService) PreviewShowImport(content []byte) (*contracts.ImportPreviewResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Parse the markdown
	parsed, err := s.ParseShowMarkdown(content)
	if err != nil {
		return nil, err
	}

	response := &contracts.ImportPreviewResponse{
		Show:      parsed.Frontmatter.Show,
		Venues:    make([]contracts.VenueMatchResult, 0),
		Artists:   make([]contracts.ArtistMatchResult, 0),
		Warnings:  make([]string, 0),
		CanImport: true,
	}

	// Validate required fields
	if parsed.Frontmatter.Show.EventDate == "" {
		response.Warnings = append(response.Warnings, "Missing event date")
		response.CanImport = false
	}

	if len(parsed.Frontmatter.Venues) == 0 {
		response.Warnings = append(response.Warnings, "No venues specified")
		response.CanImport = false
	}

	if len(parsed.Frontmatter.Artists) == 0 {
		response.Warnings = append(response.Warnings, "No artists specified")
		response.CanImport = false
	}

	// Check venues
	for _, venueData := range parsed.Frontmatter.Venues {
		result := contracts.VenueMatchResult{
			Name:  venueData.Name,
			City:  venueData.City,
			State: venueData.State,
		}

		// Match by LOWER(name) = ? AND LOWER(city) = ?
		var venue catalogm.Venue
		err := s.db.Where("LOWER(name) = ? AND LOWER(city) = ?",
			strings.ToLower(venueData.Name),
			strings.ToLower(venueData.City),
		).First(&venue).Error

		if err == nil {
			result.ExistingID = &venue.ID
			result.WillCreate = false
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			result.WillCreate = true
		} else {
			return nil, fmt.Errorf("failed to check venue: %w", err)
		}

		response.Venues = append(response.Venues, result)
	}

	// Check artists.
	//
	// set_type is normalized to exactly what ConfirmShowImport will store, not
	// echoed raw: a preview that shows a value the import will not write is
	// worse than no preview, and the headliner warning below compares against
	// this field. An unmappable label previews as the neutral default, which
	// is what the import writes.
	for _, artistData := range parsed.Frontmatter.Artists {
		previewSetType := contracts.NormalizeSetType(artistData.SetType)
		if previewSetType == "" {
			previewSetType = contracts.SetTypeDefault
		}
		result := contracts.ArtistMatchResult{
			Name:     artistData.Name,
			Position: artistData.Position,
			SetType:  previewSetType,
		}

		// Match by LOWER(name) = ?
		var artist catalogm.Artist
		err := s.db.Where("LOWER(name) = ?", strings.ToLower(artistData.Name)).First(&artist).Error

		if err == nil {
			result.ExistingID = &artist.ID
			result.WillCreate = false
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			result.WillCreate = true
		} else {
			return nil, fmt.Errorf("failed to check artist: %w", err)
		}

		response.Artists = append(response.Artists, result)
	}

	// Check for potential duplicate (same headliner at same venue on same date)
	eventDate, err := time.Parse(time.RFC3339, parsed.Frontmatter.Show.EventDate)
	if err == nil {
		// Find headliners
		for _, artistResult := range response.Artists {
			if artistResult.SetType == contracts.SetTypeHeadliner && artistResult.ExistingID != nil {
				for _, venueResult := range response.Venues {
					if venueResult.ExistingID != nil {
						// Check for existing show
						var existingShows []catalogm.Show
						s.db.Table("shows").
							Joins("JOIN show_artists ON shows.id = show_artists.show_id").
							Joins("JOIN show_venues ON shows.id = show_venues.show_id").
							Where("show_artists.artist_id = ? AND show_venues.venue_id = ? AND shows.event_date = ? AND show_artists.set_type = ?",
								*artistResult.ExistingID, *venueResult.ExistingID, eventDate.UTC(), "headliner").
							Find(&existingShows)

						if len(existingShows) > 0 {
							response.Warnings = append(response.Warnings,
								fmt.Sprintf("Warning: Headliner '%s' already has a show at '%s' on this date",
									artistResult.Name, venueResult.Name))
						}
					}
				}
			}
		}
	}

	return response, nil
}

// contracts.AdminShowFilters contains filters for GetAdminShows

// GetAdminShows retrieves shows for admin with optional filters (for CLI export)
// Returns shows with all statuses including pending, rejected, and private
func (s *ShowService) GetAdminShows(limit, offset int, filters contracts.AdminShowFilters) ([]*contracts.ShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Build base query
	baseQuery := s.db.Model(&catalogm.Show{})

	// Apply status filter
	if filters.Status != "" {
		baseQuery = baseQuery.Where("status = ?", filters.Status)
	}

	// Apply date filters
	if filters.FromDate != "" {
		fromDate, err := time.Parse(time.RFC3339, filters.FromDate)
		if err == nil {
			baseQuery = baseQuery.Where("event_date >= ?", fromDate.UTC())
		}
	}
	if filters.ToDate != "" {
		toDate, err := time.Parse(time.RFC3339, filters.ToDate)
		if err == nil {
			baseQuery = baseQuery.Where("event_date <= ?", toDate.UTC())
		}
	}

	// Apply city filter
	if filters.City != "" {
		baseQuery = baseQuery.Where("city = ?", filters.City)
	}

	// Get total count
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count shows: %w", err)
	}

	// Get shows with pagination
	var shows []catalogm.Show
	err := s.db.Preload("Venues").Preload("Artists").
		Scopes(func(db *gorm.DB) *gorm.DB {
			if filters.Status != "" {
				db = db.Where("status = ?", filters.Status)
			}
			if filters.FromDate != "" {
				fromDate, err := time.Parse(time.RFC3339, filters.FromDate)
				if err == nil {
					db = db.Where("event_date >= ?", fromDate.UTC())
				}
			}
			if filters.ToDate != "" {
				toDate, err := time.Parse(time.RFC3339, filters.ToDate)
				if err == nil {
					db = db.Where("event_date <= ?", toDate.UTC())
				}
			}
			if filters.City != "" {
				db = db.Where("city = ?", filters.City)
			}
			return db
		}).
		Order("event_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&shows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get shows: %w", err)
	}

	// Build responses
	responses := make([]*contracts.ShowResponse, len(shows))
	for i, show := range shows {
		responses[i] = s.buildShowResponse(&show)
	}

	return responses, total, nil
}

// ConfirmShowImport creates a show from the parsed markdown content
// Admin imports auto-verify venues
func (s *ShowService) ConfirmShowImport(content []byte, isAdmin bool) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Parse the markdown
	parsed, err := s.ParseShowMarkdown(content)
	if err != nil {
		return nil, err
	}

	// Parse event date
	eventDate, err := time.Parse(time.RFC3339, parsed.Frontmatter.Show.EventDate)
	if err != nil {
		return nil, fmt.Errorf("invalid event date: %w", err)
	}

	// Build venues for contracts.CreateShowRequest
	var requestVenues []contracts.CreateShowVenue
	for _, venueData := range parsed.Frontmatter.Venues {
		requestVenues = append(requestVenues, contracts.CreateShowVenue{
			Name:    venueData.Name,
			City:    venueData.City,
			State:   venueData.State,
			Address: venueData.Address,
		})
	}

	// Build artists for contracts.CreateShowRequest.
	//
	// The export frontmatter's set_type is a curated value that a prior export
	// wrote out, so it is passed through rather than collapsed to a boolean --
	// an import used to preserve only "was this the headliner" and silently
	// flattened every other role. Values the vocabulary does not recognize are
	// left off so the import falls back to the default instead of failing a
	// whole file on one stale label.
	var requestArtists []contracts.CreateShowArtist
	for _, artistData := range parsed.Frontmatter.Artists {
		// is_headliner is pinned false for every entry, and that false is the
		// only part still doing work: a curated set_type below already decides
		// the slot and outranks the flag. Pinning it suppresses the position-0
		// headliner inference, because an export that stated ANY label -- even
		// one the vocabulary cannot map -- has already described the bill, and
		// first-in-file is not a second opinion.
		noPositionInference := false
		entry := contracts.CreateShowArtist{
			Name:        artistData.Name,
			IsHeadliner: &noPositionInference,
		}
		if normalized := contracts.NormalizeSetType(artistData.SetType); normalized != "" {
			entry.SetType = &normalized
		}
		requestArtists = append(requestArtists, entry)
	}

	// Build the create request
	req := &contracts.CreateShowRequest{
		Title:            parsed.Frontmatter.Show.Title,
		EventDate:        eventDate,
		City:             parsed.Frontmatter.Show.City,
		State:            parsed.Frontmatter.Show.State,
		Price:            parsed.Frontmatter.Show.Price,
		AgeRequirement:   parsed.Frontmatter.Show.AgeRequirement,
		Description:      parsed.Description,
		Venues:           requestVenues,
		Artists:          requestArtists,
		SubmitterIsAdmin: isAdmin,
	}

	// Create the show
	return s.CreateShow(req)
}

// ============================================================================
// Show Status Flag Methods (Admin Only)
// ============================================================================

// SetShowSoldOut sets or clears the is_sold_out flag on a show
func (s *ShowService) SetShowSoldOut(showID uint, isSoldOut bool) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var show catalogm.Show
	if err := s.db.First(&show, showID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrShowNotFound(showID)
		}
		return nil, fmt.Errorf("failed to find show: %w", err)
	}

	if err := s.db.Model(&show).Update("is_sold_out", isSoldOut).Error; err != nil {
		return nil, fmt.Errorf("failed to update show sold out status: %w", err)
	}

	return s.GetShow(showID)
}

// SetShowCancelled sets or clears the is_cancelled flag on a show
func (s *ShowService) SetShowCancelled(showID uint, isCancelled bool) (*contracts.ShowResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var show catalogm.Show
	if err := s.db.First(&show, showID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrShowNotFound(showID)
		}
		return nil, fmt.Errorf("failed to find show: %w", err)
	}

	if err := s.db.Model(&show).Update("is_cancelled", isCancelled).Error; err != nil {
		return nil, fmt.Errorf("failed to update show cancelled status: %w", err)
	}

	return s.GetShow(showID)
}
