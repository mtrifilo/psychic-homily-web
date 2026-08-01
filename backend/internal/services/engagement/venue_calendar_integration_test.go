package engagement

import (
	"strings"
	"testing"
	"time"

	// Embed the IANA tz database so LoadLocation works in any CI image.
	_ "time/tzdata"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	catalogsvc "psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/testutil"
)

// The unit tests above cover rendering. This one covers the half they cannot:
// the QUERY. Which rows reach the renderer is where a venue feed silently goes
// wrong — a cancelled show filtered out at the SQL level would make
// STATUS:CANCELLED unreachable no matter how correct the rendering is, and no
// unit test would notice.
func TestGenerateVenueFeed_SelectsTheRightShows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	tz := "America/Phoenix"
	address := "2303 E Indian School Rd"
	venueSlug := "feed-test-room-phoenix-az"
	venue := &catalogm.Venue{
		Name:     "Feed Test Room",
		Slug:     &venueSlug,
		City:     "Phoenix",
		State:    "AZ",
		Timezone: &tz,
		Address:  &address,
		Verified: true,
	}
	require.NoError(t, td.DB.Create(venue).Error)

	otherVenue := &catalogm.Venue{Name: "Some Other Room", City: "Phoenix", State: "AZ", Timezone: &tz}
	require.NoError(t, td.DB.Create(otherVenue).Error)

	headliner := &catalogm.Artist{Name: "Feed Headliner"}
	support := &catalogm.Artist{Name: "Feed Support"}
	require.NoError(t, td.DB.Create(headliner).Error)
	require.NoError(t, td.DB.Create(support).Error)

	now := time.Now()
	seedShow := func(title string, at time.Time, status catalogm.ShowStatus, cancelled bool, atVenue *catalogm.Venue) *catalogm.Show {
		t.Helper()
		show := &catalogm.Show{
			Title:       title,
			EventDate:   at,
			Status:      status,
			IsCancelled: cancelled,
		}
		require.NoError(t, td.DB.Create(show).Error)
		require.NoError(t, td.DB.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: atVenue.ID}).Error)
		return show
	}

	upcoming := seedShow("Upcoming Show", now.Add(7*24*time.Hour), catalogm.ShowStatusApproved, false, venue)
	cancelled := seedShow("Cancelled Show", now.Add(10*24*time.Hour), catalogm.ShowStatusApproved, true, venue)
	past := seedShow("Past Show", now.Add(-30*24*time.Hour), catalogm.ShowStatusApproved, false, venue)
	pending := seedShow("Pending Show", now.Add(9*24*time.Hour), catalogm.ShowStatusPending, false, venue)
	elsewhere := seedShow("Show Elsewhere", now.Add(8*24*time.Hour), catalogm.ShowStatusApproved, false, otherVenue)

	// Billing order must come from show_artists.position, not artist id order.
	require.NoError(t, td.DB.Create(&catalogm.ShowArtist{ShowID: upcoming.ID, ArtistID: support.ID, Position: 1, SetType: "support"}).Error)
	require.NoError(t, td.DB.Create(&catalogm.ShowArtist{ShowID: upcoming.ID, ArtistID: headliner.ID, Position: 0, SetType: "headliner"}).Error)

	svc := NewVenueCalendarService(td.DB, catalogsvc.NewVenueService(td.DB))

	feed, err := svc.GenerateVenueFeed(venueSlug, "https://psychichomily.com")
	require.NoError(t, err)

	parsed, err := ics.ParseCalendar(strings.NewReader(string(feed.ICS)))
	require.NoError(t, err)

	uids := map[string]bool{}
	for _, event := range parsed.Events() {
		uids[event.GetProperty(ics.ComponentPropertyUniqueId).Value] = true
	}

	assert.True(t, uids[showEventUID(upcoming.ID)], "an upcoming approved show must be in the feed")
	assert.True(t, uids[showEventUID(cancelled.ID)],
		"a cancelled upcoming show must stay in the feed so STATUS:CANCELLED reaches subscribers")
	assert.False(t, uids[showEventUID(past.ID)], "past shows must not be in an upcoming feed")
	assert.False(t, uids[showEventUID(pending.ID)], "unapproved shows must never be published")
	assert.False(t, uids[showEventUID(elsewhere.ID)], "another venue's show must not leak into this feed")

	body := string(feed.ICS)
	assert.Contains(t, body, "STATUS:CANCELLED")
	assert.Contains(t, body, "DTSTART;TZID=America/Phoenix:")
	assert.Contains(t, body, "Feed Test Room")

	unfolded := strings.ReplaceAll(body, "\r\n ", "")
	assert.Contains(t, unfolded, `Artists: Feed Headliner\, Feed Support`,
		"billing must follow show_artists.position, not artist id")

	// The ETag must be a real content hash, and the feed must be servable.
	assert.NotEmpty(t, feed.ETag)
	assert.Equal(t, venue.Name, feed.VenueName)
}

