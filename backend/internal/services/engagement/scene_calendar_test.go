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

	"psychic-homily-backend/internal/services/contracts"
)

// These tests exercise the pure rendering half of the scene feed — everything
// from a resolved scene plus a slice of show summaries to the serialized
// calendar. That is where every silent-failure mode of an ICS feed lives (wrong
// local time, unstable UID, unescaped text, a leaked address), and none of it
// needs a database.

const sceneTestFrontendURL = "https://psychichomily.com"

var sceneRevisionCreated = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func testSceneShow(id uint, overrides func(*contracts.SceneShowSummary)) contracts.SceneShowSummary {
	show := contracts.SceneShowSummary{
		ID:    id,
		Slug:  fmt.Sprintf("show-%d-slug", id),
		Title: "Night of Noise",
		// 20:00 Phoenix on Aug 14.
		StartsAt:      time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC),
		VenueName:     "The Rebel Lounge",
		VenueSlug:     "the-rebel-lounge",
		VenueAddress:  "2303 E Indian School Rd",
		VenueCity:     "Phoenix",
		VenueState:    "AZ",
		VenueTimezone: "America/Phoenix",
	}
	if overrides != nil {
		overrides(&show)
	}
	return show
}

// revisionsFor supplies the DTSTAMP/SEQUENCE metadata the service loads
// separately, at a fixed instant so renders stay comparable.
func revisionsFor(shows []contracts.SceneShowSummary, edit func(uint) showCalendarRevision) map[uint]showCalendarRevision {
	out := make(map[uint]showCalendarRevision, len(shows))
	for _, show := range shows {
		if edit != nil {
			out[show.ID] = edit(show.ID)
			continue
		}
		out[show.ID] = showCalendarRevision{
			ID:        show.ID,
			CreatedAt: sceneRevisionCreated,
			UpdatedAt: sceneRevisionCreated,
		}
	}
	return out
}

// renderSceneFeed drives buildSceneCalendar directly — no DB, no scene service.
func renderSceneFeed(shows []contracts.SceneShowSummary) string {
	return string(buildSceneCalendar("Phoenix", "AZ", shows, revisionsFor(shows, nil), sceneTestFrontendURL))
}

// unfoldScene reverses RFC 5545 3.1 line folding so a test can search the feed's
// VALUES rather than its wire bytes. A property longer than 75 octets is split
// across lines with CRLF plus one leading space or tab, which silently defeats a
// plain substring assertion — a privacy test that greps for an address it must
// NOT find would pass while the address sits in the payload, folded.
func unfoldScene(feed string) string {
	unfolded := strings.ReplaceAll(feed, "\r\n ", "")
	unfolded = strings.ReplaceAll(unfolded, "\r\n\t", "")
	unfolded = strings.ReplaceAll(unfolded, "\n ", "")
	return strings.ReplaceAll(unfolded, "\n\t", "")
}

// sceneEventLines narrows the feed to the content lines of its FIRST VEVENT, so
// a test can count REAL properties instead of substring-matching wire bytes.
// Counting inside the component matters because the VCALENDAR carries its own
// NAME / DESCRIPTION.
func sceneEventLines(t *testing.T, feed string) []string {
	t.Helper()
	normalized := strings.ReplaceAll(unfoldScene(feed), "\r\n", "\n")
	var (
		event  []string
		inside bool
		done   bool
	)
	for _, line := range strings.Split(strings.TrimRight(normalized, "\n"), "\n") {
		switch {
		case line == "BEGIN:VEVENT" && !done:
			inside = true
		case line == "END:VEVENT" && inside:
			inside, done = false, true
		case inside:
			event = append(event, line)
		}
	}
	require.NotEmpty(t, event, "feed must contain a VEVENT")
	return event
}

func countSceneProperty(lines []string, name string) int {
	n := 0
	for _, line := range lines {
		if strings.HasPrefix(line, name+":") || strings.HasPrefix(line, name+";") {
			n++
		}
	}
	return n
}

