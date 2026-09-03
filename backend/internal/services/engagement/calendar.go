package engagement

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// icalLocalTimeFormat is RFC 5545's "date with local time" layout
// (golang-ical's unexported icalTimestampFormatLocal). Used for DTSTART/DTEND
// values that carry an explicit TZID parameter instead of a trailing Z.
const icalLocalTimeFormat = "20060102T150405"

// Mirrored by the frontend's SHOW_DURATION_MS (ShowAddToCalendar.tsx) for the
// Google Calendar link — the two export paths must agree on when a show ends.
//
// defaultShowDuration is the assumed length of a show when building calendar
// events (the source data has no end time).
const defaultShowDuration = 3 * time.Hour

// icsFeedCacheTTL is a short per-user cache for ICS payloads. Calendar clients
// poll coarsely; this cuts DB load without serving stale feeds after regenerate
// (cache is keyed by user and cleared on CreateToken/DeleteToken).
const icsFeedCacheTTL = 2 * time.Minute

const (
	// CalendarTokenPrefix is prepended to calendar tokens for identification
	CalendarTokenPrefix = "phcal_"

	// calendarTokenLength is the length of the generated token in bytes (32 bytes = 64 hex chars)
	calendarTokenLength = 32

	// calendarFeedPathPrefix is the canonical public feed path (PSY-1430 / PSY-1505).
	// /calendar/{token} remains as a backward-compatible iCal alias.
	calendarFeedPathPrefix    = "/feeds/"
	calendarFeedPathSuffix    = "/saved-shows.ics"
	followsActivityPathSuffix = "/follows.atom"
)

type icsFeedCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

// CalendarService handles personal feed-token CRUD plus ICS / Atom generation.
// One calendar_tokens row authenticates both /feeds/{token}/saved-shows.ics
// and /feeds/{token}/follows.atom (PSY-1430 / PSY-1505).
type CalendarService struct {
	db            *gorm.DB
	savedShowSvc  contracts.SavedShowServiceInterface
	feedCache     sync.Map // userID (uint) → icsFeedCacheEntry (ICS)
	atomFeedCache sync.Map // userID (uint) → icsFeedCacheEntry (Atom)
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

// calendarFeedURL builds the canonical iCal subscribe URL for a plaintext token.
func calendarFeedURL(apiBaseURL, plainToken string) string {
	return fmt.Sprintf("%s%s%s%s", strings.TrimRight(apiBaseURL, "/"), calendarFeedPathPrefix, plainToken, calendarFeedPathSuffix)
}

// followsActivityFeedURL builds the canonical Atom subscribe URL for a plaintext token.
func followsActivityFeedURL(apiBaseURL, plainToken string) string {
	return fmt.Sprintf("%s%s%s%s", strings.TrimRight(apiBaseURL, "/"), calendarFeedPathPrefix, plainToken, followsActivityPathSuffix)
}

func (s *CalendarService) invalidateFeedCache(userID uint) {
	s.feedCache.Delete(userID)
	s.atomFeedCache.Delete(userID)
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

	s.invalidateFeedCache(userID)

	// Fetch the created token to get the server-set created_at
	var created engagementm.CalendarToken
	if err := s.db.Where("user_id = ?", userID).First(&created).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch created token: %w", err)
	}

	return &contracts.CalendarTokenCreateResponse{
		Token:          plainToken,
		FeedURL:        calendarFeedURL(apiBaseURL, plainToken),
		FollowsFeedURL: followsActivityFeedURL(apiBaseURL, plainToken),
		CreatedAt:      created.CreatedAt,
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
	s.invalidateFeedCache(userID)
	return nil
}

// ValidateCalendarToken validates a plaintext calendar token and returns the associated user.
// Lookup hashes the candidate then constant-time-compares against the stored hash so a
// successful match path does not short-circuit on string equality (PSY-1430).
func (s *CalendarService) ValidateCalendarToken(plainToken string) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	tokenHash := hashCalendarToken(plainToken)

	var token engagementm.CalendarToken
	err := s.db.Preload("User").Where("token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		// Dummy compare so miss-path work is closer to hit-path (hash already
		// computed; compare against itself to keep timing shape similar).
		_ = subtle.ConstantTimeCompare([]byte(tokenHash), []byte(tokenHash))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("invalid calendar token")
		}
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	if subtle.ConstantTimeCompare([]byte(token.TokenHash), []byte(tokenHash)) != 1 {
		return nil, fmt.Errorf("invalid calendar token")
	}

	if !token.User.IsActive {
		return nil, fmt.Errorf("user account is not active")
	}

	return &token.User, nil
}

