package engagement

import (
	"fmt"
	"strings"
	"testing"
	"time"

	// Embed the IANA tz database so LoadLocation works in any CI image.
	// Test-only — not linked into the server.
	_ "time/tzdata"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestGenerateCalendarToken_Format(t *testing.T) {
	token, err := generateCalendarToken()
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, CalendarTokenPrefix), "token should start with %s", CalendarTokenPrefix)
	// prefix "phcal_" (6) + hex-encoded 32 bytes (64) = 70 chars
	assert.Len(t, token, 6+64, "token should be prefix(6) + hex(64) = 70 chars")
}

func TestGenerateCalendarToken_Unique(t *testing.T) {
	token1, err1 := generateCalendarToken()
	token2, err2 := generateCalendarToken()
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, token1, token2, "two generated tokens should be different")
}

func TestGenerateICSFeed_Format(t *testing.T) {
	// Create a service with a mock saved show service
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        1,
				Slug:      "test-show",
				Title:     "Test Show",
				EventDate: time.Now().Add(24 * time.Hour),
				City:      ptrString("Phoenix"),
				State:     ptrString("AZ"),
				Status:    "approved",
				Venues: []contracts.VenueResponse{
					{
						ID:      1,
						Name:    "The Venue",
						Address: ptrString("123 Main St"),
						City:    "Phoenix",
						State:   "AZ",
					},
				},
				Artists: []contracts.ArtistResponse{
					{ID: 1, Name: "Artist One"},
					{ID: 2, Name: "Artist Two"},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.NotNil(t, data)
	assert.Equal(t, "upcoming", mockSvc.lastTimeFilter)

	icsStr := string(data)
	assert.Contains(t, icsStr, "BEGIN:VCALENDAR")
	assert.Contains(t, icsStr, "END:VCALENDAR")
	assert.Contains(t, icsStr, "BEGIN:VEVENT")
	assert.Contains(t, icsStr, "END:VEVENT")
	assert.Contains(t, icsStr, "SUMMARY:Test Show")
	assert.Contains(t, icsStr, "show-1@psychichomily.com")
	// ICS escapes commas per RFC 5545
	assert.Contains(t, icsStr, "The Venue\\, 123 Main St\\, Phoenix\\, AZ")
	// ICS uses line folding (CRLF + space) for long DESCRIPTION lines
	unfolded := strings.ReplaceAll(strings.ReplaceAll(icsStr, "\r\n ", ""), "\n ", "")
	assert.Contains(t, unfolded, "Artist One")
	assert.Contains(t, unfolded, "Artist Two")
	assert.Contains(t, icsStr, "https://psychichomily.com/shows/test-show")
	assert.Contains(t, icsStr, "METHOD:PUBLISH")

	parsed, err := ics.ParseCalendar(strings.NewReader(icsStr))
	assert.NoError(t, err)
	assert.Len(t, parsed.Events(), 1)
	assert.Equal(t, "Test Show", parsed.Events()[0].GetProperty(ics.ComponentPropertySummary).Value)
}

// The advance/door split has to reach the DESCRIPTION, not just the helper that
// formats it (PSY-1962). A calendar entry sits in a subscriber's phone until the
// show happens, so a price that under-reports there is the one a reader budgets
// against and finds wrong at the door.
//
// Through GenerateICSFeed rather than through ShowPriceText, because the wiring
// is what was missing: the helper had tests from the day it was written while
// buildEventDescription's price line had none, in either shape.
func TestGenerateICSFeed_CarriesTheAdvanceDoorSplit(t *testing.T) {
	show := func(price, doorPrice *float64) *contracts.SavedShowResponse {
		return &contracts.SavedShowResponse{
			ShowResponse: contracts.ShowResponse{
				ID: 1, Slug: "test-show", Title: "Test Show",
				EventDate: time.Now().Add(24 * time.Hour),
				Status:    "approved",
				Price:     price,
				DoorPrice: doorPrice,
			},
		}
	}
	price := func(v float64) *float64 { return &v }

	describe := func(s *contracts.SavedShowResponse) string {
		svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: &mockSavedShowSvc{
			shows: []*contracts.SavedShowResponse{s}, total: 1,
		}}
		data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
		assert.NoError(t, err)
		// Unfold RFC 5545 line folding before matching, or a long DESCRIPTION
		// splits the price across a CRLF and the assertion misses it.
		return strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n ", ""), "\n ", "")
	}

	assert.Contains(t, describe(show(price(35), price(40))), "Price: $35/$40",
		"a split price must reach the calendar, not just its advance half")
	assert.Contains(t, describe(show(nil, price(15))), "Price: $15",
		"a door-only show has a known price and must not read as priceless")
	assert.Contains(t, describe(show(price(0), nil)), "Price: Free",
		"a free show says Free, the same word the site prints, not $0")
	assert.NotContains(t, describe(show(nil, nil)), "Price:",
		"a show with no recorded price claims nothing")
}

