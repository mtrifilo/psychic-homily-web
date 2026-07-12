package engagement

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// SavedShowService handles saved show business logic
// Backed by the generic user_bookmarks table via BookmarkService
type SavedShowService struct {
	db       *gorm.DB
	bookmark *BookmarkService
}

// NewSavedShowService creates a new saved show service
func NewSavedShowService(database *gorm.DB) *SavedShowService {
	if database == nil {
		database = db.GetDB()
	}
	return &SavedShowService{
		db:       database,
		bookmark: NewBookmarkService(database),
	}
}

// SaveShow saves a show to a user's list
// Note: Unlike the original plan, this allows saving shows of any status (pending/approved/rejected)
// as per user requirements
func (s *SavedShowService) SaveShow(userID, showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if show exists
	var show catalogm.Show
	if err := s.db.First(&show, showID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrShowNotFound(showID)
		}
		return fmt.Errorf("failed to verify show: %w", err)
	}

	if err := s.bookmark.CreateBookmark(userID, engagementm.BookmarkEntityShow, showID, engagementm.BookmarkActionSave); err != nil {
		return fmt.Errorf("failed to save show: %w", err)
	}

	return nil
}

// UnsaveShow removes a show from a user's list
func (s *SavedShowService) UnsaveShow(userID, showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	err := s.bookmark.DeleteBookmark(userID, engagementm.BookmarkEntityShow, showID, engagementm.BookmarkActionSave)
	if err != nil {
		if err.Error() == "bookmark not found" {
			return fmt.Errorf("show was not saved")
		}
		return fmt.Errorf("failed to unsave show: %w", err)
	}

	return nil
}

// savedShowRef pairs a saved show's ID with when the user saved it. The slice
// order is the response order, so each list path (saved-at vs event-date) picks
// its ordering once, up front, and hydration stays order-agnostic.
type savedShowRef struct {
	ShowID  uint
	SavedAt time.Time
}

// savedShowVenueTZJoin resolves each saved show's venue-local IANA zone for the
// upcoming/past partition. The first venue (lowest venue_id) decides the zone,
// mirroring the "first venue" convention the calendar/reminder renderers use.
// The pg_timezone_names lookup + COALESCE('UTC') follows the radio precedent
// (catalog.stationLocalToday): a NULL, blank, or malformed stored zone degrades
// to UTC instead of erroring the whole query. Shows with no venue keep the
// LEFT LATERAL row NULL, so the conditions below COALESCE once more.
const savedShowVenueTZJoin = `LEFT JOIN LATERAL (
	SELECT COALESCE(
		(SELECT name FROM pg_timezone_names
		 WHERE lower(name) = lower(btrim(v.timezone, E' \t\n\r'))),
		'UTC') AS name
	FROM show_venues sv
	JOIN venues v ON v.id = sv.venue_id
	WHERE sv.show_id = shows.id
	ORDER BY sv.venue_id
	LIMIT 1
) venue_tz ON true`

// savedShowVenueLocalDateSQL is the show's calendar date in the venue's local
// zone. event_date is TIMESTAMPTZ (migration 000028), so a single AT TIME ZONE
// shifts the instant into the venue's wall clock before the ::date cast —
// matching how the calendar and reminder services render it with
// time.Time.In(venueZone).
const savedShowVenueLocalDateSQL = `(shows.event_date AT TIME ZONE COALESCE(venue_tz.name, 'UTC'))::date`

// savedShowVenueLocalTodaySQL is "today" on the venue's local calendar. A show
// graduates from upcoming to past when this date passes its venue-local event
// date — i.e. at venue-local midnight, not at the event's start instant.
const savedShowVenueLocalTodaySQL = `(now() AT TIME ZONE COALESCE(venue_tz.name, 'UTC'))::date`

// GetUserSavedShows retrieves shows saved by a user.
//
// timeFilter selects the partition and ordering:
//   - "" — every saved show, most recently saved first (the original
//     behavior; the iOS app and the iCal calendar feed rely on it).
//   - "upcoming" — shows whose venue-local event date is today or later,
//     soonest first.
//   - "past" — shows whose venue-local event date has passed, most recent
//     first.
func (s *SavedShowService) GetUserSavedShows(userID uint, limit, offset int, timeFilter string) ([]*contracts.SavedShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	var refs []savedShowRef
	var total int64

	switch timeFilter {
	case "":
		// Page over bookmarks (created_at DESC) exactly as before.
		bookmarks, bookmarkTotal, err := s.bookmark.GetUserBookmarks(userID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, limit, offset)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to get saved shows: %w", err)
		}
		total = bookmarkTotal
		refs = make([]savedShowRef, len(bookmarks))
		for i, b := range bookmarks {
			refs[i] = savedShowRef{ShowID: b.EntityID, SavedAt: b.CreatedAt}
		}
	case "upcoming", "past":
		var err error
		refs, total, err = s.savedShowPageByEventDate(userID, limit, offset, timeFilter)
		if err != nil {
			return nil, 0, err
		}
	default:
		return nil, 0, fmt.Errorf("invalid time filter: %q", timeFilter)
	}

	return s.hydrateSavedShows(refs, total)
}

