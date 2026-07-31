package engagement

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// PSY-1584: a PUBLIC per-venue iCalendar feed.
//
// This lives next to the personal saved-shows feed (calendar.go) rather than in
// the catalog package on purpose: the load-bearing, easy-to-get-silently-wrong
// part of an ICS feed is venue-local time anchoring, and that logic
// (setVenueLocalEventTimes) already exists here. A second copy in another
// package is exactly how the two would drift, and a drifted copy fails
// invisibly — subscribers just see shows at the wrong hour.
//
// What is deliberately NOT shared with the personal feed:
//   - Cache scope. That one is keyed by user and invalidated on token
//     create/delete; this one is keyed by venue with no invalidation hook, so it
//     leans on a short TTL plus HTTP revalidation instead.
//   - Cancelled shows. The personal feed drops them; this one must emit
//     STATUS:CANCELLED so a subscriber sees the cancellation rather than the
//     event silently vanishing from their calendar.

const (
	// venueFeedShowLimit bounds a single feed. A public endpoint must not let a
	// venue with a huge history turn one poll into an unbounded query; 500
	// upcoming shows is far beyond any real room's booking horizon and matches
	// the personal feed's cap.
	venueFeedShowLimit = 500

	// venueFeedCacheTTL is the in-process cache window. Calendar clients poll on
	// their own schedule and can be aggressive, and this endpoint is anonymous,
	// so repeated polls must not each cost a multi-query DB round trip.
	venueFeedCacheTTL = 5 * time.Minute

	// venueFeedCacheMaxEntries caps the cache so a crawler walking every venue
	// slug cannot grow it without bound. On overflow the whole map is dropped
	// rather than evicted LRU-style: the TTL is short, a cold rebuild is cheap,
	// and a crude bound that is obviously correct beats an LRU that is subtly
	// wrong.
	//
	// Worst case is this count times a venueFeedShowLimit-sized feed (~450 bytes
	// per event, so ~29MB at 128 x 500 events). Real rooms book tens of shows,
	// not hundreds, which puts the realistic figure near 3MB — but the ceiling
	// is what matters for a public endpoint, so it is stated rather than hoped.
	venueFeedCacheMaxEntries = 128

	// venueFeedCalendarProductID identifies this generator per RFC 5545 3.7.3.
	venueFeedCalendarProductID = "-//Psychic Homily//Venue Calendar Feed//EN"
)

type venueFeedCacheEntry struct {
	feed      contracts.VenueCalendarFeed
	expiresAt time.Time
}

// VenueCalendarService renders the public per-venue ICS feed.
//
// It reads the venue through VenueServiceInterface rather than the venues table
// directly. That is not indirection for its own sake: the service layer redacts
// street address and zipcode for unverified venues (DIY / house-show
// protection), and a feed that queried the model directly would publish exactly
// the addresses that redaction exists to withhold.
type VenueCalendarService struct {
	db       *gorm.DB
	venueSvc contracts.VenueServiceInterface

	cacheMu sync.RWMutex
	cache   map[uint]venueFeedCacheEntry
}

// NewVenueCalendarService creates the public venue feed service.
func NewVenueCalendarService(database *gorm.DB, venueSvc contracts.VenueServiceInterface) *VenueCalendarService {
	if database == nil {
		database = db.GetDB()
	}
	return &VenueCalendarService{
		db:       database,
		venueSvc: venueSvc,
		cache:    make(map[uint]venueFeedCacheEntry),
	}
}

