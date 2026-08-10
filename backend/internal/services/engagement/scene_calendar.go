package engagement

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// A PUBLIC per-scene iCalendar feed — the "follow this city" affordance that
// works with no account.
//
// It lives next to the venue feed (venue_calendar.go) and the per-show download
// (show_calendar.go) so all four calendar surfaces share event identity
// (showEventUID), revisioning (showSequence), naming (applyEventSummaryAndStatus),
// venue-local time anchoring (setVenueLocalEventTimes) and text sanitization
// (sanitizeICSText). The same show reached through two of them MUST be the same
// event to a client, or a subscriber to both a room and its city ends up with
// duplicates.
//
// What is scene-specific, and is the whole reason this is not the venue feed
// with a different WHERE clause:
//
//   - Every event carries its OWN venue's timezone. A venue feed can hoist the
//     zone to the calendar because every show in it is at one room; a scene
//     spans rooms, and a metro can straddle a zone boundary.
//   - The upcoming cutoff is therefore judged per show against ITS venue's local
//     today, not once against the scene's.
//   - The window is BOUNDED (sceneFeedWindowDays). A room books a season ahead;
//     a whole city does not have a horizon, so an unbounded scene feed would be
//     a slow-growing payload with no natural ceiling.

const (
	// sceneFeedWindowDays bounds how far ahead the feed looks.
	//
	// Two months is past any listing a reader plans around while still being a
	// payload that a calendar client re-fetches cheaply. It is a WINDOW rather
	// than "everything upcoming" because a scene aggregates every room in a
	// metro: the count grows with coverage, and a subscriber gains nothing from
	// a show eight months out that will be re-published on every poll until then.
	sceneFeedWindowDays = 60

	// sceneFeedShowLimit bounds a single feed. A public endpoint must not let a
	// dense metro turn one poll into an unbounded query. Chicago, the densest
	// scene, runs roughly 50 shows a week, so 500 is about ten weeks of its
	// listings — comfortably past the window above.
	sceneFeedShowLimit = 500

	// sceneFeedCacheTTL is the in-process cache window, matching the venue
	// feed's: calendar clients poll on their own schedule and can be aggressive,
	// and this endpoint is anonymous, so repeated polls must not each cost a
	// multi-query DB round trip.
	sceneFeedCacheTTL = 5 * time.Minute

	// sceneFeedCacheMaxEntries caps the cache so a crawler walking every scene
	// slug cannot grow it without bound. On overflow the whole map is dropped
	// rather than evicted LRU-style, exactly as the venue feed does it: the TTL
	// is short, a cold rebuild is cheap, and a crude bound that is obviously
	// correct beats an LRU that is subtly wrong.
	sceneFeedCacheMaxEntries = 128

	// sceneFeedCalendarProductID identifies this generator per RFC 5545 3.7.3.
	sceneFeedCalendarProductID = "-//Psychic Homily//Scene Calendar Feed//EN"
)

type sceneFeedCacheEntry struct {
	feed      contracts.SceneCalendarFeed
	expiresAt time.Time
}

// SceneCalendarService renders the public per-scene ICS feed.
//
// It reads shows through SceneServiceInterface rather than assembling its own
// scene query. That is not indirection for its own sake — the scene service owns
// three things this feed would otherwise have to reimplement and would silently
// get wrong:
//
//   - Scene SCOPE. A scene is a Census metro, not a city string (PSY-1255), so a
//     Tempe show belongs to the Phoenix feed. A hand-rolled city match here
//     would publish a different set of shows than the page the subscriber
//     clicked the feed from.
//   - Scene EXISTENCE. Below the verified-venue threshold there is no scene, and
//     the service already answers that with a not-found error.
//   - Address REDACTION. Its show query serves a street address for VERIFIED
//     venues only, the same site-wide gate as the venue detail payload. A feed
//     that queried the venues table directly would publish exactly the DIY and
//     house-show addresses that gate exists to withhold — into an ICS LOCATION,
//     which is copied onto the subscriber's device where a later redaction can
//     never reach it.
//
// The database handle is for one thing only: the per-show revision metadata the
// display payload deliberately does not publish (see showCalendarRevisions).
type SceneCalendarService struct {
	db       *gorm.DB
	sceneSvc contracts.SceneServiceInterface

	cacheMu sync.RWMutex
	cache   map[string]sceneFeedCacheEntry
}

