package engagement

import (
	"bytes"
	"fmt"
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

// icsSequence is RFC 5545 3.8.7.4's SEQUENCE: a non-negative INTEGER.
var icsSequence = regexp.MustCompile(`^SEQUENCE:\d+$`)

// rfc5545WireProblems reads feed as the exact bytes a calendar client receives
// and lists every way it falls short of the wire contract: a line that does
// not end in CRLF, a bare CR inside a line, a VEVENT without exactly one UTC
// DTSTAMP and exactly one non-negative SEQUENCE, or the wrong VEVENT count.
// Nothing here normalizes line endings before checking them, because a check
// that does cannot see an LF-delimited feed.
//
// SEQUENCE is optional in RFC 5545 but required here: every surface shares one
// UID per show, and a copy without it ranks below an edited copy that has it
// (RFC 5546 2.1.5).
func rfc5545WireProblems(feed []byte, wantEvents int) []string {
	var problems []string

	if !bytes.HasSuffix(feed, []byte("\r\n")) {
		problems = append(problems, "the last content line does not end in CRLF (RFC 5545 3.1)")
	}

	// Splitting on LF and requiring each piece to end in exactly one CR is what
	// catches both defects: a piece without the CR is a bare LF, and a CR
	// anywhere else is a bare CR.
	physical := bytes.Split(bytes.TrimSuffix(feed, []byte("\n")), []byte("\n"))
	for i, line := range physical {
		if !bytes.HasSuffix(line, []byte("\r")) {
			problems = append(problems, fmt.Sprintf("line %d ends in a bare LF: %q", i+1, line))
			continue
		}
		if bytes.Count(line, []byte("\r")) != 1 {
			problems = append(problems, fmt.Sprintf("line %d carries a bare CR: %q", i+1, line))
		}
		// A trailing blank line is trimmed away by icsContentLines below, so it
		// has to be caught here, where the physical lines are still intact.
		if len(line) == 1 {
			problems = append(problems, fmt.Sprintf("line %d is empty", i+1))
		}
	}

	// icsContentLines normalizes line endings, which is safe for the property
	// checks because the line-ending defects were recorded above. Property names
	// are case-insensitive (RFC 5545 2), so every content line is compared in
	// upper case; a VEVENT that opens inside another, or never closes, is a
	// defect of its own rather than a reason to skip its checks.
	events := 0
	var event []string
	for i, line := range icsContentLines(string(feed)) {
		if line == "" {
			problems = append(problems, fmt.Sprintf("content line %d is empty", i+1))
			continue
		}
		upper := strings.ToUpper(line)
		switch upper {
		case "BEGIN:VEVENT":
			if event != nil {
				problems = append(problems, fmt.Sprintf("VEVENT %d opens before VEVENT %d closes", events+1, events))
			}
			events++
			event = []string{}
		case "END:VEVENT":
			if event == nil {
				problems = append(problems, fmt.Sprintf("content line %d: END:VEVENT without an open VEVENT", i+1))
				continue
			}
			if n := countICSProperty(event, "DTSTAMP"); n != 1 {
				problems = append(problems, fmt.Sprintf("VEVENT %d carries %d DTSTAMP properties, want exactly one", events, n))
			}
			if n := countICSProperty(event, "SEQUENCE"); n != 1 {
				problems = append(problems, fmt.Sprintf("VEVENT %d carries %d SEQUENCE properties, want exactly one", events, n))
			}
			for _, prop := range event {
				if strings.HasPrefix(prop, "DTSTAMP;") {
					problems = append(problems, fmt.Sprintf("VEVENT %d: DTSTAMP carries a parameter, want a bare UTC DATE-TIME: %q", events, prop))
				}
				if strings.HasPrefix(prop, "DTSTAMP:") && !icsUTCDateTime.MatchString(strings.TrimPrefix(prop, "DTSTAMP:")) {
					problems = append(problems, fmt.Sprintf("VEVENT %d: DTSTAMP is not a UTC DATE-TIME: %q", events, prop))
				}
				if (strings.HasPrefix(prop, "SEQUENCE:") || strings.HasPrefix(prop, "SEQUENCE;")) && !icsSequence.MatchString(prop) {
					problems = append(problems, fmt.Sprintf("VEVENT %d: SEQUENCE is not a non-negative integer: %q", events, prop))
				}
			}
			event = nil
		default:
			if event != nil {
				event = append(event, upper)
			}
		}
	}
	if event != nil {
		problems = append(problems, fmt.Sprintf("VEVENT %d is never closed", events))
	}
	if events != wantEvents {
		problems = append(problems, fmt.Sprintf("%d VEVENTs on the wire, want %d", events, wantEvents))
	}
	return problems
}

// assertRFC5545Wire fails unless feed has no wire problems and golang-ical
// reads its own bytes back with wantEvents events. The parse is a round-trip
// sanity check, not a strictness claim: golang-ical's parser accepts a bare LF
// and a missing DTSTAMP, which TestRFC5545Wire_RejectsTheLegacySerialization
// pins, so the byte-level checks above it are the ones that carry the contract.
func assertRFC5545Wire(t *testing.T, feed []byte, wantEvents int) {
	t.Helper()

	require.Empty(t, rfc5545WireProblems(feed, wantEvents), "RFC 5545 wire problems")

	parsed, err := ics.ParseCalendar(bytes.NewReader(feed))
	require.NoError(t, err, "golang-ical must parse its own serialized bytes")
	assert.Len(t, parsed.Events(), wantEvents, "VEVENT count after parsing")
}

// The shape the saved-shows feed had before it joined the sibling feeds: golang-
// ical's default serialization, bare LF, no DTSTAMP, no SEQUENCE. The wire
// checks must reject it, and the lenient parser must still accept it, or a
// later reader could drop the byte checks because "the parse covers it".
func TestRFC5545Wire_RejectsTheLegacySerialization(t *testing.T) {
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	event := cal.AddEvent("legacy@psychichomily.com")
	start := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)
	event.SetStartAt(start)
	event.SetEndAt(start.Add(3 * time.Hour))
	event.SetSummary("Desert Doom Night")
	legacy := []byte(cal.Serialize())

	problems := strings.Join(rfc5545WireProblems(legacy, 1), "\n")
	assert.Contains(t, problems, "bare LF", "an LF-delimited feed must be reported")
	assert.Contains(t, problems, "0 DTSTAMP properties", "a VEVENT without DTSTAMP must be reported")
	assert.Contains(t, problems, "0 SEQUENCE properties", "a VEVENT without SEQUENCE must be reported")

	_, err := ics.ParseCalendar(bytes.NewReader(legacy))
	require.NoError(t, err, "golang-ical's parser is lenient about exactly these defects, which is why the byte checks exist")
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
	saved := &mockSavedShowSvc{shows: shows, total: int64(len(shows))}
	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: saved}

	first, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	require.NoError(t, err)
	assertRFC5545Wire(t, first, len(shows))
	require.Contains(t, string(first), "\r\n ",
		"the fixture must fold at least one line, or CRLF on continuation lines goes unchecked")
	assert.Contains(t, string(first), "DTSTAMP:20260601T120000Z\r\n",
		"DTSTAMP comes from the show's UpdatedAt in UTC")
	assert.Contains(t, string(first), "DTSTAMP:20260603T000000Z\r\n")
	assert.Contains(t, string(first), "SEQUENCE:0\r\n", "an unedited show is SEQUENCE 0")
	assert.Contains(t, string(first), fmt.Sprintf("SEQUENCE:%d\r\n", showSequence(created, edited)),
		"an edited show carries the sequence the sibling feeds give it")

	// Emptying the source proves the second read comes from feedCache, and the
	// cached bytes must be the same wire bytes.
	saved.shows, saved.total = nil, 0
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