func TestGenerateICSFeed_VenueLocalTime(t *testing.T) {
	nyLoc, err := time.LoadLocation("America/New_York")
	assert.NoError(t, err)
	nyTzid := "America/New_York"

	// A future instant so the 30-day-past cutoff never filters it, regardless
	// of the wall clock running the test.
	eventDate := time.Now().Add(48 * time.Hour)
	expectedLocal := eventDate.In(nyLoc).Format("20060102T150405")
	expectedEnd := eventDate.In(nyLoc).Add(defaultShowDuration).Format("20060102T150405")

	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        7,
				Slug:      "ny-show",
				Title:     "NY Show",
				EventDate: eventDate,
				Status:    "approved",
				Venues: []contracts.VenueResponse{
					{ID: 1, Name: "Brooklyn Steel", City: "Brooklyn", State: "NY", Timezone: ptrString(nyTzid)},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)

	icsStr := string(data)
	// DTSTART/DTEND must carry the venue's IANA TZID and the venue-local time,
	// not a bare UTC instant (which a calendar client would re-shift into the
	// viewer's own zone).
	assert.Contains(t, icsStr, "DTSTART;TZID="+nyTzid+":"+expectedLocal)
	assert.Contains(t, icsStr, "DTEND;TZID="+nyTzid+":"+expectedEnd)
	// No floating/UTC DTSTART form for this event.
	assert.NotContains(t, icsStr, "DTSTART:"+eventDate.UTC().Format("20060102T150405")+"Z")
}

// A room whose zone the site cannot name gets an ALL-DAY event. The TZID form
// would state a wall clock derived from the Arizona default, and an ICS is
// copied onto a subscriber's device, so the guess would outlive any later
// correction to the venue.
func TestGenerateICSFeed_UnresolvedZoneIsAllDay(t *testing.T) {
	fallbackLoc, err := time.LoadLocation("America/Phoenix")
	assert.NoError(t, err)

	eventDate := time.Now().Add(48 * time.Hour)
	localDay := eventDate.In(fallbackLoc)

	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        9,
				Slug:      "windmill-show",
				Title:     "Windmill Show",
				EventDate: eventDate,
				Status:    "approved",
				Venues: []contracts.VenueResponse{
					{ID: 2, Name: "The Windmill", City: "London", State: "England"},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)

	icsStr := string(data)
	assert.Contains(t, icsStr, "DTSTART;VALUE=DATE:"+localDay.Format("20060102"))
	assert.Contains(t, icsStr, "DTEND;VALUE=DATE:"+localDay.AddDate(0, 0, 1).Format("20060102"))
	// Nothing anywhere in the feed names an hour for this show.
	assert.NotContains(t, icsStr, "DTSTART;TZID=")
	assert.NotContains(t, icsStr, "DTSTART:"+eventDate.UTC().Format("20060102T150405")+"Z")
	// Identity and revision properties keep their ordinary semantics, so a client
	// that already synced this show updates the event it holds rather than
	// gaining a second one.
	assert.Contains(t, icsStr, "UID:show-9@psychichomily.com")
	assert.Contains(t, icsStr, "CREATED:")
	assert.Contains(t, icsStr, "LAST-MODIFIED:")
}

func TestGenerateICSFeed_SoldOutLabel(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        2,
				Slug:      "sold-out-show",
				Title:     "Hot Show",
				EventDate: time.Now().Add(48 * time.Hour),
				Status:    "approved",
				IsSoldOut: true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.Contains(t, string(data), "SUMMARY:Hot Show [SOLD OUT]")
}