// NewSceneCalendarService creates the public scene feed service.
func NewSceneCalendarService(database *gorm.DB, sceneSvc contracts.SceneServiceInterface) *SceneCalendarService {
	if database == nil {
		database = db.GetDB()
	}
	return &SceneCalendarService{
		db:       database,
		sceneSvc: sceneSvc,
		cache:    make(map[string]sceneFeedCacheEntry),
	}
}

// GenerateSceneFeed renders the scene's upcoming shows as an RFC 5545 calendar.
func (s *SceneCalendarService) GenerateSceneFeed(slug string, frontendURL string) (*contracts.SceneCalendarFeed, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if s.sceneSvc == nil {
		return nil, fmt.Errorf("scene service not initialized")
	}

	// Resolves an alias to the scene's canonical display identity (a metro member
	// slug lands on its principal city) and returns a not-found error for a slug
	// that names no scene.
	city, state, err := s.sceneSvc.ParseSceneSlug(slug)
	if err != nil {
		return nil, err
	}

	// Cache AFTER resolution, keyed by that canonical identity rather than by the
	// requested slug: keying on the raw path segment would let a caller mint an
	// unbounded number of entries for one scene by varying the alias, and would
	// cache the same bytes once per alias.
	//
	// The key does NOT include frontendURL, which is safe only because that value
	// comes from process config and is identical for every request. If it ever
	// becomes request-derived (a Host header, a locale prefix), this cache starts
	// serving the wrong show links and the key has to grow.
	cacheKey := sceneFeedCacheKey(city, state)
	if cached, ok := s.cachedFeed(cacheKey); ok {
		return cached, nil
	}

	shows, err := s.upcomingShowsForScene(city, state)
	if err != nil {
		return nil, err
	}

	revisions, err := s.showCalendarRevisions(shows)
	if err != nil {
		return nil, err
	}

	payload := buildSceneCalendar(city, state, shows, revisions, frontendURL)

	sum := sha256.Sum256(payload)
	feed := contracts.SceneCalendarFeed{
		SceneName: sceneDisplayName(city, state),
		SceneSlug: sceneFeedSlug(city, state),
		ICS:       payload,
		ETag:      `W/"` + hex.EncodeToString(sum[:16]) + `"`,
	}
	s.storeFeed(cacheKey, feed)

	return cloneSceneFeed(feed), nil
}

// upcomingShowsForScene returns the scene's approved shows that have not yet
// happened, bounded by sceneFeedWindowDays.
//
// "Not yet happened" is judged per show against the start of ITS OWN venue's
// local today — the same rule the venue feed applies, applied per room because a
// scene spans rooms. A show stored at 2026-08-01T02:00Z is still "tonight" for a
// Phoenix venue, and a UTC-midnight cutoff would drop it from the feed hours
// early.
//
// The query therefore asks for a day more than it keeps. Start-of-local-today is
// always within 24 hours of now no matter what zone the room is in (it is "now
// minus the local time of day", and a local time of day is under 24 hours), so a
// 24-hour lower slack is exactly enough to guarantee no still-current show is
// missed, and the per-venue filter below discards the rest. The slack is inside
// sceneFeedShowLimit's headroom (see its note), so at most one day of already-
// finished shows can eat into the cap.
//
// Cancelled shows are deliberately INCLUDED — they are emitted with
// STATUS:CANCELLED, so a subscriber sees the cancellation rather than the event
// silently vanishing from their calendar.
func (s *SceneCalendarService) upcomingShowsForScene(city, state string) ([]contracts.SceneShowSummary, error) {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now.AddDate(0, 0, sceneFeedWindowDays)

	// time.UTC because this feed never reads SceneShowSummary.EventDate — the
	// scene-local calendar date the page groups by. Every event here is anchored
	// from StartsAt in its own venue's zone instead.
	shows, err := s.sceneSvc.GetSceneShowsInRange(city, state, from, to, time.UTC, sceneFeedShowLimit)
	if err != nil {
		return nil, err
	}

	upcoming := make([]contracts.SceneShowSummary, 0, len(shows))
	for _, show := range shows {
		if showHasNotHappenedYet(show, now) {
			upcoming = append(upcoming, show)
		}
	}
	return upcoming, nil
}