// savedShowPageByEventDate pages over a user's saved shows partitioned and
// ordered by event date, venue-timezone-aware. Unlike the saved-at path (which
// pages over bookmarks and then fetches shows), the date partition has to live
// on the shows side of the join, so this pages over shows joined to bookmarks.
func (s *SavedShowService) savedShowPageByEventDate(userID uint, limit, offset int, timeFilter string) ([]savedShowRef, int64, error) {
	var dateCondition, order string
	switch timeFilter {
	case "past":
		dateCondition = savedShowVenueLocalDateSQL + " < " + savedShowVenueLocalTodaySQL
		order = "shows.event_date DESC, shows.id DESC"
	default: // "upcoming"
		dateCondition = savedShowVenueLocalDateSQL + " >= " + savedShowVenueLocalTodaySQL
		order = "shows.event_date ASC, shows.id ASC"
	}

	// Fresh builder per query: GORM builders accumulate clauses, so Count and
	// Find must not share one.
	baseQuery := func() *gorm.DB {
		return s.db.Table("user_bookmarks").
			Joins("JOIN shows ON shows.id = user_bookmarks.entity_id").
			Joins(savedShowVenueTZJoin).
			Where("user_bookmarks.user_id = ? AND user_bookmarks.entity_type = ? AND user_bookmarks.action = ?",
				userID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave).
			Where(dateCondition)
	}

	var total int64
	if err := baseQuery().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count saved shows: %w", err)
	}

	var refs []savedShowRef
	err := baseQuery().
		Select("user_bookmarks.entity_id AS show_id, user_bookmarks.created_at AS saved_at").
		Order(order).
		Limit(limit).
		Offset(offset).
		Find(&refs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get saved shows: %w", err)
	}

	return refs, total, nil
}

// hydrateSavedShows fetches full show data (venues, artists) for the given
// refs and builds responses preserving the refs' order.
func (s *SavedShowService) hydrateSavedShows(refs []savedShowRef, total int64) ([]*contracts.SavedShowResponse, int64, error) {
	showIDs := make([]uint, len(refs))
	for i, r := range refs {
		showIDs[i] = r.ShowID
	}

	if len(showIDs) == 0 {
		return []*contracts.SavedShowResponse{}, total, nil
	}

	// Fetch shows with associations (no status filter - user can save any show)
	var shows []catalogm.Show
	err := s.db.Preload("Venues").
		Where("id IN ?", showIDs).
		Find(&shows).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch shows: %w", err)
	}

	// Create a map for O(1) lookup
	showMap := make(map[uint]*catalogm.Show)
	for i := range shows {
		showMap[shows[i].ID] = &shows[i]
	}

	// Batch-load all ShowArtist records for all shows
	var allShowArtists []catalogm.ShowArtist
	if len(showIDs) > 0 {
		s.db.Where("show_id IN ?", showIDs).Order("position ASC").Find(&allShowArtists)
	}

	// Collect all unique artist IDs
	var allArtistIDs []uint
	for _, sa := range allShowArtists {
		allArtistIDs = append(allArtistIDs, sa.ArtistID)
	}

	// Batch-fetch all artists in one query
	artistMap := make(map[uint]*catalogm.Artist)
	if len(allArtistIDs) > 0 {
		var allArtists []catalogm.Artist
		s.db.Where("id IN ?", allArtistIDs).Find(&allArtists)
		for i := range allArtists {
			artistMap[allArtists[i].ID] = &allArtists[i]
		}
	}

	// Build per-show artist response slices
	artistsByShow := make(map[uint][]contracts.ArtistResponse)
	for _, sa := range allShowArtists {
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
		var slug string
		if artist.Slug != nil {
			slug = *artist.Slug
		}
		artistsByShow[sa.ShowID] = append(artistsByShow[sa.ShowID], contracts.ArtistResponse{
			ID:               artist.ID,
			Slug:             slug,
			Name:             artist.Name,
			State:            artist.State,
			City:             artist.City,
			IsHeadliner:      &isHeadliner,
			SetType:          sa.SetType,
			Position:         sa.Position,
			IsNewArtist:      &isNewArtist,
			BandcampEmbedURL: artist.BandcampEmbedURL,
			Socials:          socials,
		})
	}

	// Build responses in the refs' order
	responses := make([]*contracts.SavedShowResponse, 0, len(shows))
	for _, r := range refs {
		if show, ok := showMap[r.ShowID]; ok {
			showResp := s.buildShowResponse(show, artistsByShow)
			responses = append(responses, &contracts.SavedShowResponse{
				ShowResponse: *showResp,
				SavedAt:      r.SavedAt,
			})
		}
	}

	return responses, total, nil
}