func TestSceneFeed_ParsesAsCalendar(t *testing.T) {
	out := renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(1, func(s *contracts.SceneShowSummary) {
			s.ArtistNames = []string{"Headliner Band", "Support Act"}
		}),
	})

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "feed must round-trip through an iCalendar parser")
	require.Len(t, parsed.Events(), 1)

	assert.Contains(t, out, "BEGIN:VCALENDAR")
	assert.Contains(t, out, "END:VCALENDAR")
	assert.Contains(t, out, "VERSION:2.0")
	assert.Contains(t, out, "PRODID:"+sceneFeedCalendarProductID)
	assert.Contains(t, out, "METHOD:PUBLISH")
	assert.Contains(t, out, "X-WR-CALNAME:Shows in Phoenix\\, AZ")
	// Commas are escaped as `\,` inside a TEXT value per RFC 5545.
	assert.Contains(t, unfoldScene(out), `Artists: Headliner Band\, Support Act`,
		"artists must be listed in billed order")
}

// RFC 5545 3.6.1 requires UID and DTSTAMP on every VEVENT. golang-ical supplies
// neither DTSTAMP nor SEQUENCE on its own, so their absence would be silent.
func TestSceneFeed_EmitsRequiredEventProperties(t *testing.T) {
	out := renderSceneFeed([]contracts.SceneShowSummary{testSceneShow(42, nil)})

	for _, required := range []string{
		"UID:show-42@psychichomily.com",
		"DTSTAMP:",
		"SEQUENCE:",
		"STATUS:CONFIRMED",
		"SUMMARY:Night of Noise",
	} {
		assert.Contains(t, out, required, "every VEVENT needs %s", required)
	}

	// CRLF line endings are mandatory (RFC 5545 3.1); a bare-LF feed is rejected
	// outright by some clients.
	assert.NotContains(t, strings.ReplaceAll(out, "\r\n", ""), "\n",
		"all line breaks must be CRLF")
}

// The UID and SEQUENCE a scene feed emits must match what the venue feed emits
// for the same show. Every calendar surface shares one UID per show, and RFC
// 5546 3.2 resolves a UID collision in favour of the higher SEQUENCE — a scene
// feed stuck at 0 would always lose to the venue feed's copy.
func TestSceneFeed_SharesEventIdentityWithSiblingSurfaces(t *testing.T) {
	shows := []contracts.SceneShowSummary{testSceneShow(99, nil)}
	edited := buildSceneCalendar("Phoenix", "AZ", shows,
		revisionsFor(shows, func(id uint) showCalendarRevision {
			return showCalendarRevision{
				ID:        id,
				CreatedAt: sceneRevisionCreated,
				UpdatedAt: sceneRevisionCreated.Add(90 * time.Second),
			}
		}), sceneTestFrontendURL)

	assert.Contains(t, string(edited), "UID:"+showEventUID(99))
	assert.Contains(t, string(edited), "SEQUENCE:90",
		"the revision counter must move with the show's UpdatedAt")
	assert.Contains(t, renderSceneFeed(shows), "SEQUENCE:0", "an unedited show starts at 0")
}

// The scene-specific risk the venue feed does not have: a feed spans rooms, so
// each event must carry its OWN venue's zone. Hoisting one zone to the whole
// calendar would put half a multi-metro feed at the wrong hour.
func TestSceneFeed_EachEventUsesItsOwnVenueTimezone(t *testing.T) {
	out := renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(1, nil),
		testSceneShow(2, func(s *contracts.SceneShowSummary) {
			s.VenueName = "Empty Bottle"
			s.VenueCity = "Chicago"
			s.VenueState = "IL"
			s.VenueTimezone = "America/Chicago"
			s.VenueAddress = ""
		}),
	})

	assert.Contains(t, out, "DTSTART;TZID=America/Phoenix:20260814T200000",
		"stored 2026-08-15T03:00Z must render as 20:00 Phoenix on Aug 14")
	assert.Contains(t, out, "DTEND;TZID=America/Phoenix:20260814T230000",
		"end time must stay in the room's zone")
	assert.Contains(t, out, "DTSTART;TZID=America/Chicago:20260814T220000",
		"the same instant is 22:00 in Chicago — a scene feed must not hoist one zone")
	assert.NotContains(t, out, "DTSTART:20260815T030000Z",
		"a bare UTC DTSTART lets each client re-shift a fixed-location event")
}