// GenerateVenueFeed renders every upcoming show at the venue identified by
// idOrSlug as an RFC 5545 calendar.
func (s *VenueCalendarService) GenerateVenueFeed(idOrSlug string, frontendURL string) (*contracts.VenueCalendarFeed, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if s.venueSvc == nil {
		return nil, fmt.Errorf("venue service not initialized")
	}

	venue, err := s.resolveVenue(idOrSlug)
	if err != nil {
		return nil, err
	}

	// Cache AFTER resolution, keyed by the venue's ID. Keying by the raw
	// idOrSlug would let a caller mint an unbounded number of cache entries for
	// one venue (or for venues that do not exist) simply by varying the path.
	//
	// The key does NOT include frontendURL, which is safe only because that
	// value comes from process config and is identical for every request. If it
	// ever becomes request-derived (a Host header, a locale prefix), this cache
	// starts serving the wrong show links and the key has to grow.
	if cached, ok := s.cachedFeed(venue.ID); ok {
		return cached, nil
	}

	shows, err := s.upcomingShowsForVenue(venue)
	if err != nil {
		return nil, err
	}

	artistsByShow, err := s.artistNamesByShow(shows)
	if err != nil {
		return nil, err
	}

	payload := s.buildCalendar(venue, shows, artistsByShow, frontendURL)

	sum := sha256.Sum256(payload)
	feed := contracts.VenueCalendarFeed{
		VenueName: venue.Name,
		VenueSlug: venue.Slug,
		ICS:       payload,
		ETag:      `W/"` + hex.EncodeToString(sum[:16]) + `"`,
	}
	s.storeFeed(venue.ID, feed)

	return cloneFeed(feed), nil
}

// resolveVenue accepts either a numeric ID or a slug, matching the
// {venue_id} convention every other venue sub-resource uses.
func (s *VenueCalendarService) resolveVenue(idOrSlug string) (*contracts.VenueDetailResponse, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return s.venueSvc.GetVenue(uint(id))
	}
	return s.venueSvc.GetVenueBySlug(idOrSlug)
}

// upcomingShowsForVenue returns approved shows at the venue from the start of
// the venue's local today onward.
//
// The cutoff is computed in the VENUE's zone, not UTC and not the caller's:
// a show stored at 2026-08-01T02:00Z is still "tonight" for a Phoenix room, and
// a UTC-midnight cutoff would drop it from the feed hours early.
//
// Cancelled shows are deliberately INCLUDED — they are emitted with
// STATUS:CANCELLED below.
func (s *VenueCalendarService) upcomingShowsForVenue(venue *contracts.VenueDetailResponse) ([]catalogm.Show, error) {
	loc := venueLocation(venue)
	now := time.Now().In(loc)
	startOfLocalToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()

	var showIDs []uint
	err := s.db.Table("show_venues").
		Select("show_venues.show_id").
		Joins("JOIN shows ON show_venues.show_id = shows.id").
		Where("show_venues.venue_id = ? AND shows.status = ? AND shows.event_date >= ?",
			venue.ID, catalogm.ShowStatusApproved, startOfLocalToday).
		Order("shows.event_date ASC").
		Limit(venueFeedShowLimit).
		Pluck("show_venues.show_id", &showIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get venue show ids: %w", err)
	}
	if len(showIDs) == 0 {
		return nil, nil
	}

	var shows []catalogm.Show
	if err := s.db.Where("id IN ?", showIDs).Order("event_date ASC").Find(&shows).Error; err != nil {
		return nil, fmt.Errorf("failed to get venue shows: %w", err)
	}
	return shows, nil
}

// artistNamesByShow batch-loads the billing for every show in one pair of
// queries (not one per show) and returns names in billed order.
func (s *VenueCalendarService) artistNamesByShow(shows []catalogm.Show) (map[uint][]string, error) {
	names := make(map[uint][]string, len(shows))
	if len(shows) == 0 {
		return names, nil
	}

	showIDs := make([]uint, len(shows))
	for i, show := range shows {
		showIDs[i] = show.ID
	}

	var billing []catalogm.ShowArtist
	if err := s.db.Where("show_id IN ?", showIDs).Order("position ASC").Find(&billing).Error; err != nil {
		return nil, fmt.Errorf("failed to get show billing: %w", err)
	}
	if len(billing) == 0 {
		return names, nil
	}

	artistIDs := make([]uint, 0, len(billing))
	for _, row := range billing {
		artistIDs = append(artistIDs, row.ArtistID)
	}

	var artists []catalogm.Artist
	if err := s.db.Where("id IN ?", artistIDs).Find(&artists).Error; err != nil {
		return nil, fmt.Errorf("failed to get show artists: %w", err)
	}
	byID := make(map[uint]string, len(artists))
	for _, artist := range artists {
		byID[artist.ID] = artist.Name
	}

	for _, row := range billing {
		if name, ok := byID[row.ArtistID]; ok {
			names[row.ShowID] = append(names[row.ShowID], name)
		}
	}
	return names, nil
}

