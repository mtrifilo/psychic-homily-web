package engagement

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	// Embed the IANA tz database so LoadLocation works in any CI image.
	_ "time/tzdata"

	ics "github.com/arran4/golang-ical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// These tests exercise the pure rendering half of the venue feed — everything
// from a resolved venue plus a slice of shows to the serialized calendar. That
// is where every silent-failure mode of an ICS feed lives (wrong local time,
// unstable UID, unescaped text), and none of it needs a database.

func testVenue(overrides func(*contracts.VenueDetailResponse)) *contracts.VenueDetailResponse {
	tz := "America/Phoenix"
	address := "2303 E Indian School Rd"
	venue := &contracts.VenueDetailResponse{
		ID:       7,
		Slug:     "the-rebel-lounge",
		Name:     "The Rebel Lounge",
		Address:  &address,
		City:     "Phoenix",
		State:    "AZ",
		Timezone: &tz,
	}
	if overrides != nil {
		overrides(venue)
	}
	return venue
}

func testShow(id uint, overrides func(*catalogm.Show)) catalogm.Show {
	slug := fmt.Sprintf("show-%d-slug", id)
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	show := catalogm.Show{
		ID:        id,
		Title:     "Night of Noise",
		Slug:      &slug,
		EventDate: time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC), // 20:00 Phoenix on Aug 14
		Status:    catalogm.ShowStatusApproved,
		CreatedAt: created,
		UpdatedAt: created,
	}
	if overrides != nil {
		overrides(&show)
	}
	return show
}

// renderFeed drives buildCalendar directly — no DB, no venue service.
func renderFeed(venue *contracts.VenueDetailResponse, shows []catalogm.Show, artists map[uint][]string) string {
	svc := &VenueCalendarService{}
	if artists == nil {
		artists = map[uint][]string{}
	}
	return string(svc.buildCalendar(venue, shows, artists, "https://psychichomily.com"))
}

// unfold reverses RFC 5545 line folding so assertions can match long values.
//
// A thin alias for unfoldICS (calendar_test.go) rather than a second
// implementation. The version that lived here handled only a leading SPACE
// continuation, and RFC 5545 3.1 permits a TAB as well — which made
// TestVenueFeed_OmitsRedactedAddress, a PRIVACY assertion, able to pass with a
// tab-folded address sitting in the payload.
func unfold(s string) string {
	return unfoldICS(s)
}

func TestVenueFeed_ParsesAsCalendar(t *testing.T) {
	out := renderFeed(testVenue(nil), []catalogm.Show{testShow(1, nil)}, map[uint][]string{
		1: {"Headliner Band", "Support Act"},
	})

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "feed must round-trip through an iCalendar parser")
	require.Len(t, parsed.Events(), 1)

	assert.Contains(t, out, "BEGIN:VCALENDAR")
	assert.Contains(t, out, "END:VCALENDAR")
	assert.Contains(t, out, "VERSION:2.0")
	assert.Contains(t, out, "PRODID:"+venueFeedCalendarProductID)
	assert.Contains(t, out, "METHOD:PUBLISH")
	// Commas are escaped as `\,` inside a TEXT value per RFC 5545.
	assert.Contains(t, unfold(out), `Artists: Headliner Band\, Support Act`,
		"artists must be listed in billed order")
}

// RFC 5545 3.6.1 requires UID and DTSTAMP on every VEVENT. golang-ical supplies
// neither DTSTAMP nor SEQUENCE on its own, so their absence would be silent.
func TestVenueFeed_EmitsRequiredEventProperties(t *testing.T) {
	out := renderFeed(testVenue(nil), []catalogm.Show{testShow(42, nil)}, nil)

	for _, required := range []string{
		"UID:show-42@psychichomily.com",
		"DTSTAMP:",
		"SEQUENCE:",
		"STATUS:CONFIRMED",
		"SUMMARY:Night of Noise",
	} {
		assert.Contains(t, out, required, "every VEVENT needs %s", required)
	}

	// CRLF line endings are mandatory (RFC 5545 3.1); a bare-LF feed is
	// rejected outright by some clients.
	assert.NotContains(t, strings.ReplaceAll(out, "\r\n", ""), "\n",
		"all line breaks must be CRLF")
}