// A venue row with no explicit IANA zone must still resolve through the shared
// state fallback rather than silently landing in UTC.
func TestSceneFeed_FallsBackToStateTimezone(t *testing.T) {
	out := renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(1, func(s *contracts.SceneShowSummary) {
			s.VenueTimezone = ""
			s.VenueCity = "Chicago"
			s.VenueState = "IL"
		}),
	})

	assert.Contains(t, out, "DTSTART;TZID=America/Chicago:",
		"a missing venues.timezone must fall back to the state map, not UTC")
}

// A cancelled show must remain in the feed carrying STATUS:CANCELLED. Dropping
// it makes the event vanish silently, which reads to a subscriber as "the show
// is still on, my calendar just lost it".
func TestSceneFeed_CancelledShowIsPublishedAsCancelled(t *testing.T) {
	out := renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(1, nil),
		testSceneShow(2, func(s *contracts.SceneShowSummary) {
			s.Title = "Doomed Gig"
			s.IsCancelled = true
		}),
	})

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, parsed.Events(), 2, "a cancelled show must NOT disappear from the feed")

	assert.Contains(t, out, "STATUS:CANCELLED")
	assert.Contains(t, out, "UID:show-2@psychichomily.com")
	assert.Contains(t, out, "SUMMARY:CANCELLED: Doomed Gig",
		"clients that ignore STATUS still need a human-visible signal")
	assert.Contains(t, unfoldScene(out), "This show has been cancelled.")
}

func TestSceneFeed_SoldOutIsMarked(t *testing.T) {
	out := renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(1, func(s *contracts.SceneShowSummary) { s.IsSoldOut = true }),
	})

	assert.Contains(t, out, "SUMMARY:Night of Noise [SOLD OUT]")
	assert.Contains(t, out, "STATUS:CONFIRMED", "sold out is not cancelled")
}

func TestSceneFeed_UntitledShowNamedAfterBill(t *testing.T) {
	// Every calendar surface emits the same UID per show, so they must agree on
	// its name — an untitled show is named after its bill, and only a bill-less
	// show falls back to the venue.
	billed := renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(1, func(s *contracts.SceneShowSummary) {
			s.Title = ""
			s.ArtistNames = []string{"Headliner Band", "Support Act"}
		}),
	})
	assert.Contains(t, unfoldScene(billed), "SUMMARY:Headliner Band\\, Support Act")

	bare := renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(1, func(s *contracts.SceneShowSummary) { s.Title = "" }),
	})
	assert.Contains(t, unfoldScene(bare), "SUMMARY:Show at The Rebel Lounge")
}

// ICS is line-oriented and community members name venues, shows and bands.
// golang-ical escapes LF but NOT CR, so an unsanitized CR could terminate a
// property line for a lenient parser and let the rest of the value forge
// calendar properties.
func TestSceneFeed_RejectsPropertyInjectionFromCommunityText(t *testing.T) {
	shows := []contracts.SceneShowSummary{
		testSceneShow(1, func(s *contracts.SceneShowSummary) {
			s.Title = "Gig\r\nSUMMARY:Hijacked\r\nDTSTART:19700101T000000Z"
			s.VenueName = "Evil Room\r\nX-FORGED-VENUE:yes"
			s.VenueAddress = "1 Main St\r\nX-FORGED-ADDRESS:yes"
			s.ArtistNames = []string{"Band\r\nATTENDEE:mailto:victim@example.com"}
		}),
	}
	out := string(buildSceneCalendar(
		"Phoe\r\nX-FORGED-SCENE:yes", "AZ", shows, revisionsFor(shows, nil), sceneTestFrontendURL))

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "hostile text must still produce a parseable calendar")
	require.Len(t, parsed.Events(), 1)
	event := parsed.Events()[0]

	// The payload must contain no bare CR outside the CRLF line endings — that is
	// the character golang-ical does not escape and the one a lenient parser could
	// treat as a line break. Checked per content line so folding cannot hide one.
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		assert.NotContains(t, line, "\r", "no raw carriage return may survive into the payload")
	}

	// The injected text must survive only as INERT VALUE inside the property it
	// was typed into — never as a property of its own.
	require.NotNil(t, event.GetProperty(ics.ComponentPropertySummary))
	assert.Equal(t, "GigSUMMARY:HijackedDTSTART:19700101T000000Z",
		event.GetProperty(ics.ComponentPropertySummary).Value,
		"the whole hostile string must stay inside SUMMARY's value")

	lines := sceneEventLines(t, out)
	assert.Equal(t, 1, countSceneProperty(lines, "SUMMARY"))
	assert.Equal(t, 1, countSceneProperty(lines, "DTSTART"))
	assert.Nil(t, event.GetProperty("X-FORGED-VENUE"), "a venue name must not add a property")
	assert.Nil(t, event.GetProperty("X-FORGED-ADDRESS"), "a venue address must not add a property")
	assert.Nil(t, event.GetProperty("X-FORGED-SCENE"), "a scene name must not add a property")
	assert.Nil(t, event.GetProperty(ics.ComponentPropertyAttendee), "an artist name must not add an attendee")

	// Exactly one real DTSTART, and it is the venue-local one.
	dtstarts := event.GetProperties(ics.ComponentPropertyDtStart)
	require.Len(t, dtstarts, 1, "injected text must not add a second DTSTART")
	assert.Equal(t, "20260814T200000", dtstarts[0].Value)
}