// buildShowResponse builds a ShowResponse from a catalogm.Show
// artistsByShow contains pre-loaded artist responses keyed by show ID
func (s *SavedShowService) buildShowResponse(show *catalogm.Show, artistsByShow map[uint][]contracts.ArtistResponse) *contracts.ShowResponse {
	// Build venue responses
	venues := make([]contracts.VenueResponse, len(show.Venues))
	for i, venue := range show.Venues {
		var venueSlug string
		if venue.Slug != nil {
			venueSlug = *venue.Slug
		}
		venues[i] = contracts.VenueResponse{
			ID:       venue.ID,
			Slug:     venueSlug,
			Name:     venue.Name,
			Address:  venue.Address,
			City:     venue.City,
			State:    venue.State,
			Timezone: venue.Timezone,
			Verified: venue.Verified,
		}
	}

	artists := artistsByShow[show.ID]

	showSlug := ""
	if show.Slug != nil {
		showSlug = *show.Slug
	}
	return &contracts.ShowResponse{
		ID:                show.ID,
		Slug:              showSlug,
		Title:             show.Title,
		EventDate:         show.EventDate,
		City:              show.City,
		State:             show.State,
		Price:             show.Price,
		AgeRequirement:    show.AgeRequirement,
		Description:       show.Description,
		TicketURL:         show.TicketURL,
		Status:            string(show.Status),
		SubmittedBy:       show.SubmittedBy,
		RejectionReason:   show.RejectionReason,
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

// IsShowSaved checks if a show is saved by a user
func (s *SavedShowService) IsShowSaved(userID, showID uint) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	return s.bookmark.IsBookmarked(userID, engagementm.BookmarkEntityShow, showID, engagementm.BookmarkActionSave)
}

// GetSavedShowIDs returns a set of show IDs that a user has saved
// Useful for batch checking (e.g., mark which shows in a list are saved)
func (s *SavedShowService) GetSavedShowIDs(userID uint, showIDs []uint) (map[uint]bool, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	return s.bookmark.GetBookmarkedEntityIDs(userID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, showIDs)
}

// GetSaveCount returns the public save count for a show.
//
// The count is an aggregate only — no endpoint anywhere exposes which users
// saved a show, so a user's saved list stays private while the count doubles as
// a buzz signal for visitors.
func (s *SavedShowService) GetSaveCount(showID uint) (int, error) {
	counts, err := s.GetBatchSaveCounts([]uint{showID})
	if err != nil {
		return 0, err
	}
	return counts[showID], nil
}

// GetBatchSaveCounts returns public save counts for multiple shows in a single
// query.
//
// Only APPROVED shows contribute a count. A show can be saved while it is
// pending, rejected, or private (SaveShow deliberately allows any status), and
// GET /shows/{id} already 404s those for anyone but the submitter and admins.
// Without the status filter the public count would be a side channel revealing
// that a hidden show exists and has engagement — enumerable across the whole
// sequential ID space via the batch endpoint.
//
// Hidden shows are reported as 0 rather than omitted or 404'd, which is what
// makes this safe: an unlisted show is indistinguishable from an approved show
// nobody has saved, so there is no existence oracle. Every requested ID is
// present in the map, zero-filled, so callers can still distinguish "requested"
// from "not requested".
func (s *SavedShowService) GetBatchSaveCounts(showIDs []uint) (map[uint]int, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := make(map[uint]int, len(showIDs))
	if len(showIDs) == 0 {
		return result, nil
	}
	for _, id := range showIDs {
		result[id] = 0
	}

	type countRow struct {
		EntityID uint
		Count    int
	}
	var rows []countRow

	err := s.db.Model(&engagementm.UserBookmark{}).
		Select("user_bookmarks.entity_id, COUNT(*) as count").
		Joins("JOIN shows ON shows.id = user_bookmarks.entity_id").
		Where("user_bookmarks.entity_type = ? AND user_bookmarks.entity_id IN ? AND user_bookmarks.action = ?",
			engagementm.BookmarkEntityShow, showIDs, engagementm.BookmarkActionSave,
		).
		Where("shows.status = ?", catalogm.ShowStatusApproved).
		Group("user_bookmarks.entity_id").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get batch save counts: %w", err)
	}

	for _, row := range rows {
		if _, requested := result[row.EntityID]; requested {
			result[row.EntityID] = row.Count
		}
	}

	return result, nil
}