func TestBuildSavedShowsCalendar_RendersGivenShows(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	shows := []*contracts.SavedShowResponse{
		savedShowFixture(1, "Desert Doom Night", created),
		savedShowFixture(2, "Last Call", created),
	}

	out := buildSavedShowsCalendar(shows, "https://psychichomily.com")
	assertRFC5545Wire(t, out, len(shows))
	assert.Contains(t, string(out), "SUMMARY:Desert Doom Night\r\n")
	assert.Contains(t, string(out), "SUMMARY:Last Call\r\n")
}

// A save made after the subscription feed cached its bytes is absent from the
// feed until the cache turns over, and present in an uncached render at once.
// That difference is the reason RenderSavedShowsCalendar exists.
func TestRenderSavedShowsCalendar_FreshWhileFeedIsCached(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	first := savedShowFixture(1, "Desert Doom Night", created)
	saved := &mockSavedShowSvc{shows: []*contracts.SavedShowResponse{first}, total: 1}
	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: saved}

	feed, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	require.NoError(t, err)
	assertRFC5545Wire(t, feed, 1)

	// The user saves another show. Nothing clears the feed cache.
	saved.shows = append(saved.shows, savedShowFixture(2, "Last Call", created))
	saved.total = 2

	stillCached, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	require.NoError(t, err)
	assert.Equal(t, feed, stillCached, "the subscription feed serves the cached bytes until the TTL turns over")

	fresh, err := svc.RenderSavedShowsCalendar(1, "https://psychichomily.com")
	require.NoError(t, err)
	assertRFC5545Wire(t, fresh, 2)
	assert.Contains(t, string(fresh), "SUMMARY:Last Call\r\n", "an uncached render includes the save made a moment ago")
}

// savedShowFixture is a saved show whose only distinguishing fields are the
// ones these wire tests read back.
func savedShowFixture(id uint, title string, stamp time.Time) *contracts.SavedShowResponse {
	return &contracts.SavedShowResponse{ShowResponse: *testShowResponse(func(s *contracts.ShowResponse) {
		s.ID = id
		s.Slug = ""
		s.Title = title
		s.CreatedAt = stamp
		s.UpdatedAt = stamp
	})}
}

// Structural defects the parser is lenient about must still surface as wire
// problems: a VEVENT that never closes, one that opens inside another, an
// empty content line, and a second DTSTAMP spelled in lower case.
func TestRFC5545Wire_ReportsStructuralDefects(t *testing.T) {
	cases := map[string]struct {
		feed string
		want string
	}{
		"unterminated VEVENT": {
			feed: "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:a\r\nEND:VCALENDAR\r\n",
			want: "is never closed",
		},
		"nested VEVENT": {
			feed: "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:a\r\nBEGIN:VEVENT\r\nUID:b\r\nDTSTAMP:20260601T120000Z\r\nSEQUENCE:0\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
			want: "opens before VEVENT 1 closes",
		},
		"empty content line": {
			feed: "BEGIN:VCALENDAR\r\n\r\nEND:VCALENDAR\r\n",
			want: "is empty",
		},
		"trailing blank line": {
			feed: "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:a\r\nDTSTAMP:20260601T120000Z\r\nSEQUENCE:0\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n\r\n",
			want: "is empty",
		},
		"lower-case duplicate DTSTAMP": {
			feed: "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:a\r\nDTSTAMP:20260601T120000Z\r\ndtstamp:19700101T000000Z\r\nSEQUENCE:0\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
			want: "2 DTSTAMP properties",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			problems := strings.Join(rfc5545WireProblems([]byte(tc.feed), 1), "\n")
			assert.Contains(t, problems, tc.want)
		})
	}
}