// URL is the one property golang-ical does NOT escape: it types the value as a
// URI and writes it verbatim, control characters included. A slug carrying CRLF
// would emit real forged properties, so the sanitization has to happen at the
// shared chokepoint (showPageURL) rather than being left to the TEXT escaper
// that never runs here. (PSY-1701 found this on the sibling surfaces.)
func TestSceneFeed_HostileSlugCannotForgeAUrlProperty(t *testing.T) {
	shows := []contracts.SceneShowSummary{
		testSceneShow(7, func(s *contracts.SceneShowSummary) {
			s.Slug = "evil\r\nATTENDEE;RSVP=TRUE:mailto:victim@example.com\r\nX-FORGED-URL:yes"
		}),
	}
	out := string(buildSceneCalendar("Phoenix", "AZ", shows, revisionsFor(shows, nil), sceneTestFrontendURL))

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, parsed.Events(), 1)
	event := parsed.Events()[0]

	lines := sceneEventLines(t, out)
	assert.Equal(t, 1, countSceneProperty(lines, "URL"), "a slug must not add a second URL property")
	assert.Nil(t, event.GetProperty("X-FORGED-URL"))
	assert.Nil(t, event.GetProperty(ics.ComponentPropertyAttendee),
		"URL is written verbatim, so an unsanitized slug would forge a real ATTENDEE")
	assert.Equal(t,
		"https://psychichomily.com/shows/evilATTENDEE;RSVP=TRUE:mailto:victim@example.comX-FORGED-URL:yes",
		event.GetProperty(ics.ComponentPropertyUrl).Value,
		"the hostile slug must stay inert inside the URL value")
}

// NEL (U+0085), LS (U+2028) and PS (U+2029) are not content-line delimiters to a
// conformant parser, which splits on CRLF alone. Plenty of consumers are not
// conformant: anything that decodes to a string and splits on its platform's
// notion of a line break sees three more delimiters than the RFC does.
func TestSceneFeed_StripsUnicodeLineSeparators(t *testing.T) {
	out := renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(8, func(s *contracts.SceneShowSummary) {
			s.Title = "Gig\u2028X-FORGED-LS:yes\u2029X-FORGED-PS:yes\u0085X-FORGED-NEL:yes"
		}),
	})

	for _, sep := range []string{"\u0085", "\u2028", "\u2029"} {
		assert.NotContains(t, out, sep, "no Unicode line separator may reach the payload")
	}
	assert.Equal(t, 1, countSceneProperty(sceneEventLines(t, out), "SUMMARY"))

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, parsed.Events(), 1)
	assert.Equal(t, "GigX-FORGED-LS:yesX-FORGED-PS:yesX-FORGED-NEL:yes",
		parsed.Events()[0].GetProperty(ics.ComponentPropertySummary).Value,
		"the separators are dropped and the surrounding text stays inert")
}