func TestGenerateICSFeed_FiltersCancelled(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:          3,
				Title:       "Cancelled Show",
				EventDate:   time.Now().Add(24 * time.Hour),
				Status:      "approved",
				IsCancelled: true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "Cancelled Show")
	assert.NotContains(t, string(data), "BEGIN:VEVENT")
}

func TestGenerateICSFeed_FiltersNonApproved(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        4,
				Title:     "Pending Show",
				EventDate: time.Now().Add(24 * time.Hour),
				Status:    "pending",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "Pending Show")
}

func TestGenerateICSFeed_RequestsUpcomingFilter(t *testing.T) {
	// Past shows are filtered by GetUserSavedShows(time_filter=upcoming);
	// GenerateICSFeed must request that filter rather than applying a local cutoff.
	mockSvc := &mockSavedShowSvc{shows: []*contracts.SavedShowResponse{}, total: 0}
	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	_, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.Equal(t, "upcoming", mockSvc.lastTimeFilter)
}

func TestGenerateICSFeed_EmptyList(t *testing.T) {
	mockSvc := &mockSavedShowSvc{shows: []*contracts.SavedShowResponse{}, total: 0}
	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.Contains(t, string(data), "BEGIN:VCALENDAR")
	assert.NotContains(t, string(data), "BEGIN:VEVENT")
}

func TestGenerateICSFeed_FallbackToID(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        42,
				Slug:      "", // no slug
				Title:     "No Slug Show",
				EventDate: time.Now().Add(24 * time.Hour),
				Status:    "approved",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.Contains(t, string(data), "https://psychichomily.com/shows/42")
}

// The personal feed shares its per-event assembly with the per-show download,
// so it inherits that surface's naming rule and STATUS. Asserted here and not
// only on the helper, because the three surfaces emit the same UID per show:
// if this feed ever stopped agreeing with the others on an event's name, a
// subscriber to two of them would watch the title flip.
func TestGenerateICSFeed_NamesUntitledShowAfterBillAndConfirmsStatus(t *testing.T) {
	show := func(mutate func(*contracts.ShowResponse)) []*contracts.SavedShowResponse {
		s := contracts.ShowResponse{
			ID:        9,
			Slug:      "untitled-show",
			EventDate: time.Now().Add(24 * time.Hour),
			Status:    "approved",
			Venues:    []contracts.VenueResponse{{ID: 1, Name: "The Rebel Lounge", City: "Phoenix", State: "AZ"}},
			Artists:   []contracts.ArtistResponse{{ID: 1, Name: "AJJ"}, {ID: 2, Name: "Calexico"}},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if mutate != nil {
			mutate(&s)
		}
		return []*contracts.SavedShowResponse{{ShowResponse: s}}
	}
	render := func(shows []*contracts.SavedShowResponse) string {
		svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: &mockSavedShowSvc{shows: shows, total: 1}}
		data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
		require.NoError(t, err)
		return string(data)
	}

	titled := render(show(func(s *contracts.ShowResponse) { s.Title = "Desert Doom Night" }))
	assert.Contains(t, unfoldICS(titled), "SUMMARY:Desert Doom Night")
	assert.Contains(t, unfoldICS(titled), "STATUS:CONFIRMED",
		"an uncancelled event carries the machine-readable confirmed status")

	untitled := render(show(nil))
	assert.Contains(t, unfoldICS(untitled), "SUMMARY:AJJ\\, Calexico",
		"an untitled show is named after its bill, as on the other calendar surfaces")

	bare := render(show(func(s *contracts.ShowResponse) { s.Artists = nil }))
	assert.Contains(t, unfoldICS(bare), "SUMMARY:Show at The Rebel Lounge")
}

// icsContentLines unfolds the feed and splits it into RFC 5545 content lines,
// so a test can count REAL properties instead of substring-matching wire bytes.
// Folding (CRLF + one space/tab) can split a property name away from its value,
// which makes a naive "the payload must not contain X" assertion pass while X
// is sitting in the payload.
func icsContentLines(feed string) []string {
	normalized := strings.ReplaceAll(unfoldICS(feed), "\r\n", "\n")
	return strings.Split(strings.TrimRight(normalized, "\n"), "\n")
}