// The single highest-risk behaviour: a Phoenix show stored at 03:00 UTC is
// 20:00 the PREVIOUS day locally. A UTC-naive feed would show it at the wrong
// hour for every subscriber.
func TestVenueFeed_TimesAreVenueLocal(t *testing.T) {
	out := renderFeed(testVenue(nil), []catalogm.Show{testShow(1, nil)}, nil)

	assert.Contains(t, out, "DTSTART;TZID=America/Phoenix:20260814T200000",
		"stored 2026-08-15T03:00Z must render as 20:00 Phoenix on Aug 14")
	assert.Contains(t, out, "DTEND;TZID=America/Phoenix:20260814T230000",
		"end time must stay in the venue's zone")
	assert.NotContains(t, out, "DTSTART:20260815T030000Z",
		"a bare UTC DTSTART lets each client re-shift a fixed-location event")
}

// A venue with no explicit IANA zone must still resolve through the shared
// state fallback rather than silently landing in UTC.
func TestVenueFeed_FallsBackToStateTimezone(t *testing.T) {
	venue := testVenue(func(v *contracts.VenueDetailResponse) {
		v.Timezone = nil
		v.State = "IL"
		v.City = "Chicago"
	})
	out := renderFeed(venue, []catalogm.Show{testShow(1, nil)}, nil)

	assert.Contains(t, out, "DTSTART;TZID=America/Chicago:",
		"a missing venues.timezone must fall back to the state map, not UTC")
}

// A venue with no explicit zone AND a state outside the US map has nothing left
// to fall back to but the Arizona default, so the feed publishes the DAY and
// refuses the hour. RFC 5545 makes a DATE-valued DTEND exclusive, hence the
// following day.
func TestVenueFeed_UnresolvedZoneIsAllDay(t *testing.T) {
	venue := testVenue(func(v *contracts.VenueDetailResponse) {
		v.Timezone = nil
		v.State = "England"
		v.City = "London"
	})
	out := renderFeed(venue, []catalogm.Show{testShow(1, nil)}, nil)

	assert.Contains(t, out, "DTSTART;VALUE=DATE:20260814",
		"stored 2026-08-15T03:00Z reads as Aug 14 on the fallback, and the day is still published")
	assert.Contains(t, out, "DTEND;VALUE=DATE:20260815")
	assert.NotContains(t, out, "DTSTART;TZID=", "no zone may be named for this room")
	assert.NotContains(t, out, "T200000", "no wall clock may be published for it either")
	// The properties that make a re-sync an UPDATE rather than a duplicate are
	// untouched by the withholding.
	assert.Contains(t, out, "UID:")
	assert.Contains(t, out, "DTSTAMP:")
	assert.Contains(t, out, "SEQUENCE:")
}

// UID stability is what makes an edit update the event instead of duplicating
// it, so it must survive changes to every mutable field.
func TestVenueFeed_UIDSurvivesEdits(t *testing.T) {
	original := testShow(99, nil)

	edited := testShow(99, func(s *catalogm.Show) {
		s.Title = "Completely Different Title"
		s.EventDate = s.EventDate.Add(72 * time.Hour)
		newSlug := "renamed-show"
		s.Slug = &newSlug
		s.UpdatedAt = s.CreatedAt.Add(90 * time.Second)
	})

	before := renderFeed(testVenue(nil), []catalogm.Show{original}, nil)
	after := renderFeed(testVenue(nil), []catalogm.Show{edited}, nil)

	const uid = "UID:show-99@psychichomily.com"
	assert.Contains(t, before, uid)
	assert.Contains(t, after, uid, "UID must derive only from the immutable show ID")

	// ...and the revision counter must move, or clients ignore the update.
	assert.Contains(t, before, "SEQUENCE:0")
	assert.Contains(t, after, "SEQUENCE:90")
}

func TestShowSequence_IsMonotonicAndBounded(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, 0, showSequence(created, created), "an unedited show starts at 0")
	assert.Equal(t, 3600, showSequence(created, created.Add(time.Hour)))
	assert.Equal(t, 0, showSequence(created, created.Add(-time.Hour)),
		"clock skew must not produce a negative SEQUENCE")
	assert.Equal(t, 2147483647, showSequence(created, created.AddDate(200, 0, 0)),
		"SEQUENCE must stay inside the 32-bit range clients assume")
}

// A cancelled show must remain in the feed carrying STATUS:CANCELLED. Dropping
// it makes the event vanish silently, which reads to a subscriber as "the show
// is still on, my calendar just lost it".
func TestVenueFeed_CancelledShowIsPublishedAsCancelled(t *testing.T) {
	shows := []catalogm.Show{
		testShow(1, nil),
		testShow(2, func(s *catalogm.Show) {
			s.Title = "Doomed Gig"
			s.IsCancelled = true
		}),
	}
	out := renderFeed(testVenue(nil), shows, nil)

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, parsed.Events(), 2, "a cancelled show must NOT disappear from the feed")

	assert.Contains(t, out, "STATUS:CANCELLED")
	assert.Contains(t, out, "UID:show-2@psychichomily.com")
	assert.Contains(t, out, "SUMMARY:CANCELLED: Doomed Gig",
		"clients that ignore STATUS still need a human-visible signal")
	assert.Contains(t, unfold(out), "This show has been cancelled.")
}