// showHasNotHappenedYet reports whether a show still belongs in an upcoming
// feed: its start instant is at or after the beginning of the current day in its
// own venue's timezone.
func showHasNotHappenedYet(show contracts.SceneShowSummary, now time.Time) bool {
	loc := utils.EventLocation(optionalString(show.VenueTimezone), show.VenueState)
	local := now.In(loc)
	startOfLocalToday := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return !show.StartsAt.Before(startOfLocalToday)
}

// showCalendarRevision is the per-show data a CALENDAR needs and a listing does
// not: when the row was created and last edited, plus the age requirement the
// other calendar surfaces put in their description.
type showCalendarRevision struct {
	ID             uint      `gorm:"column:id"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
	AgeRequirement *string   `gorm:"column:age_requirement"`
}

// showCalendarRevisions batch-loads that data for the shows already selected by
// the scene service.
//
// It is a second query rather than three more fields on SceneShowSummary
// deliberately. Those fields are wire-format additions to a payload three public
// listing surfaces share (the Atlas preview row, the weekly page, the nightly
// page), none of which has any use for a revision counter — and the timestamps
// are needed here for reasons that are purely iCalendar:
//
//   - DTSTAMP is REQUIRED on a VEVENT under METHOD:PUBLISH, and it is set from
//     UpdatedAt rather than time.Now() so identical DB state renders
//     byte-identical output. That is what makes the ETag stable enough for a
//     polling client to get a 304 instead of a fresh copy every time.
//   - SEQUENCE must increase whenever the event changes, or clients ignore the
//     update. It matters MORE here than in isolation: every calendar surface
//     emits the same UID per show, and RFC 5546 3.2 resolves a UID collision in
//     favour of the higher SEQUENCE. A scene feed that emitted 0 while the venue
//     feed emitted a real revision would always lose, and a subscriber to both
//     would never see this feed's copy of a shared show.
func (s *SceneCalendarService) showCalendarRevisions(shows []contracts.SceneShowSummary) (map[uint]showCalendarRevision, error) {
	revisions := make(map[uint]showCalendarRevision, len(shows))
	if len(shows) == 0 {
		return revisions, nil
	}

	ids := make([]uint, len(shows))
	for i, show := range shows {
		ids[i] = show.ID
	}

	var rows []showCalendarRevision
	if err := s.db.Table("shows").
		Select("id, created_at, updated_at, age_requirement").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get scene show revisions: %w", err)
	}
	for _, row := range rows {
		revisions[row.ID] = row
	}
	return revisions, nil
}

// buildSceneCalendar renders the VCALENDAR. Every value that originates in
// community-editable data is passed through sanitizeICSText first — see its doc
// comment for why that is a correctness requirement and not just hygiene.
func buildSceneCalendar(
	city, state string,
	shows []contracts.SceneShowSummary,
	revisions map[uint]showCalendarRevision,
	frontendURL string,
) []byte {
	sceneName := sanitizeICSText(sceneDisplayName(city, state))
	calendarName := "Shows in " + sceneName

	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	cal.SetProductId(sceneFeedCalendarProductID)
	cal.SetName(calendarName)
	cal.SetDescription("Upcoming shows in " + sceneName + ", via Psychic Homily")
	cal.SetXWRCalName(calendarName)

	for i := range shows {
		show := &shows[i]

		// A show with no revision row was deleted between the two queries. It is
		// no longer a show, so it is not published — rather than emitting a VEVENT
		// with a zero DTSTAMP, which is a malformed timestamp a client would have
		// to guess at.
		revision, ok := revisions[show.ID]
		if !ok {
			continue
		}

		event := cal.AddEvent(showEventUID(show.ID))

		// DTSTAMP / SEQUENCE discipline as in the venue feed's buildCalendar and
		// the per-show download, whose reasons showCalendarRevisions records.
		event.SetDtStampTime(revision.UpdatedAt.UTC())
		event.SetCreatedTime(revision.CreatedAt)
		event.SetModifiedAt(revision.UpdatedAt)
		event.SetSequence(showSequence(revision.CreatedAt, revision.UpdatedAt))

		// Each event carries its OWN room's zone. This is the one place a scene
		// feed cannot copy the venue feed, which hoists a single zone to the whole
		// calendar because every show in it is at one room.
		setVenueLocalEventTimes(event, show.StartsAt, defaultShowDuration,
			optionalString(show.VenueTimezone), show.VenueState)

		applyEventSummaryAndStatus(event, show.Title, show.ArtistNames, show.VenueName,
			show.IsCancelled, show.IsSoldOut)

		// VenueAddress arrives already gated: the scene service serves a street
		// address for verified venues only, so an unverified room's is "" here and
		// formatEventLocation is handed nil rather than an empty segment.
		location := sanitizeICSText(formatEventLocation(
			show.VenueName, optionalString(show.VenueAddress), show.VenueCity, show.VenueState))
		if location != "" {
			event.SetLocation(location)
		}

		showURL := showPageURL(frontendURL, show.Slug, show.ID)
		event.SetDescription(buildEventDescription(
			location, show.ArtistNames, show.Price, revision.AgeRequirement, show.IsCancelled, showURL))
		if showURL != "" {
			event.SetURL(showURL)
		}
	}

	// RFC 5545 3.1 mandates CRLF line endings, and golang-ical defaults to bare
	// LF. Strict clients reject an LF-delimited calendar outright.
	return []byte(cal.Serialize(ics.WithNewLine("\r\n")))
}

// sceneDisplayName is how a scene names itself to a reader: "Phoenix, AZ".
// Matches SceneDayResponse.SceneName so the calendar a subscriber adds carries
// the same name as the page they added it from.
func sceneDisplayName(city, state string) string {
	return city + ", " + state
}

// sceneFeedSlug renders the scene's canonical slug for the download FILENAME.
//
// Cosmetic only: nothing routes, caches or identifies on this value (the cache
// keys on the canonical city/state pair, and the handler resolves the scene from
// the request path). It reproduces catalog.buildSceneSlug's shape so the file a
// subscriber saves is named after the URL they fetched it from; if the two ever
// drift, a filename changes and nothing else does.
func sceneFeedSlug(city, state string) string {
	return strings.ToLower(strings.ReplaceAll(city, " ", "-")) + "-" + strings.ToLower(state)
}

// sceneFeedCacheKey identifies a scene by the canonical (city, state) the slug
// resolved to, using a separator that cannot appear in either half — so
// ("a-b", "c") and ("a", "b-c") can never collide on one entry.
func sceneFeedCacheKey(city, state string) string {
	return city + "\x00" + state
}

// optionalString adapts an empty-string-means-absent field to the pointer form
// the shared calendar helpers take, so an absent value stays absent instead of
// arriving as an empty segment.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// cachedFeed returns a defensive copy of a live cache entry.
func (s *SceneCalendarService) cachedFeed(key string) (*contracts.SceneCalendarFeed, bool) {
	s.cacheMu.RLock()
	entry, ok := s.cache[key]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return cloneSceneFeed(entry.feed), true
}

func (s *SceneCalendarService) storeFeed(key string, feed contracts.SceneCalendarFeed) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if len(s.cache) >= sceneFeedCacheMaxEntries {
		s.cache = make(map[string]sceneFeedCacheEntry, sceneFeedCacheMaxEntries)
	}
	s.cache[key] = sceneFeedCacheEntry{
		feed:      *cloneSceneFeed(feed),
		expiresAt: time.Now().Add(sceneFeedCacheTTL),
	}
}

// cloneSceneFeed copies the payload so a caller can never mutate the cached bytes.
func cloneSceneFeed(feed contracts.SceneCalendarFeed) *contracts.SceneCalendarFeed {
	out := feed
	out.ICS = make([]byte, len(feed.ICS))
	copy(out.ICS, feed.ICS)
	return &out
}