// icsEventLines narrows the feed to the content lines of its single VEVENT.
// Counting inside the component matters because the VCALENDAR carries its own
// NAME / DESCRIPTION, so a whole-payload count of an event property would be
// off by the calendar-level one.
func icsEventLines(t *testing.T, feed string) []string {
	t.Helper()
	var (
		event  []string
		inside bool
	)
	for _, line := range icsContentLines(feed) {
		switch {
		case line == "BEGIN:VEVENT":
			inside = true
		case line == "END:VEVENT":
			inside = false
		case inside:
			event = append(event, line)
		}
	}
	require.NotEmpty(t, event, "feed must contain a VEVENT")
	return event
}

// countICSProperty counts content lines that OPEN the named property. A value
// that merely mentions "SUMMARY:" mid-line is inert text and is not counted;
// only a line that starts one is a property.
func countICSProperty(lines []string, name string) int {
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, name+":") || strings.HasPrefix(line, name+";") {
			count++
		}
	}
	return count
}

// The personal feed is served from a bearer-token URL, and every text value in
// it originates in community-editable data. golang-ical's TEXT escaper handles
// `\`, `;`, `,` and LF but NOT CR, so without sanitization a submitted title
// carrying a carriage return could close its own property and have a lenient
// client read the remainder as a forged one.
func TestGenerateICSFeed_RejectsPropertyInjectionFromCommunityText(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID: 7,
				// URL is the one property golang-ical does not escape at all: it
				// types the value as a URI and writes it verbatim. Slugs are
				// server-generated today and cannot carry a control character,
				// so this pins the chokepoint rather than a live hole.
				Slug:      "hostile-show\r\nX-SLUG-FORGED:yes",
				Title:     "Gig\r\nSUMMARY:Hijacked\r\nDTSTART:19700101T000000Z",
				EventDate: time.Now().Add(24 * time.Hour),
				Status:    "approved",
				Venues: []contracts.VenueResponse{
					{
						ID:      1,
						Name:    "Evil Room\r\nX-FORGED-VENUE:yes",
						Address: ptrString("1 Main St\nX-FORGED-ADDRESS:yes"),
						City:    "Phoenix",
						State:   "AZ",
					},
				},
				Artists: []contracts.ArtistResponse{
					{ID: 1, Name: "Band\r\nATTENDEE:mailto:victim@example.com"},
					// A bare CR with no LF: the character golang-ical does not
					// escape, and the one a lenient parser is likeliest to treat
					// as a line break on its own.
					{ID: 2, Name: "Support\rORGANIZER:mailto:victim@example.com"},
				},
				AgeRequirement: ptrString("21+\r\nX-FORGED-AGES:yes"),
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
		},
	}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: &mockSavedShowSvc{shows: mockShows, total: 1}}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	require.NoError(t, err)
	out := string(data)

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "hostile text must still produce a parseable calendar")
	require.Len(t, parsed.Events(), 1)
	event := parsed.Events()[0]

	// Assertions run on UNFOLDED content lines: the hostile strings are long
	// enough that folding would otherwise hide an injected property from a
	// substring match.
	all := icsContentLines(out)
	eventLines := icsEventLines(t, out)

	// No CR may survive INSIDE a content line. Asserted per line rather than
	// over the whole payload so it stays strict if the serializer is ever
	// switched to the RFC's CRLF line endings: a whole-payload check would have
	// to strip "\r\n" first, and that strip would also swallow an injected CR
	// that happened to be followed by a LF.
	for _, line := range all {
		assert.NotContains(t, line, "\r", "no raw carriage return may survive inside a content line")
	}

	assert.Equal(t, 1, countICSProperty(eventLines, "SUMMARY"), "injected text must not add a second SUMMARY")
	assert.Equal(t, 1, countICSProperty(eventLines, "DTSTART"), "injected text must not add a second DTSTART")
	assert.Equal(t, 1, countICSProperty(eventLines, "LOCATION"))
	assert.Equal(t, 1, countICSProperty(eventLines, "DESCRIPTION"))
	assert.Equal(t, 1, countICSProperty(eventLines, "URL"), "injected text must not add a second URL")
	assert.Equal(t, 0, countICSProperty(all, "X-FORGED-VENUE"), "a venue name must not add a property")
	assert.Equal(t, 0, countICSProperty(all, "X-FORGED-ADDRESS"), "a venue address must not add a property")
	assert.Equal(t, 0, countICSProperty(all, "X-FORGED-AGES"), "an age requirement must not add a property")
	assert.Equal(t, 0, countICSProperty(all, "X-SLUG-FORGED"), "a slug must not add a property through the unescaped URL value")
	assert.Equal(t, 0, countICSProperty(all, "ATTENDEE"), "an artist name must not add an attendee")
	assert.Equal(t, 0, countICSProperty(all, "ORGANIZER"), "a bare CR in an artist name must not add an organizer")

	// One VEVENT, not one per injected line break (BEGIN:VCALENDAR is the other).
	assert.Equal(t, 2, countICSProperty(all, "BEGIN"), "the calendar must hold exactly one VEVENT")

	// The hostile text survives only as INERT VALUE inside the property it was
	// typed into.
	require.NotNil(t, event.GetProperty(ics.ComponentPropertySummary))
	assert.Equal(t, "GigSUMMARY:HijackedDTSTART:19700101T000000Z",
		event.GetProperty(ics.ComponentPropertySummary).Value,
		"the whole hostile string must stay inside SUMMARY's value")
	assert.Nil(t, event.GetProperty("X-FORGED-VENUE"))
	assert.Nil(t, event.GetProperty(ics.ComponentPropertyAttendee))
	assert.Nil(t, event.GetProperty(ics.ComponentPropertyOrganizer))
}