// buildCalendar renders the VCALENDAR. Every value that originates in
// community-editable data is passed through sanitizeICSText first — see its doc
// comment for why that is a correctness requirement and not just hygiene.
func (s *VenueCalendarService) buildCalendar(
	venue *contracts.VenueDetailResponse,
	shows []catalogm.Show,
	artistsByShow map[uint][]string,
	frontendURL string,
) []byte {
	venueName := sanitizeICSText(venue.Name)
	location := sanitizeICSText(formatVenueDetailLocation(venue))
	calendarName := venueName + " — Shows"

	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	cal.SetProductId(venueFeedCalendarProductID)
	cal.SetName(calendarName)
	cal.SetDescription("Upcoming shows at " + venueName + ", via Psychic Homily")
	cal.SetXWRCalName(calendarName)

	for i := range shows {
		show := &shows[i]
		event := cal.AddEvent(showEventUID(show.ID))

		// DTSTAMP is REQUIRED by RFC 5545 for a VEVENT in a METHOD:PUBLISH
		// calendar, and golang-ical does not add one. It is set from the show's
		// UpdatedAt rather than time.Now() so that identical DB state renders a
		// byte-identical feed — which is what makes the ETag below stable enough
		// for a polling client to get a 304 instead of a fresh copy every time.
		event.SetDtStampTime(show.UpdatedAt.UTC())
		event.SetCreatedTime(show.CreatedAt)
		event.SetModifiedAt(show.UpdatedAt)

		// SEQUENCE must increase whenever the event changes, or clients ignore
		// the update. Seconds-since-creation is monotonic (UpdatedAt only moves
		// forward), starts at 0, stays far below the 32-bit ceiling clients
		// assume, and — unlike a raw Unix timestamp — does not overflow in 2038.
		event.SetSequence(showSequence(show.CreatedAt, show.UpdatedAt))

		// The show inherits the FEED's venue zone. Every show in this feed is at
		// this room by construction, so there is no per-show venue to consult.
		setVenueLocalEventTimes(event, show.EventDate, defaultShowDuration, venue.Timezone, venue.State)

		summary := sanitizeICSText(show.Title)
		if summary == "" {
			summary = "Show at " + venueName
		}
		if show.IsCancelled {
			// STATUS:CANCELLED is the machine-readable signal, but several
			// mobile clients render a cancelled event indistinguishably from a
			// live one. The prefix is what a human actually sees.
			summary = "CANCELLED: " + summary
		} else if show.IsSoldOut {
			summary += " [SOLD OUT]"
		}
		event.SetSummary(summary)

		if show.IsCancelled {
			event.SetStatus(ics.ObjectStatusCancelled)
		} else {
			event.SetStatus(ics.ObjectStatusConfirmed)
		}

		if location != "" {
			event.SetLocation(location)
		}

		showURL := showPageURL(frontendURL, show)
		event.SetDescription(buildEventDescription(location, artistsByShow[show.ID], show, showURL))
		if showURL != "" {
			event.SetURL(showURL)
		}
	}

	// RFC 5545 3.1 mandates CRLF line endings, and golang-ical defaults to bare
	// LF. Strict clients reject an LF-delimited calendar outright, so the ending
	// has to be requested explicitly — the default is quietly non-conformant.
	return []byte(cal.Serialize(ics.WithNewLine("\r\n")))
}

// buildEventDescription assembles the human-readable body. Lines are joined with
// "\n" deliberately: golang-ical escapes those into the literal `\n` that RFC
// 5545 uses for a line break inside a TEXT value.
func buildEventDescription(location string, artistNames []string, show *catalogm.Show, showURL string) string {
	var parts []string

	if location != "" {
		parts = append(parts, "Venue: "+location)
	}
	if len(artistNames) > 0 {
		clean := make([]string, 0, len(artistNames))
		for _, name := range artistNames {
			if sanitized := sanitizeICSText(name); sanitized != "" {
				clean = append(clean, sanitized)
			}
		}
		if len(clean) > 0 {
			parts = append(parts, "Artists: "+strings.Join(clean, ", "))
		}
	}
	if show.Price != nil {
		parts = append(parts, fmt.Sprintf("Price: $%.0f", *show.Price))
	}
	if show.AgeRequirement != nil {
		if ages := sanitizeICSText(*show.AgeRequirement); ages != "" {
			parts = append(parts, "Ages: "+ages)
		}
	}
	if show.IsCancelled {
		parts = append(parts, "This show has been cancelled.")
	}
	if showURL != "" {
		parts = append(parts, showURL)
	}

	return strings.Join(parts, "\n")
}