// GenerateICSFeed creates an ICS calendar feed for a user's saved upcoming shows.
//
// The venue addresses that reach the feed's LOCATION are already gated: the
// shows come from SavedShowService.GetUserSavedShows, whose venue builder
// applies Venue.PublicAddress, so an unverified venue arrives with a nil
// Address. That is load-bearing rather than incidental. The feed URL carries a
// bearer token instead of a session, so anyone holding the link reads it, and
// an ICS LOCATION is copied onto the subscriber's device where a later
// redaction can never reach it. See formatEventLocation for the rule that binds
// every calendar caller.
func (s *CalendarService) GenerateICSFeed(userID uint, frontendURL string) ([]byte, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if cached, ok := s.feedCache.Load(userID); ok {
		entry := cached.(icsFeedCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			out := make([]byte, len(entry.data))
			copy(out, entry.data)
			return out, nil
		}
		s.feedCache.Delete(userID)
	}

	// Upcoming only — venue-local date ≥ today (PSY-1430).
	shows, _, err := s.savedShowSvc.GetUserSavedShows(userID, 500, 0, "upcoming")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch saved shows: %w", err)
	}

	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	cal.SetProductId("-//Psychic Homily//Calendar Feed//EN")
	cal.SetName("Psychic Homily - My Shows")
	cal.SetDescription("Your saved shows from Psychic Homily")
	cal.SetXWRCalName("Psychic Homily - My Shows")

	for _, show := range shows {
		if show.Status != "approved" {
			continue
		}
		if show.IsCancelled {
			continue
		}

		// Shared with the public calendar surfaces (venue_calendar.go): the same
		// show reached through any of them must be the same calendar event, so
		// the UID format lives in one place rather than being spelled out per
		// surface and drifting.
		event := cal.AddEvent(showEventUID(show.ID))
		event.SetCreatedTime(show.CreatedAt)
		event.SetModifiedAt(show.UpdatedAt)

		// Times, naming, location, description and link are assembled by the
		// helper the per-show download also uses, rather than by a local copy of
		// the same sequence. That is what puts every community-editable value
		// through sanitizeICSText (see its doc for why an unsanitized CR is a
		// property-injection vector).
		//
		// The helper's cancelled branch is unreachable from here: the loop skips
		// cancelled shows above. Calling the shared helper anyway keeps this feed
		// from re-deciding how an event is named, and stays correct if that
		// filter is ever relaxed.
		applyShowEventContent(event, &show.ShowResponse, frontendURL)
	}

	// REMAINING DIVERGENCE from the two sibling feeds, recorded so the next
	// reader does not mistake any of it for an oversight. The safety gap is
	// closed above: every community-editable value now passes through
	// sanitizeICSText, which golang-ical's TEXT escaper does not do for CR.
	// What is still different:
	//
	//   - No DTSTAMP per VEVENT, though RFC 5545 requires one under
	//     METHOD:PUBLISH.
	//   - Bare LF line endings, though RFC 5545 3.1 mandates CRLF and strict
	//     clients reject an LF-delimited calendar.
	//   - No SEQUENCE. This one is not merely cosmetic: the surfaces share a
	//     UID per show, and RFC 5546 3.2 resolves a UID collision in favour of
	//     the higher SEQUENCE, so a subscriber to both this feed and a public
	//     one always sees the public copy win.
	//   - feedCache has no entry cap and expires only lazily on that user's next
	//     request, where the venue feed bounds itself at
	//     venueFeedCacheMaxEntries. Bounded in practice by the number of issued
	//     tokens, so it is capacity planning rather than an attack surface.
	//
	// Each is a wire-format or memory change wanting its own test, so they are
	// tracked as their own work rather than folded into a security fix.
	data := []byte(cal.Serialize())
	cachedCopy := make([]byte, len(data))
	copy(cachedCopy, data)
	s.feedCache.Store(userID, icsFeedCacheEntry{
		data:      cachedCopy,
		expiresAt: time.Now().Add(icsFeedCacheTTL),
	})
	return data, nil
}

// setVenueLocalEventTimes writes DTSTART/DTEND anchored to the venue's local
// timezone instead of UTC. A show happens at a fixed wall-clock time in the
// venue's city, so the calendar event must read e.g. "8:00 PM" for every
// subscriber regardless of where they live — a bare UTC DTSTART would be
// silently re-shifted into the viewer's own zone by their calendar client.
//
// We emit DTSTART;TZID=<IANA>:<local time> using the venue's IANA zone
// (PSY-985 geocoding, falling back to the legacy state map via
// utils.EventLocation). We deliberately do NOT emit a VTIMEZONE component:
// golang-ical cannot synthesize the DST transition RRULEs a correct VTIMEZONE
// needs, and a hand-rolled partial one would be less accurate than letting the
// client resolve the well-known IANA TZID against its own bundled tz database
// (Google Calendar, Apple Calendar, and modern Outlook all do this). If no
// usable zone resolves (loc collapses to UTC) we fall back to the prior UTC
// instant. (PSY-987)
//
// A room whose zone the site cannot NAME (no stored venues.timezone, and a state
// outside the US map, which is what a non-US room without a geocode looks like)
// gets an ALL-DAY event instead: DTSTART;VALUE=DATE on the fallback's reading of
// the calendar day, DTEND the day after it (RFC 5545 makes a DATE-valued DTEND
// exclusive). A TZID form here would state a wall clock derived from the Arizona
// default, and an ICS is copied onto a subscriber's device, so that guess would
// outlive any later correction to the venue. The DAY is still the best available
// answer, so it is still published; a three-hour duration cannot be expressed
// against a DATE-valued DTSTART and is dropped with the hour it belonged to.
func setVenueLocalEventTimes(event *ics.VEvent, start time.Time, duration time.Duration, venueTimezone *string, venueState string) {
	loc, resolved := shared.EventLocationResolved(venueTimezone, venueState)

	if !resolved {
		localStart := start.In(loc)
		event.SetAllDayStartAt(localStart)
		event.SetAllDayEndAt(localStart.AddDate(0, 0, 1))
		return
	}

	tzid := loc.String()
	if tzid == "" || tzid == "UTC" {
		event.SetStartAt(start)
		event.SetEndAt(start.Add(duration))
		return
	}

	localStart := start.In(loc)
	localEnd := localStart.Add(duration)
	event.SetProperty(ics.ComponentPropertyDtStart, localStart.Format(icalLocalTimeFormat), ics.WithTZID(tzid))
	event.SetProperty(ics.ComponentPropertyDtEnd, localEnd.Format(icalLocalTimeFormat), ics.WithTZID(tzid))
}