// NEL (U+0085), LS (U+2028) and PS (U+2029) are not content-line delimiters to
// a conformant iCalendar parser, which splits on CRLF alone, so they reach the
// wire as ordinary text unless something removes them. Plenty of consumers are
// not conformant: anything that decodes the payload to a string and splits on
// its platform's notion of a line break (Python's str.splitlines, a Java regex
// `\R`) treats all three as breaks. No legitimate name contains one, so they
// are stripped rather than escaped or preserved.
func TestGenerateICSFeed_StripsUnicodeLineSeparators(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        8,
				Slug:      "unicode-show",
				Title:     "Gig\u2028X-FORGED-LS:yes\u2029X-FORGED-PS:yes\u0085X-FORGED-NEL:yes",
				EventDate: time.Now().Add(24 * time.Hour),
				Status:    "approved",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: &mockSavedShowSvc{shows: mockShows, total: 1}}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	require.NoError(t, err)
	out := string(data)

	for _, sep := range []string{"\u0085", "\u2028", "\u2029"} {
		assert.NotContains(t, out, sep, "no Unicode line separator may reach the payload")
	}
	assert.Equal(t, 1, countICSProperty(icsEventLines(t, out), "SUMMARY"))

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, parsed.Events(), 1)
	assert.Equal(t, "GigX-FORGED-LS:yesX-FORGED-PS:yesX-FORGED-NEL:yes",
		parsed.Events()[0].GetProperty(ics.ComponentPropertySummary).Value,
		"the separators are dropped and the surrounding text stays inert")
}

// =============================================================================
// Mock saved show service for unit tests
// =============================================================================

type mockSavedShowSvc struct {
	shows          []*contracts.SavedShowResponse
	total          int64
	err            error
	lastTimeFilter string
}

func (m *mockSavedShowSvc) SaveShow(_, _ uint) error   { return nil }
func (m *mockSavedShowSvc) UnsaveShow(_, _ uint) error { return nil }
func (m *mockSavedShowSvc) GetUserSavedShows(_ uint, _, _ int, timeFilter string) ([]*contracts.SavedShowResponse, int64, error) {
	m.lastTimeFilter = timeFilter
	return m.shows, m.total, m.err
}
func (m *mockSavedShowSvc) IsShowSaved(_, _ uint) (bool, error) { return false, nil }
func (m *mockSavedShowSvc) GetSavedShowIDs(_ uint, _ []uint) (map[uint]bool, error) {
	return nil, nil
}
func (m *mockSavedShowSvc) GetSaveCount(_ uint) (int, error) { return 0, nil }
func (m *mockSavedShowSvc) GetBatchSaveCounts(showIDs []uint) (map[uint]int, error) {
	result := make(map[uint]int, len(showIDs))
	for _, id := range showIDs {
		result[id] = 0
	}
	return result, nil
}

