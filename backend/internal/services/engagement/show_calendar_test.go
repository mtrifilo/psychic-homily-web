package engagement

import (
	"errors"
	"strings"
	"testing"
	"time"

	// Embed the IANA tz database so LoadLocation works in any CI image.
	_ "time/tzdata"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// These tests exercise the pure rendering half of the per-show download —
// everything from a response-shaped show to the serialized calendar — plus the
// approved-only gate, which is the one piece of access control this anonymous
// surface owns.

func testShowResponse(overrides func(*contracts.ShowResponse)) *contracts.ShowResponse {
	tz := "America/Phoenix"
	address := "2303 E Indian School Rd"
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	show := &contracts.ShowResponse{
		ID:        42,
		Slug:      "desert-doom-night",
		Title:     "Desert Doom Night",
		EventDate: time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC), // 20:00 Phoenix on Aug 14
		Status:    "approved",
		CreatedAt: created,
		UpdatedAt: created,
		Venues: []contracts.VenueResponse{{
			ID:       7,
			Slug:     "the-rebel-lounge",
			Name:     "The Rebel Lounge",
			Address:  &address,
			City:     "Phoenix",
			State:    "AZ",
			Timezone: &tz,
			Verified: true,
		}},
		Artists: []contracts.ArtistResponse{
			{ID: 1, Name: "AJJ"},
			{ID: 2, Name: "Calexico"},
		},
	}
	if overrides != nil {
		overrides(show)
	}
	return show
}

func renderShowEvent(show *contracts.ShowResponse) string {
	return string(buildShowEventCalendar(show, "https://psychichomily.com"))
}

// stubShowService implements only the two lookup methods the calendar service
// uses; any other call panics via the embedded nil interface, which is exactly
// the failure a test should surface.
type stubShowService struct {
	contracts.ShowServiceInterface
	byID   map[uint]*contracts.ShowResponse
	bySlug map[string]*contracts.ShowResponse
}

func (s *stubShowService) GetShow(showID uint) (*contracts.ShowResponse, error) {
	if show, ok := s.byID[showID]; ok {
		return show, nil
	}
	return nil, apperrors.ErrShowNotFound(showID)
}

func (s *stubShowService) GetShowBySlug(slug string) (*contracts.ShowResponse, error) {
	if show, ok := s.bySlug[slug]; ok {
		return show, nil
	}
	return nil, apperrors.ErrShowNotFound(0)
}

func newTestShowCalendarService(shows ...*contracts.ShowResponse) *ShowCalendarService {
	stub := &stubShowService{
		byID:   map[uint]*contracts.ShowResponse{},
		bySlug: map[string]*contracts.ShowResponse{},
	}
	for _, show := range shows {
		stub.byID[show.ID] = show
		if show.Slug != "" {
			stub.bySlug[show.Slug] = show
		}
	}
	return NewShowCalendarService(stub)
}

func TestShowEvent_ParsesAsSingleEvent(t *testing.T) {
	out := renderShowEvent(testShowResponse(nil))

	cal, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "rendered calendar must round-trip through an ICS parser")
	require.Len(t, cal.Events(), 1)

	event := cal.Events()[0]
	assert.Equal(t, "show-42@psychichomily.com", event.Id(),
		"UID must derive from the immutable show ID, matching the venue feed's scheme")
	assert.Contains(t, out, "PRODID:"+showEventCalendarProductID)
	assert.Contains(t, out, "DTSTAMP:20260601T120000Z")
	assert.Contains(t, out, "SEQUENCE:0")
}

func TestShowEvent_TimesAreVenueLocal(t *testing.T) {
	out := renderShowEvent(testShowResponse(nil))

	// 03:00Z on Aug 15 is 20:00 Phoenix on Aug 14 — the date rollback across
	// UTC midnight is exactly the case a UTC-naive export gets wrong.
	assert.Contains(t, out, "DTSTART;TZID=America/Phoenix:20260814T200000")
	assert.Contains(t, out, "DTEND;TZID=America/Phoenix:20260814T230000",
		"end time must be start + the shared default show duration")
}

func TestShowEvent_TimezoneFallsBackToStateMap(t *testing.T) {
	out := renderShowEvent(testShowResponse(func(s *contracts.ShowResponse) {
		s.Venues[0].Timezone = nil
		s.Venues[0].State = "IL"
		s.Venues[0].City = "Chicago"
	}))
	assert.Contains(t, out, "DTSTART;TZID=America/Chicago:",
		"NULL venues.timezone must fall through the state map, not land in UTC")
}

func TestShowEvent_SummaryPrefersTitleThenBill(t *testing.T) {
	withTitle := renderShowEvent(testShowResponse(nil))
	assert.Contains(t, unfold(withTitle), "SUMMARY:Desert Doom Night")

	noTitle := renderShowEvent(testShowResponse(func(s *contracts.ShowResponse) {
		s.Title = ""
	}))
	assert.Contains(t, unfold(noTitle), "SUMMARY:AJJ\\, Calexico",
		"an untitled show must be named after its bill, matching the frontend display-title convention")

	bare := renderShowEvent(testShowResponse(func(s *contracts.ShowResponse) {
		s.Title = ""
		s.Artists = nil
	}))
	assert.Contains(t, unfold(bare), "SUMMARY:Show at The Rebel Lounge")
}

