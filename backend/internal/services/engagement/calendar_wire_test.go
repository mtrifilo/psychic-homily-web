package engagement

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// These tests read every ICS feed as the exact bytes a calendar client
// receives. Nothing here normalizes "\r\n" to "\n" or unfolds before checking
// line endings, because a test that does cannot see an LF-delimited feed. All
// four builders go through the same assertion so the family stays aligned.

// icsUTCDateTime is the only form RFC 5545 3.8.7.2 allows for DTSTAMP: a
// DATE-TIME in UTC, with no TZID parameter.
var icsUTCDateTime = regexp.MustCompile(`^\d{8}T\d{6}Z$`)

// assertRFC5545Wire fails unless feed is a wire-valid calendar carrying
// wantEvents VEVENTs: every line ends in CRLF with no bare CR or LF anywhere,
// every VEVENT has exactly one DTSTAMP in UTC form, and golang-ical's own
// parser reads the bytes back.
func assertRFC5545Wire(t *testing.T, feed []byte, wantEvents int) {
	t.Helper()

	require.True(t, bytes.HasSuffix(feed, []byte("\r\n")),
		"RFC 5545 3.1: the last content line must end in CRLF")

	// Splitting on LF and requiring each piece to end in exactly one CR is what
	// catches both defects: a piece without the CR is a bare LF, and a CR
	// anywhere else is a bare CR.
	physical := bytes.Split(bytes.TrimSuffix(feed, []byte("\n")), []byte("\n"))
	for i, line := range physical {
		require.Truef(t, bytes.HasSuffix(line, []byte("\r")),
			"line %d must end in CRLF, got %q", i+1, line)
		require.Equalf(t, 1, bytes.Count(line, []byte("\r")),
			"line %d carries a bare CR: %q", i+1, line)
	}

	// icsContentLines normalizes line endings, which is safe only now that every
	// one is known to be CRLF.
	events := 0
	var event []string
	for _, line := range icsContentLines(string(feed)) {
		switch line {
		case "BEGIN:VEVENT":
			events++
			event = []string{}
		case "END:VEVENT":
			assert.Equalf(t, 1, countICSProperty(event, "DTSTAMP"),
				"VEVENT %d must carry exactly one DTSTAMP", events)
			for _, prop := range event {
				if strings.HasPrefix(prop, "DTSTAMP:") || strings.HasPrefix(prop, "DTSTAMP;") {
					assert.Regexpf(t, icsUTCDateTime, strings.TrimPrefix(prop, "DTSTAMP:"),
						"VEVENT %d: DTSTAMP must be a bare UTC DATE-TIME, got %q", events, prop)
				}
			}
			event = nil
		default:
			if event != nil {
				event = append(event, line)
			}
		}
	}
	require.Equal(t, wantEvents, events, "VEVENT count on the wire")

	parsed, err := ics.ParseCalendar(bytes.NewReader(feed))
	require.NoError(t, err, "golang-ical must parse its own serialized bytes")
	assert.Len(t, parsed.Events(), wantEvents, "VEVENT count after parsing")
}

func TestICSWire_SavedShowsFeed(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	edited := created.Add(36 * time.Hour)
	show := func(id uint, title string, updated time.Time) *contracts.SavedShowResponse {
		return &contracts.SavedShowResponse{ShowResponse: *testShowResponse(func(s *contracts.ShowResponse) {
			s.ID = id
			s.Slug = ""
			s.Title = title
			s.CreatedAt = created
			s.UpdatedAt = updated
		})}
	}
	shows := []*contracts.SavedShowResponse{
		show(1, "Desert Doom Night", created),
		// Long enough that SUMMARY folds, so continuation lines are on the wire.
		show(2, strings.Repeat("A Very Long Title For A Very Long Night ", 4), edited),
		show(3, "Last Call", edited),
	}
	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: &mockSavedShowSvc{shows: shows, total: int64(len(shows))}}

	first, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	require.NoError(t, err)
	assertRFC5545Wire(t, first, len(shows))
	assert.Contains(t, string(first), "DTSTAMP:20260601T120000Z\r\n",
		"DTSTAMP comes from the show's UpdatedAt in UTC")
	assert.Contains(t, string(first), "DTSTAMP:20260603T000000Z\r\n")

	// The second read is served from feedCache; it must be the same wire bytes.
	cached, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	require.NoError(t, err)
	assert.Equal(t, first, cached)
}

func TestICSWire_ShowEvent(t *testing.T) {
	assertRFC5545Wire(t, []byte(renderShowEvent(testShowResponse(nil))), 1)
}

func TestICSWire_VenueFeed(t *testing.T) {
	out := renderFeed(testVenue(nil), []catalogm.Show{testShow(1, nil), testShow(2, nil)}, map[uint][]string{
		1: {"Headliner Band", "Support Act"},
	})
	assertRFC5545Wire(t, []byte(out), 2)
}

func TestICSWire_SceneFeed(t *testing.T) {
	out := renderSceneFeed([]contracts.SceneShowSummary{testSceneShow(1, nil), testSceneShow(2, nil)})
	assertRFC5545Wire(t, []byte(out), 2)
}