func TestVenueFeed_SoldOutIsMarked(t *testing.T) {
	out := renderFeed(testVenue(nil), []catalogm.Show{
		testShow(1, func(s *catalogm.Show) { s.IsSoldOut = true }),
	}, nil)

	assert.Contains(t, out, "SUMMARY:Night of Noise [SOLD OUT]")
	assert.Contains(t, out, "STATUS:CONFIRMED", "sold out is not cancelled")
}

// ICS is line-oriented and community members name venues and shows. golang-ical
// escapes LF but NOT CR, so an unsanitized CR could terminate a property line
// for a lenient parser and let the rest of the value forge calendar properties.
func TestVenueFeed_RejectsPropertyInjectionFromCommunityText(t *testing.T) {
	venue := testVenue(func(v *contracts.VenueDetailResponse) {
		v.Name = "Evil Room\r\nX-FORGED-VENUE:yes"
	})
	shows := []catalogm.Show{
		testShow(1, func(s *catalogm.Show) {
			s.Title = "Gig\r\nSUMMARY:Hijacked\r\nDTSTART:19700101T000000Z"
		}),
	}
	artists := map[uint][]string{1: {"Band\r\nATTENDEE:mailto:victim@example.com"}}

	out := renderFeed(venue, shows, artists)

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "hostile text must still produce a parseable calendar")
	require.Len(t, parsed.Events(), 1)
	event := parsed.Events()[0]

	// The payload must contain no bare CR outside the CRLF line endings — that
	// is the character golang-ical does not escape and the one a lenient parser
	// could treat as a line break.
	assert.NotContains(t, strings.ReplaceAll(out, "\r\n", ""), "\r",
		"no raw carriage returns may survive into the payload")

	// The injected text must survive only as INERT VALUE inside the property it
	// was typed into — never as a property of its own.
	require.NotNil(t, event.GetProperty(ics.ComponentPropertySummary))
	assert.Equal(t, "GigSUMMARY:HijackedDTSTART:19700101T000000Z",
		event.GetProperty(ics.ComponentPropertySummary).Value,
		"the whole hostile string must stay inside SUMMARY's value")

	assert.Nil(t, event.GetProperty("X-FORGED-VENUE"), "a venue name must not be able to add a property")
	assert.Nil(t, event.GetProperty(ics.ComponentPropertyAttendee), "an artist name must not add an attendee")

	// Exactly one real DTSTART, and it is the venue-local one.
	dtstarts := event.GetProperties(ics.ComponentPropertyDtStart)
	require.Len(t, dtstarts, 1, "injected text must not add a second DTSTART")
	assert.Equal(t, "20260814T200000", dtstarts[0].Value)
	assert.NotEqual(t, "19700101T000000Z", dtstarts[0].Value)
}

func TestVenueFeed_UntitledShowNamedAfterBill(t *testing.T) {
	// Both calendar surfaces emit the same UID per show, so they must agree on
	// its name — an untitled show is named after its bill, exactly as the
	// per-show download names it, and only a bill-less show falls back to the
	// venue.
	out := renderFeed(testVenue(nil), []catalogm.Show{testShow(1, func(s *catalogm.Show) {
		s.Title = ""
	})}, map[uint][]string{1: {"Headliner Band", "Support Act"}})
	assert.Contains(t, unfold(out), "SUMMARY:Headliner Band\\, Support Act")

	bare := renderFeed(testVenue(nil), []catalogm.Show{testShow(1, func(s *catalogm.Show) {
		s.Title = ""
	})}, nil)
	assert.Contains(t, unfold(bare), "SUMMARY:Show at The Rebel Lounge")
}

func TestSanitizeICSText(t *testing.T) {
	assert.Equal(t, "cleanSUMMARY:x", sanitizeICSText("clean\r\nSUMMARY:x"))
	assert.Equal(t, "tab separated", sanitizeICSText("tab\tseparated"))
	assert.Equal(t, "trimmed", sanitizeICSText("  trimmed  "))
	assert.Equal(t, "", sanitizeICSText("\x00\x01\x02"))
	assert.Equal(t, "Café Möbius", sanitizeICSText("Café Möbius"), "non-ASCII names must survive")
}