func ptrString(s string) *string { return &s }

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type CalendarIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *CalendarService
}

func (suite *CalendarIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	mockSavedShows := &mockSavedShowSvc{shows: []*contracts.SavedShowResponse{}, total: 0}
	suite.svc = &CalendarService{db: suite.testDB.DB, savedShowSvc: mockSavedShows}
}

func (suite *CalendarIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *CalendarIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	for _, table := range []string{
		"show_artists", "show_venues", "artist_releases",
		"shows", "releases", "artists", "venues",
		"user_bookmarks", "calendar_tokens", "users",
	} {
		_, _ = sqlDB.Exec("DELETE FROM " + table)
	}
}

func TestCalendarIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CalendarIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *CalendarIntegrationTestSuite) createTestUser(active bool) *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("cal-user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Calendar"),
		LastName:      stringPtr("User"),
		IsActive:      active,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

// =============================================================================
// CreateToken tests
// =============================================================================

func (suite *CalendarIntegrationTestSuite) TestCreateToken_Success() {
	user := suite.createTestUser(true)
	resp, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.True(strings.HasPrefix(resp.Token, CalendarTokenPrefix))
	suite.Contains(resp.FeedURL, "http://localhost:8080/feeds/phcal_")
	suite.True(strings.HasSuffix(resp.FeedURL, "/saved-shows.ics"))
	suite.Contains(resp.FollowsFeedURL, "http://localhost:8080/feeds/phcal_")
	suite.True(strings.HasSuffix(resp.FollowsFeedURL, "/follows.atom"))
	suite.NotZero(resp.CreatedAt)
}

func (suite *CalendarIntegrationTestSuite) TestCreateToken_ReplacesExisting() {
	user := suite.createTestUser(true)

	resp1, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	resp2, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	// New token should be different
	suite.NotEqual(resp1.Token, resp2.Token)

	// Old token should no longer validate
	_, err = suite.svc.ValidateCalendarToken(resp1.Token)
	suite.Error(err)
	suite.Contains(err.Error(), "invalid calendar token")

	// New token should validate
	validatedUser, err := suite.svc.ValidateCalendarToken(resp2.Token)
	suite.Require().NoError(err)
	suite.Equal(user.ID, validatedUser.ID)
}

// =============================================================================
// GetTokenStatus tests
// =============================================================================

func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_NoToken() {
	user := suite.createTestUser(true)
	status, err := suite.svc.GetTokenStatus(user.ID)
	suite.Require().NoError(err)
	suite.False(status.HasToken)
	suite.Nil(status.CreatedAt)
}

func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_HasToken() {
	user := suite.createTestUser(true)
	_, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	status, err := suite.svc.GetTokenStatus(user.ID)
	suite.Require().NoError(err)
	suite.True(status.HasToken)
	suite.NotNil(status.CreatedAt)
}

// =============================================================================
// DeleteToken tests
// =============================================================================

func (suite *CalendarIntegrationTestSuite) TestDeleteToken_Success() {
	user := suite.createTestUser(true)
	resp, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	err = suite.svc.DeleteToken(user.ID)
	suite.NoError(err)

	// Token should no longer validate
	_, err = suite.svc.ValidateCalendarToken(resp.Token)
	suite.Error(err)

	// Status should show no token
	status, err := suite.svc.GetTokenStatus(user.ID)
	suite.Require().NoError(err)
	suite.False(status.HasToken)
}

func (suite *CalendarIntegrationTestSuite) TestDeleteToken_NotFound() {
	user := suite.createTestUser(true)
	err := suite.svc.DeleteToken(user.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "no calendar token found")
}

// =============================================================================
// ValidateCalendarToken tests
// =============================================================================

func (suite *CalendarIntegrationTestSuite) TestValidateCalendarToken_Success() {
	user := suite.createTestUser(true)
	resp, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	validatedUser, err := suite.svc.ValidateCalendarToken(resp.Token)
	suite.Require().NoError(err)
	suite.Equal(user.ID, validatedUser.ID)
}

func (suite *CalendarIntegrationTestSuite) TestValidateCalendarToken_Invalid() {
	_, err := suite.svc.ValidateCalendarToken("phcal_nonexistent_token")
	suite.Error(err)
	suite.Contains(err.Error(), "invalid calendar token")
}