func TestShowEvent_CancelledAndSoldOut(t *testing.T) {
	cancelled := renderShowEvent(testShowResponse(func(s *contracts.ShowResponse) {
		s.IsCancelled = true
	}))
	assert.Contains(t, cancelled, "STATUS:CANCELLED")
	assert.Contains(t, unfold(cancelled), "SUMMARY:CANCELLED: Desert Doom Night")

	soldOut := renderShowEvent(testShowResponse(func(s *contracts.ShowResponse) {
		s.IsSoldOut = true
	}))
	assert.Contains(t, soldOut, "STATUS:CONFIRMED")
	assert.Contains(t, unfold(soldOut), "SUMMARY:Desert Doom Night [SOLD OUT]")
}

func TestShowEvent_LocationOmitsRedactedAddress(t *testing.T) {
	out := renderShowEvent(testShowResponse(func(s *contracts.ShowResponse) {
		// The show service nulls the address for unverified venues before the
		// response reaches this renderer; the renderer must cope, not re-fetch.
		s.Venues[0].Address = nil
		s.Venues[0].Verified = false
	}))
	unfolded := unfold(out)
	assert.Contains(t, unfolded, "LOCATION:The Rebel Lounge\\, Phoenix\\, AZ")
	assert.NotContains(t, unfolded, "Indian School")
}

func TestShowEvent_SanitizesInjectedProperties(t *testing.T) {
	out := renderShowEvent(testShowResponse(func(s *contracts.ShowResponse) {
		s.Title = "Evil Show\r\nX-FORGED-PROP:yes"
		s.Artists = []contracts.ArtistResponse{{ID: 1, Name: "Band\r\nATTENDEE:mailto:x@y"}}
		s.Venues[0].Name = "Room\r\nDTSTART:19700101T000000Z"
	}))

	cal, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "hostile text must still produce a parseable calendar")
	require.Len(t, cal.Events(), 1)
	event := cal.Events()[0]

	// No bare CR may survive outside the CRLF line endings — that is the
	// character golang-ical does not escape and the one a lenient parser could
	// treat as a line break.
	assert.NotContains(t, strings.ReplaceAll(out, "\r\n", ""), "\r",
		"no raw carriage returns may survive into the payload")

	// The injected text may remain as INERT text inside a property value; what
	// it must never do is become a property of its own.
	assert.Nil(t, event.GetProperty("X-FORGED-PROP"), "a show title must not be able to add a property")
	assert.Nil(t, event.GetProperty(ics.ComponentPropertyAttendee), "an artist name must not add an attendee")

	dtstarts := event.GetProperties(ics.ComponentPropertyDtStart)
	require.Len(t, dtstarts, 1, "injected text must not add a second DTSTART")
	assert.Equal(t, "20260814T200000", dtstarts[0].Value)
}

func TestShowEvent_URLPrefersSlug(t *testing.T) {
	out := unfold(renderShowEvent(testShowResponse(nil)))
	assert.Contains(t, out, "URL:https://psychichomily.com/shows/desert-doom-night")

	noSlug := unfold(renderShowEvent(testShowResponse(func(s *contracts.ShowResponse) {
		s.Slug = ""
	})))
	assert.Contains(t, noSlug, "URL:https://psychichomily.com/shows/42")
}

func TestShowEvent_ETagStableForIdenticalState(t *testing.T) {
	svc := newTestShowCalendarService(testShowResponse(nil))

	first, err := svc.GenerateShowEvent("42", "https://psychichomily.com")
	require.NoError(t, err)
	second, err := svc.GenerateShowEvent("desert-doom-night", "https://psychichomily.com")
	require.NoError(t, err)

	assert.Equal(t, first.ETag, second.ETag,
		"identical DB state must render a byte-identical document regardless of lookup path")
	assert.Equal(t, first.ICS, second.ICS)

	edited := testShowResponse(func(s *contracts.ShowResponse) {
		s.UpdatedAt = s.UpdatedAt.Add(time.Hour)
	})
	third, err := newTestShowCalendarService(edited).GenerateShowEvent("42", "https://psychichomily.com")
	require.NoError(t, err)
	assert.NotEqual(t, first.ETag, third.ETag)
}

func TestGenerateShowEvent_NonApprovedIsIndistinguishableFromMissing(t *testing.T) {
	for _, status := range []string{"pending", "private", "rejected"} {
		svc := newTestShowCalendarService(testShowResponse(func(s *contracts.ShowResponse) {
			s.Status = status
		}))

		_, err := svc.GenerateShowEvent("42", "https://psychichomily.com")
		require.Error(t, err, "status %q must not be exportable anonymously", status)

		var showErr *apperrors.ShowError
		require.True(t, errors.As(err, &showErr), "status %q must map to a ShowError", status)
		assert.Equal(t, apperrors.CodeShowNotFound, showErr.Code,
			"a hidden show must be indistinguishable from a missing one")
	}
}

func TestGenerateShowEvent_ResolvesByIDAndSlug(t *testing.T) {
	svc := newTestShowCalendarService(testShowResponse(nil))

	byID, err := svc.GenerateShowEvent("42", "https://psychichomily.com")
	require.NoError(t, err)
	bySlug, err := svc.GenerateShowEvent("desert-doom-night", "https://psychichomily.com")
	require.NoError(t, err)

	assert.Equal(t, byID.ICS, bySlug.ICS)
	assert.Equal(t, "desert-doom-night", byID.ShowSlug)
}