// The service reads the venue through VenueServiceInterface precisely so that
// address redaction for unverified venues is honoured. A nil address must not
// leak an empty ", ," into LOCATION.
func TestVenueFeed_OmitsRedactedAddress(t *testing.T) {
	venue := testVenue(func(v *contracts.VenueDetailResponse) { v.Address = nil })
	out := renderFeed(venue, []catalogm.Show{testShow(1, nil)}, nil)

	assert.Contains(t, out, "LOCATION:The Rebel Lounge\\, Phoenix\\, AZ")
	assert.NotContains(t, out, "2303 E Indian School Rd")
}

func TestVenueFeed_LinksBackToShowPage(t *testing.T) {
	out := unfold(renderFeed(testVenue(nil), []catalogm.Show{testShow(5, nil)}, nil))
	assert.Contains(t, out, "https://psychichomily.com/shows/show-5-slug")
}

func TestVenueFeed_FallsBackToShowIDWhenSlugMissing(t *testing.T) {
	out := unfold(renderFeed(testVenue(nil), []catalogm.Show{
		testShow(5, func(s *catalogm.Show) { s.Slug = nil }),
	}, nil))
	assert.Contains(t, out, "https://psychichomily.com/shows/5")
}

func TestVenueFeed_EmptyVenueStillValid(t *testing.T) {
	out := renderFeed(testVenue(nil), nil, nil)

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "a venue with no upcoming shows must still serve a valid calendar")
	assert.Empty(t, parsed.Events())
	assert.Contains(t, out, "X-WR-CALNAME:The Rebel Lounge — Shows")
}

// Identical database state must render byte-identical output, or the ETag
// changes on every poll and the 304 path never fires.
func TestVenueFeed_RenderIsDeterministic(t *testing.T) {
	shows := []catalogm.Show{testShow(1, nil), testShow(2, nil)}
	first := renderFeed(testVenue(nil), shows, map[uint][]string{1: {"A Band"}})
	second := renderFeed(testVenue(nil), shows, map[uint][]string{1: {"A Band"}})

	assert.Equal(t, first, second,
		"DTSTAMP must come from the show's UpdatedAt, not time.Now(), or ETags churn")
}

func TestVenueFeedCache_RoundTripAndBound(t *testing.T) {
	svc := &VenueCalendarService{cache: map[uint]venueFeedCacheEntry{}}
	feed := contracts.VenueCalendarFeed{VenueName: "V", VenueSlug: "v", ICS: []byte("BEGIN:VCALENDAR"), ETag: `W/"abc"`}

	svc.storeFeed(1, feed)
	got, ok := svc.cachedFeed(1)
	require.True(t, ok)
	assert.Equal(t, feed.ETag, got.ETag)

	// A returned payload must not alias the cached bytes.
	got.ICS[0] = 'X'
	again, ok := svc.cachedFeed(1)
	require.True(t, ok)
	assert.Equal(t, "BEGIN:VCALENDAR", string(again.ICS), "cache must hand out copies")

	// Expired entries are misses.
	svc.cache[2] = venueFeedCacheEntry{feed: feed, expiresAt: time.Now().Add(-time.Second)}
	_, ok = svc.cachedFeed(2)
	assert.False(t, ok)

	// Slug enumeration must not grow the cache without bound.
	for i := uint(0); i < venueFeedCacheMaxEntries*3; i++ {
		svc.storeFeed(i, feed)
	}
	assert.LessOrEqual(t, len(svc.cache), venueFeedCacheMaxEntries,
		"an unbounded cache is a memory-exhaustion surface on a public endpoint")
}

// A public feed is polled concurrently by many clients, so the cache is shared
// mutable state on the hot path. Run under -race, this is what proves the
// locking is real rather than plausible.
func TestVenueFeedCache_ConcurrentAccessIsSafe(t *testing.T) {
	svc := &VenueCalendarService{cache: map[uint]venueFeedCacheEntry{}}
	feed := contracts.VenueCalendarFeed{
		VenueName: "V",
		VenueSlug: "v",
		ICS:       []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"),
		ETag:      `W/"abc"`,
	}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(2)
		go func(n uint) {
			defer wg.Done()
			svc.storeFeed(n%8, feed)
		}(uint(i))
		go func(n uint) {
			defer wg.Done()
			if got, ok := svc.cachedFeed(n % 8); ok {
				// Mutating a returned copy must not corrupt the cache.
				got.ICS[0] = 'X'
			}
		}(uint(i))
	}
	wg.Wait()

	for id := uint(0); id < 8; id++ {
		if got, ok := svc.cachedFeed(id); ok {
			assert.True(t, strings.HasPrefix(string(got.ICS), "BEGIN:VCALENDAR"),
				"cached payload was corrupted by a caller mutating its copy")
		}
	}
}