func TestGenerateVenueFeed_UnknownVenueIsNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	svc := NewVenueCalendarService(td.DB, catalogsvc.NewVenueService(td.DB))

	for _, identifier := range []string{"no-such-venue", "999999"} {
		_, err := svc.GenerateVenueFeed(identifier, "https://psychichomily.com")
		require.Error(t, err, "identifier %q", identifier)

		var venueErr *apperrors.VenueError
		require.ErrorAs(t, err, &venueErr, "identifier %q must surface a venue error so the handler can 404", identifier)
		assert.Equal(t, apperrors.CodeVenueNotFound, venueErr.Code)
	}
}

// Unverified venues are DIY spaces and houses; their street address is withheld
// everywhere else in the API and must be withheld here too. A public calendar
// feed is a WORSE place to leak it than a page — the address ends up copied into
// every subscriber's device. This is the reason the service reads the venue
// through VenueServiceInterface instead of querying the venues table directly.
func TestGenerateVenueFeed_NeverPublishesAnUnverifiedVenueAddress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	tz := "America/Phoenix"
	secretAddress := "1400 Secret House Show Ln"
	slug := "someones-basement-phoenix-az"
	venue := &catalogm.Venue{
		Name:     "Someone's Basement",
		Slug:     &slug,
		City:     "Phoenix",
		State:    "AZ",
		Timezone: &tz,
		Address:  &secretAddress,
		Verified: false, // the whole point
	}
	require.NoError(t, td.DB.Create(venue).Error)

	show := &catalogm.Show{
		Title:     "Basement Gig",
		EventDate: time.Now().Add(3 * 24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	require.NoError(t, td.DB.Create(show).Error)
	require.NoError(t, td.DB.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)

	svc := NewVenueCalendarService(td.DB, catalogsvc.NewVenueService(td.DB))
	feed, err := svc.GenerateVenueFeed(slug, "https://psychichomily.com")
	require.NoError(t, err)

	assert.NotContains(t, string(feed.ICS), secretAddress,
		"an unverified venue's street address must never reach a public feed")
	assert.NotContains(t, string(feed.ICS), "Secret House Show")
	// The venue is still listed — only the address is withheld.
	assert.Contains(t, string(feed.ICS), "Basement Gig")
	assert.Contains(t, string(feed.ICS), "LOCATION:Someone's Basement\\, Phoenix\\, AZ")
}

// A venue whose row has no explicit IANA zone must still anchor to the state
// map rather than quietly falling back to UTC.
func TestGenerateVenueFeed_UsesStateFallbackWhenTimezoneNull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	noTzSlug := "no-timezone-room-chicago-il"
	venue := &catalogm.Venue{Name: "No Timezone Room", Slug: &noTzSlug, City: "Chicago", State: "IL", Verified: true}
	require.NoError(t, td.DB.Create(venue).Error)

	show := &catalogm.Show{
		Title:     "Windy City Gig",
		EventDate: time.Now().Add(5 * 24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	require.NoError(t, td.DB.Create(show).Error)
	require.NoError(t, td.DB.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)

	svc := NewVenueCalendarService(td.DB, catalogsvc.NewVenueService(td.DB))
	feed, err := svc.GenerateVenueFeed(noTzSlug, "https://psychichomily.com")
	require.NoError(t, err)

	assert.Contains(t, string(feed.ICS), "DTSTART;TZID=America/Chicago:",
		"a NULL venues.timezone must fall back to the state map, not UTC")
}