// unfoldICS reverses RFC 5545 3.1 line folding so a test can search the feed's
// VALUES rather than its wire bytes. A property longer than 75 octets is split
// across lines with CRLF plus one leading space or tab, which silently defeats a
// plain substring assertion — a privacy test that greps for an address it must
// NOT find would pass while the address is sitting in the payload, folded.
func unfoldICS(feed string) string {
	unfolded := strings.ReplaceAll(feed, "\r\n ", "")
	unfolded = strings.ReplaceAll(unfolded, "\r\n\t", "")
	unfolded = strings.ReplaceAll(unfolded, "\n ", "")
	return strings.ReplaceAll(unfolded, "\n\t", "")
}

// The ICS feed is the worst place for an unverified venue's street address to
// surface: the URL carries a bearer token instead of a session, so anyone
// holding the link can fetch it, and an ICS LOCATION is copied onto the
// subscriber's device where a later redaction can never reach it.
//
// This wires the REAL SavedShowService rather than the suite's mock, because
// the redaction lives in that service's response builder and the whole point of
// the test is that this feed inherits it. The peer tests above deliberately mock
// that dependency to isolate ICS formatting; a mock here would assert nothing.
func (suite *CalendarIntegrationTestSuite) TestGenerateICSFeed_OmitsUnverifiedVenueAddress() {
	user := suite.createTestUser(true)

	// Deliberately long enough that LOCATION exceeds RFC 5545's 75-octet line
	// limit and the serializer FOLDS it. With this fixture the break lands
	// INSIDE the street address, emitting `...Center\, 1234` / `  Secret St\,
	// Phoenix\, AZ`, so a plain strings.Contains(feed, "1234 Secret St") over
	// LOCATION is false even when the address is fully published. A negative
	// privacy assertion is exactly the kind that fails open under that, so every
	// assertion below runs on the unfolded text. A short venue name would leave
	// this unexercised and the guard would rot without anything noticing.
	venue := &catalogm.Venue{
		Name:     "The Basement Annex Performance Hall and Recreation Center",
		City:     "Phoenix",
		State:    "AZ",
		Address:  stringPtr("1234 Secret St"),
		Verified: false,
	}
	suite.Require().NoError(suite.db.Create(venue).Error)

	show := &catalogm.Show{
		Title:       "House Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)

	savedShows := NewSavedShowService(suite.db)
	suite.Require().NoError(savedShows.SaveShow(user.ID, show.ID))
	svc := NewCalendarService(suite.db, savedShows)

	data, err := svc.GenerateICSFeed(user.ID, "https://psychichomily.com")
	suite.Require().NoError(err)

	feed := unfoldICS(string(data))
	suite.Require().Contains(feed, "House Show", "feed must contain the show, or the assertion below is vacuous")
	suite.NotContains(feed, "1234 Secret St", "unverified venue address must not reach the ICS feed")
	// The venue is still named and placed, so redaction is not gutting LOCATION.
	suite.Contains(feed, "The Basement Annex")

	// Same venue, now verified: the address is legitimate and must come through,
	// proving the feed is gated on verification rather than dropping addresses.
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).
		Where("id = ?", venue.ID).Update("verified", true).Error)
	// The ICS payload is cached per user for icsFeedCacheTTL, so without this the
	// second read would replay the pre-verification bytes and the assertion below
	// would fail pointing at the redaction logic rather than at the cache.
	svc.invalidateFeedCache(user.ID)

	data, err = svc.GenerateICSFeed(user.ID, "https://psychichomily.com")
	suite.Require().NoError(err)
	suite.Contains(unfoldICS(string(data)), "1234 Secret St", "verified venue address must still be served")
}

func (suite *CalendarIntegrationTestSuite) TestValidateCalendarToken_InactiveUser() {
	user := suite.createTestUser(true) // create as active first
	resp, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	// Deactivate the user
	suite.db.Model(&authm.User{}).Where("id = ?", user.ID).Update("is_active", false)

	_, err = suite.svc.ValidateCalendarToken(resp.Token)
	suite.Error(err)
	suite.Contains(err.Error(), "user account is not active")
}
