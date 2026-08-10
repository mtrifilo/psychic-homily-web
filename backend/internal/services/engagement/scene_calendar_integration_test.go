package engagement

import (
	"fmt"
	"strings"
	"testing"
	"time"

	// Embed the IANA tz database so LoadLocation works in any CI image.
	_ "time/tzdata"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	catalogsvc "psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/testutil"
)

// The unit tests cover rendering. These cover the half they cannot: WHICH ROWS
// reach the renderer. That is where a scene feed silently goes wrong — a wrong
// scope publishes another city's shows, a dropped redaction publishes a house
// address, and no rendering test would notice either.
//
// The fixtures use a fallback scene ("Testville, ZZ"): ZZ pins no CBSA, so the
// scope stays literal city/state and the tests do not depend on the geocoder.
//
// One container for the whole suite, cleaned between cases, rather than the
// per-test container the venue feed's tests use: the scoping cases need several
// venues each, and five cold Postgres starts is minutes of CI for fixtures that
// differ only in their rows.

const sceneIntegrationSlug = "testville-zz"

type SceneCalendarFeedSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *SceneCalendarService
}

func TestSceneCalendarFeedSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(SceneCalendarFeedSuite))
}

func (s *SceneCalendarFeedSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *SceneCalendarFeedSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *SceneCalendarFeedSuite) SetupTest() {
	// A fresh service per test, so no case can be served a previous case's
	// cached feed.
	s.svc = NewSceneCalendarService(s.db, catalogsvc.NewSceneService(s.db))
}

func (s *SceneCalendarFeedSuite) TearDownTest() {
	for _, table := range []string{"show_artists", "show_venues", "shows", "artists", "venues", "scenes"} {
		s.db.Exec("DELETE FROM " + table)
	}
}

// --- seed helpers ---

// seedScene creates the two verified venues a scene needs to clear the existence
// gate, plus an unverified room in the same city. Returns (verified, unverified).
func (s *SceneCalendarFeedSuite) seedScene() (*catalogm.Venue, *catalogm.Venue) {
	tz := "America/Phoenix"
	address := "2303 E Indian School Rd"
	verified := s.seedVenue("Scene Feed Room", "Testville", "ZZ", &tz, &address, true)
	s.seedVenue("Second Room", "Testville", "ZZ", &tz, nil, true)

	secretAddress := "1400 Secret House Show Ln"
	unverified := s.seedVenue("Someone's Basement", "Testville", "ZZ", &tz, &secretAddress, false)

	return verified, unverified
}

func (s *SceneCalendarFeedSuite) seedVenue(name, city, state string, tz, address *string, verified bool) *catalogm.Venue {
	slug := fmt.Sprintf("%s-%d", strings.ToLower(strings.ReplaceAll(name, " ", "-")), time.Now().UnixNano())
	venue := &catalogm.Venue{Name: name, Slug: &slug, City: city, State: state, Timezone: tz, Address: address}
	s.Require().NoError(s.db.Create(venue).Error)
	if verified {
		// GORM writes no column for a false bool on Create, so `verified` has to
		// be set in a follow-up update rather than on the struct.
		s.Require().NoError(s.db.Model(venue).Update("verified", true).Error)
		venue.Verified = true
	}
	return venue
}

func (s *SceneCalendarFeedSuite) seedShow(title string, at time.Time, status catalogm.ShowStatus, cancelled bool, venue *catalogm.Venue) *catalogm.Show {
	slug := fmt.Sprintf("scene-feed-%d", time.Now().UnixNano())
	show := &catalogm.Show{Title: title, Slug: &slug, EventDate: at, Status: status, IsCancelled: cancelled}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)
	return show
}

func (s *SceneCalendarFeedSuite) generate() (string, string, string) {
	feed, err := s.svc.GenerateSceneFeed(sceneIntegrationSlug, "https://psychichomily.com")
	s.Require().NoError(err)
	return string(feed.ICS), feed.SceneName, feed.SceneSlug
}

// --- cases ---