// showEventUID derives the event identity.
//
// UID stability is the whole reason an edited show updates its calendar entry
// instead of duplicating it, so it is derived ONLY from the show's immutable
// database ID — never from the title, date, venue, or slug, every one of which
// is editable. It intentionally matches the personal feed's UID scheme: the same
// show reached through two different feeds is the same event.
func showEventUID(showID uint) string {
	return fmt.Sprintf("show-%d@psychichomily.com", showID)
}

// showSequence returns a monotonic revision counter for a show. See the call
// site for why it is seconds-since-creation rather than a Unix timestamp.
func showSequence(createdAt, updatedAt time.Time) int {
	seconds := updatedAt.Unix() - createdAt.Unix()
	if seconds < 0 {
		return 0
	}
	if seconds > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(seconds)
}

// showPageURL builds the link back to the show page, preferring the slug and
// falling back to the numeric ID for shows that predate slugs.
func showPageURL(frontendURL string, show *catalogm.Show) string {
	base := strings.TrimRight(frontendURL, "/")
	if base == "" {
		return ""
	}
	if show.Slug != nil && *show.Slug != "" {
		return base + "/shows/" + *show.Slug
	}
	return fmt.Sprintf("%s/shows/%d", base, show.ID)
}

// venueLocation resolves the venue's IANA zone via the shared convention
// (explicit venues.timezone, then the US state map).
func venueLocation(venue *contracts.VenueDetailResponse) *time.Location {
	return utils.EventLocation(venue.Timezone, venue.State)
}

// formatVenueDetailLocation builds the LOCATION string. It mirrors
// formatVenueLocation but reads VenueDetailResponse, which is the shape the
// venue service returns (and the shape whose address is already redacted for
// unverified venues).
func formatVenueDetailLocation(venue *contracts.VenueDetailResponse) string {
	parts := []string{venue.Name}
	if venue.Address != nil && strings.TrimSpace(*venue.Address) != "" {
		parts = append(parts, strings.TrimSpace(*venue.Address))
	}
	if venue.City != "" {
		parts = append(parts, venue.City)
	}
	if venue.State != "" {
		parts = append(parts, venue.State)
	}
	return strings.Join(parts, ", ")
}

// sanitizeICSText strips control characters from a value before it is written
// into a calendar property.
//
// This is a correctness requirement, not defensive decoration. iCalendar is a
// line-oriented format and golang-ical's TEXT escaper handles `\`, `;`, `,` and
// LF — but NOT CR. Venue names, show titles and artist names are all
// community-editable, so without this a submitted name containing a carriage
// return could emit a raw CR mid-property and let a lenient parser read the
// remainder of the value as a forged calendar property.
func sanitizeICSText(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(cleaned)
}

// cachedFeed returns a defensive copy of a live cache entry.
func (s *VenueCalendarService) cachedFeed(venueID uint) (*contracts.VenueCalendarFeed, bool) {
	s.cacheMu.RLock()
	entry, ok := s.cache[venueID]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return cloneFeed(entry.feed), true
}

func (s *VenueCalendarService) storeFeed(venueID uint, feed contracts.VenueCalendarFeed) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if len(s.cache) >= venueFeedCacheMaxEntries {
		s.cache = make(map[uint]venueFeedCacheEntry, venueFeedCacheMaxEntries)
	}
	s.cache[venueID] = venueFeedCacheEntry{
		feed:      *cloneFeed(feed),
		expiresAt: time.Now().Add(venueFeedCacheTTL),
	}
}

// cloneFeed copies the payload so a caller can never mutate the cached bytes.
func cloneFeed(feed contracts.VenueCalendarFeed) *contracts.VenueCalendarFeed {
	out := feed
	out.ICS = make([]byte, len(feed.ICS))
	copy(out.ICS, feed.ICS)
	return &out
}