// The scene service serves a street address for VERIFIED venues only, so an
// unverified room arrives here with an empty one. A public calendar feed is the
// worst place for a house-show address to surface: an ICS LOCATION is copied
// onto every subscriber's device, where a later redaction can never reach it.
func TestSceneFeed_OmitsRedactedAddress(t *testing.T) {
	out := unfoldScene(renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(1, func(s *contracts.SceneShowSummary) {
			s.VenueName = "Someone's Basement"
			s.VenueAddress = "" // what the redaction gate produces
		}),
	}))

	assert.Contains(t, out, "LOCATION:Someone's Basement\\, Phoenix\\, AZ",
		"the room is still named — only the street address is withheld")
	assert.NotContains(t, out, ", ,", "an absent address must not leave an empty segment")
}

func TestSceneFeed_LinksBackToShowPage(t *testing.T) {
	out := unfoldScene(renderSceneFeed([]contracts.SceneShowSummary{testSceneShow(5, nil)}))
	assert.Contains(t, out, "https://psychichomily.com/shows/show-5-slug")
}

func TestSceneFeed_FallsBackToShowIDWhenSlugMissing(t *testing.T) {
	out := unfoldScene(renderSceneFeed([]contracts.SceneShowSummary{
		testSceneShow(5, func(s *contracts.SceneShowSummary) { s.Slug = "" }),
	}))
	assert.Contains(t, out, "https://psychichomily.com/shows/5")
}

// A real city with a quiet stretch is a fact, not an error. The feed must stay a
// valid, subscribable calendar so a client that already added it does not treat
// the empty response as a broken URL.
func TestSceneFeed_EmptySceneStillValid(t *testing.T) {
	out := renderSceneFeed(nil)

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err, "a scene with no upcoming shows must still serve a valid calendar")
	assert.Empty(t, parsed.Events())
	assert.Contains(t, out, "X-WR-CALNAME:Shows in Phoenix\\, AZ")
	assert.Contains(t, out, "END:VCALENDAR")
}

// A show deleted between the shows query and the revision query has no revision
// row. It is no longer a show, so it is dropped rather than published with a
// zero DTSTAMP a client would have to guess at.
func TestSceneFeed_DropsShowsWithNoRevisionRow(t *testing.T) {
	shows := []contracts.SceneShowSummary{testSceneShow(1, nil), testSceneShow(2, nil)}
	revisions := revisionsFor(shows, nil)
	delete(revisions, 2)

	out := string(buildSceneCalendar("Phoenix", "AZ", shows, revisions, sceneTestFrontendURL))

	parsed, err := ics.ParseCalendar(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, parsed.Events(), 1)
	assert.Contains(t, out, "UID:show-1@psychichomily.com")
	assert.NotContains(t, out, "UID:show-2@psychichomily.com")
	assert.NotContains(t, out, "DTSTAMP:00010101T000000Z", "a zero DTSTAMP must never be published")
}

// Identical database state must render byte-identical output, or the ETag
// changes on every poll and the 304 path never fires.
func TestSceneFeed_RenderIsDeterministic(t *testing.T) {
	shows := []contracts.SceneShowSummary{testSceneShow(1, nil), testSceneShow(2, nil)}

	assert.Equal(t, renderSceneFeed(shows), renderSceneFeed(shows),
		"DTSTAMP must come from the show's UpdatedAt, not time.Now(), or ETags churn")
}

// The upcoming cutoff is judged per show against the start of ITS OWN venue's
// local today. This is the rule the 24-hour query slack exists to serve, and
// getting it wrong drops tonight's show from the feed hours early.
func TestShowHasNotHappenedYet_JudgedInTheVenuesOwnZone(t *testing.T) {
	// 2026-08-15T05:00Z is 22:00 on Aug 14 in Phoenix and 00:00 on Aug 15 in
	// Chicago, so the two rooms disagree about which local day it is.
	now := time.Date(2026, 8, 15, 5, 0, 0, 0, time.UTC)

	phoenixEarlier := testSceneShow(1, func(s *contracts.SceneShowSummary) {
		s.StartsAt = time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) // 20:00 Aug 14 Phoenix
	})
	assert.True(t, showHasNotHappenedYet(phoenixEarlier, now),
		"a set that started earlier tonight is still tonight's show")

	phoenixYesterday := testSceneShow(2, func(s *contracts.SceneShowSummary) {
		s.StartsAt = time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC) // 20:00 Aug 13 Phoenix
	})
	assert.False(t, showHasNotHappenedYet(phoenixYesterday, now),
		"last night's show must not stay in an upcoming feed")

	// Same instant as phoenixEarlier, but in Chicago it fell on the PREVIOUS
	// local day — the case a single scene-wide zone would get wrong.
	chicagoYesterday := testSceneShow(3, func(s *contracts.SceneShowSummary) {
		s.StartsAt = time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) // 22:00 Aug 14 Chicago
		s.VenueCity = "Chicago"
		s.VenueState = "IL"
		s.VenueTimezone = "America/Chicago"
	})
	assert.False(t, showHasNotHappenedYet(chicagoYesterday, now),
		"the Chicago day has already rolled over at this instant")

	// A room with no stored zone still resolves through the state map.
	noZone := testSceneShow(4, func(s *contracts.SceneShowSummary) {
		s.StartsAt = time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
		s.VenueTimezone = ""
	})
	assert.True(t, showHasNotHappenedYet(noZone, now))
}