func (s *SceneCalendarFeedSuite) TestSelectsTheRightShows() {
	venue, _ := s.seedScene()

	tz := "America/Phoenix"
	elsewhere := s.seedVenue("Elsewhere Room", "Otherville", "ZZ", &tz, nil, true)
	s.seedVenue("Elsewhere Room Two", "Otherville", "ZZ", &tz, nil, true)

	headliner := &catalogm.Artist{Name: "Scene Feed Headliner"}
	support := &catalogm.Artist{Name: "Scene Feed Support"}
	s.Require().NoError(s.db.Create(headliner).Error)
	s.Require().NoError(s.db.Create(support).Error)

	now := time.Now().UTC()
	upcoming := s.seedShow("Upcoming Show", now.Add(7*24*time.Hour), catalogm.ShowStatusApproved, false, venue)
	cancelled := s.seedShow("Cancelled Show", now.Add(10*24*time.Hour), catalogm.ShowStatusApproved, true, venue)
	past := s.seedShow("Past Show", now.Add(-30*24*time.Hour), catalogm.ShowStatusApproved, false, venue)
	pending := s.seedShow("Pending Show", now.Add(9*24*time.Hour), catalogm.ShowStatusPending, false, venue)
	otherCity := s.seedShow("Other City Show", now.Add(8*24*time.Hour), catalogm.ShowStatusApproved, false, elsewhere)
	// Past the bounded window — the thing that makes a scene feed different from
	// the venue feed's unbounded "everything upcoming".
	beyond := s.seedShow("Far Future Show", now.AddDate(0, 0, sceneFeedWindowDays+7), catalogm.ShowStatusApproved, false, venue)

	// Billing order must come from show_artists.position, not artist id order.
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{ShowID: upcoming.ID, ArtistID: support.ID, Position: 1, SetType: "support"}).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowArtist{ShowID: upcoming.ID, ArtistID: headliner.ID, Position: 0, SetType: "headliner"}).Error)

	body, sceneName, sceneSlug := s.generate()

	parsed, err := ics.ParseCalendar(strings.NewReader(body))
	s.Require().NoError(err)

	uids := map[string]bool{}
	for _, event := range parsed.Events() {
		uids[event.GetProperty(ics.ComponentPropertyUniqueId).Value] = true
	}

	s.True(uids[showEventUID(upcoming.ID)], "an upcoming approved show must be in the feed")
	s.True(uids[showEventUID(cancelled.ID)],
		"a cancelled upcoming show must stay in the feed so STATUS:CANCELLED reaches subscribers")
	s.False(uids[showEventUID(past.ID)], "past shows must not be in an upcoming feed")
	s.False(uids[showEventUID(pending.ID)], "unapproved shows must never be published")
	s.False(uids[showEventUID(otherCity.ID)], "another scene's show must not leak into this feed")
	s.False(uids[showEventUID(beyond.ID)], "a show past the window must not be published")

	s.Contains(body, "STATUS:CANCELLED")
	s.Contains(body, "DTSTART;TZID=America/Phoenix:")
	s.Contains(body, "Scene Feed Room")
	s.Contains(unfoldScene(body), `Artists: Scene Feed Headliner\, Scene Feed Support`,
		"billing must follow show_artists.position, not artist id")

	// DTSTAMP/SEQUENCE come from the supplemental revision query, so a broken
	// query shows up here as a zero timestamp rather than as a missing feed.
	s.NotContains(body, "DTSTAMP:00010101T000000Z")
	s.Contains(body, "SEQUENCE:")

	s.Equal("Testville, ZZ", sceneName)
	s.Equal("testville-zz", sceneSlug)
}

// Unverified venues are DIY spaces and houses; their street address is withheld
// everywhere else in the API and must be withheld here too. A public calendar
// feed is a WORSE place to leak it than a page — the address ends up copied into
// every subscriber's device, where a later redaction can never reach it. This is
// why the service reads shows through the scene service (whose query applies the
// gate) instead of joining venues itself.
func (s *SceneCalendarFeedSuite) TestNeverPublishesAnUnverifiedVenueAddress() {
	verified, unverified := s.seedScene()

	now := time.Now().UTC()
	s.seedShow("Basement Gig", now.Add(3*24*time.Hour), catalogm.ShowStatusApproved, false, unverified)
	s.seedShow("Club Gig", now.Add(4*24*time.Hour), catalogm.ShowStatusApproved, false, verified)

	raw, _, _ := s.generate()
	// Unfolded, because a folded address would slip past a raw substring check.
	body := unfoldScene(raw)

	s.NotContains(body, "1400 Secret House Show Ln",
		"an unverified venue's street address must never reach a public feed")
	s.NotContains(body, "Secret House Show")
	// The room is still listed — only the address is withheld.
	s.Contains(body, "Basement Gig")
	s.Contains(body, "LOCATION:Someone's Basement\\, Testville\\, ZZ")
	// ...while a verified room still publishes its address, so the assertions
	// above prove redaction rather than an address that never rendered at all.
	s.Contains(body, "2303 E Indian School Rd")
}

// A slug that names no scene, and a city with one verified venue but not enough
// to be a scene, must both surface a scene error so the handler 404s instead of
// 500ing.
func (s *SceneCalendarFeedSuite) TestUnknownSceneIsNotFound() {
	s.seedVenue("Lonely Room", "Thinville", "ZZ", nil, nil, true)

	for _, slug := range []string{"no-such-scene-zz", "thinville-zz"} {
		_, err := s.svc.GenerateSceneFeed(slug, "https://psychichomily.com")
		s.Require().Error(err, "slug %q", slug)

		var sceneErr *apperrors.SceneError
		s.Require().ErrorAs(err, &sceneErr, "slug %q must surface a scene error so the handler can 404", slug)
		s.Equal(apperrors.CodeSceneNotFound, sceneErr.Code)
	}
}

// A real scene with a quiet stretch is a fact, not an error: it serves a valid
// empty calendar so an already-subscribed client does not treat it as a dead URL.
func (s *SceneCalendarFeedSuite) TestQuietSceneServesAnEmptyCalendar() {
	venue, _ := s.seedScene()
	s.seedShow("Long Gone", time.Now().UTC().Add(-90*24*time.Hour), catalogm.ShowStatusApproved, false, venue)

	body, _, _ := s.generate()

	parsed, err := ics.ParseCalendar(strings.NewReader(body))
	s.Require().NoError(err)
	s.Empty(parsed.Events())
	s.Contains(body, "X-WR-CALNAME:Shows in Testville\\, ZZ")
	s.Contains(body, "END:VCALENDAR")
}