func TestSceneFeedSlugAndCacheKey(t *testing.T) {
	assert.Equal(t, "phoenix-az", sceneFeedSlug("Phoenix", "AZ"))
	assert.Equal(t, "los-angeles-ca", sceneFeedSlug("Los Angeles", "CA"))

	// The separator cannot appear in either half, so no two scenes can collide.
	assert.NotEqual(t, sceneFeedCacheKey("a-b", "c"), sceneFeedCacheKey("a", "b-c"))
	assert.Equal(t, sceneFeedCacheKey("Phoenix", "AZ"), sceneFeedCacheKey("Phoenix", "AZ"))
}

func TestSceneFeedCache_RoundTripAndBound(t *testing.T) {
	svc := &SceneCalendarService{cache: map[string]sceneFeedCacheEntry{}}
	feed := contracts.SceneCalendarFeed{
		SceneName: "Phoenix, AZ",
		SceneSlug: "phoenix-az",
		ICS:       []byte("BEGIN:VCALENDAR"),
		ETag:      `W/"abc"`,
	}

	svc.storeFeed("k1", feed)
	got, ok := svc.cachedFeed("k1")
	require.True(t, ok)
	assert.Equal(t, feed.ETag, got.ETag)

	// A returned payload must not alias the cached bytes.
	got.ICS[0] = 'X'
	again, ok := svc.cachedFeed("k1")
	require.True(t, ok)
	assert.Equal(t, "BEGIN:VCALENDAR", string(again.ICS), "cache must hand out copies")

	// Expired entries are misses.
	svc.cache["k2"] = sceneFeedCacheEntry{feed: feed, expiresAt: time.Now().Add(-time.Second)}
	_, ok = svc.cachedFeed("k2")
	assert.False(t, ok)

	// Slug enumeration must not grow the cache without bound.
	for i := 0; i < sceneFeedCacheMaxEntries*3; i++ {
		svc.storeFeed(fmt.Sprintf("scene-%d", i), feed)
	}
	assert.LessOrEqual(t, len(svc.cache), sceneFeedCacheMaxEntries,
		"an unbounded cache is a memory-exhaustion surface on a public endpoint")
}

// A public feed is polled concurrently by many clients, so the cache is shared
// mutable state on the hot path. Run under -race, this is what proves the
// locking is real rather than plausible.
func TestSceneFeedCache_ConcurrentAccessIsSafe(t *testing.T) {
	svc := &SceneCalendarService{cache: map[string]sceneFeedCacheEntry{}}
	feed := contracts.SceneCalendarFeed{
		SceneName: "Phoenix, AZ",
		SceneSlug: "phoenix-az",
		ICS:       []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"),
		ETag:      `W/"abc"`,
	}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			svc.storeFeed(fmt.Sprintf("scene-%d", n%8), feed)
		}(i)
		go func(n int) {
			defer wg.Done()
			if got, ok := svc.cachedFeed(fmt.Sprintf("scene-%d", n%8)); ok {
				// Mutating a returned copy must not corrupt the cache.
				got.ICS[0] = 'X'
			}
		}(i)
	}
	wg.Wait()

	for id := 0; id < 8; id++ {
		if got, ok := svc.cachedFeed(fmt.Sprintf("scene-%d", id)); ok {
			assert.True(t, strings.HasPrefix(string(got.ICS), "BEGIN:VCALENDAR"),
				"cached payload was corrupted by a caller mutating its copy")
		}
	}
}
