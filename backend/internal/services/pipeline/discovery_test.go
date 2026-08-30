package pipeline

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// =============================================================================
// UNIT TESTS — parseEventDate
// =============================================================================

// A scraped date with no time is anchored at the venue-local evening, NOT at
// bare UTC midnight. Midnight UTC is 17:00 the PREVIOUS day in Phoenix, so the
// old behaviour filed the show under the wrong calendar day and dropped it from
// every upcoming bound 31 hours early (PSY-1780/PSY-1849/PSY-1861).
func TestParseEventDate_ISODateOnly_AnchorsVenueLocalEvening(t *testing.T) {
	result, err := parseEventDate("2026-01-25", nil, nil, "AZ")
	assert.NoError(t, err)
	// 20:00 Phoenix (UTC-7, no DST) on the 25th = 03:00 UTC on the 26th.
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), result)

	// The property that actually matters: read back in the venue's own zone, the
	// stored instant is the evening of the date that was scraped.
	phoenix, err := time.LoadLocation("America/Phoenix")
	assert.NoError(t, err)
	assert.Equal(t, "2026-01-25 20:00", result.In(phoenix).Format("2006-01-02 15:04"))
}

// The same date-only input across a DST boundary in a zone that observes one.
// Both must land on the scraped evening; only the UTC instant differs.
func TestParseEventDate_ISODateOnly_AnchorsAcrossDST(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	assert.NoError(t, err)

	winter, err := parseEventDate("2026-01-25", nil, nil, "NY")
	assert.NoError(t, err)
	// 20:00 EST (UTC-5) = 01:00 UTC next day.
	assert.Equal(t, time.Date(2026, 1, 26, 1, 0, 0, 0, time.UTC), winter)
	assert.Equal(t, "2026-01-25 20:00", winter.In(newYork).Format("2006-01-02 15:04"))

	summer, err := parseEventDate("2026-08-15", nil, nil, "NY")
	assert.NoError(t, err)
	// 20:00 EDT (UTC-4) is EXACTLY UTC midnight on the 16th. This is the case
	// that makes a read-side repair impossible: a correctly anchored date-only
	// show and a genuine 8pm Eastern show are the same instant, and the stored
	// row carries nothing else to tell them apart. It is also why the fix has to
	// happen at the write boundary, where the submitted offset still exists.
	assert.Equal(t, time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC), summer)
	assert.Equal(t, "2026-08-15 20:00", summer.In(newYork).Format("2006-01-02 15:04"))
}

// The days either side of a spring-forward and a fall-back. 20:00 exists on all
// of them, so the anchor must simply land on the right evening; a transition
// must not push it onto an adjacent date, which is the failure mode a fixed
// +N-hours offset (rather than a real zone) would have.
func TestParseEventDate_AnchorsAcrossDSTTransitionDays(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York unavailable: %v", err)
	}
	// US transitions in 2026: spring forward Mar 8, fall back Nov 1.
	for _, date := range []string{"2026-03-07", "2026-03-08", "2026-03-09", "2026-10-31", "2026-11-01", "2026-11-02"} {
		got, err := parseEventDate(date, nil, nil, "NY")
		assert.NoError(t, err)
		assert.Equal(t, date+" 20:00", got.In(newYork).Format("2006-01-02 15:04"),
			"date-only import of %s must anchor on that evening in New York", date)
	}
}

// An unknown state still anchors — on EventLocation's documented Phoenix
// default. The anchor must never be skipped for want of a zone, because the
// fallback for "no zone" was the corrupt midnight value.
func TestParseEventDate_UnknownState_AnchorsOnDefaultZone(t *testing.T) {
	result, err := parseEventDate("2026-01-25", nil, nil, "XX")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_ISOTimestamp(t *testing.T) {
	result, err := parseEventDate("2026-01-25T19:00:00Z", nil, nil, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 19, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_RFC3339(t *testing.T) {
	result, err := parseEventDate("2026-01-25T19:00:00-07:00", nil, nil, "AZ")
	assert.NoError(t, err)
	expected := time.Date(2026, 1, 26, 2, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, result)
}

func TestParseEventDate_WithShowTimePM_AZ(t *testing.T) {
	showTime := "7:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime, nil, "AZ")
	assert.NoError(t, err)
	// 7:00 PM Phoenix (UTC-7) = 2:00 AM UTC next day
	assert.Equal(t, time.Date(2026, 1, 26, 2, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_WithShowTimePM_NY(t *testing.T) {
	showTime := "8:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime, nil, "NY")
	assert.NoError(t, err)
	// 8:00 PM New York (UTC-5 in January) = 1:00 AM UTC next day
	assert.Equal(t, time.Date(2026, 1, 26, 1, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_WithShowTimeAM_AZ(t *testing.T) {
	showTime := "11:00 am"
	result, err := parseEventDate("2026-01-25", &showTime, nil, "AZ")
	assert.NoError(t, err)
	// 11:00 AM Phoenix (UTC-7) = 6:00 PM UTC same day
	assert.Equal(t, time.Date(2026, 1, 25, 18, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_12PM_AZ(t *testing.T) {
	showTime := "12:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime, nil, "AZ")
	assert.NoError(t, err)
	// 12:00 PM Phoenix (UTC-7) = 7:00 PM UTC same day
	assert.Equal(t, time.Date(2026, 1, 25, 19, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_12AM_AZ(t *testing.T) {
	showTime := "12:00 am"
	result, err := parseEventDate("2026-01-25", &showTime, nil, "AZ")
	assert.NoError(t, err)
	// 12:00 AM Phoenix (UTC-7) = 7:00 AM UTC same day
	assert.Equal(t, time.Date(2026, 1, 25, 7, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_WithSpacesInTime(t *testing.T) {
	showTime := " 7:00 PM "
	result, err := parseEventDate("2026-01-25", &showTime, nil, "AZ")
	assert.NoError(t, err)
	// 7:00 PM Phoenix (UTC-7) = 2:00 AM UTC next day
	assert.Equal(t, time.Date(2026, 1, 26, 2, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_EmptyShowTime(t *testing.T) {
	showTime := ""
	result, err := parseEventDate("2026-01-25", &showTime, nil, "AZ")
	assert.NoError(t, err)
	// Indistinguishable from no show time at all: anchored, not UTC midnight.
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_InvalidDate(t *testing.T) {
	_, err := parseEventDate("not-a-date", nil, nil, "AZ")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to parse date")
}

func TestParseEventDate_NilShowTime(t *testing.T) {
	result, err := parseEventDate("2026-01-25", nil, nil, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), result)
}

// A FULL timestamp states its own time of day and is never re-anchored, however
// that time was written. This is the guard that keeps the date-only rule from
// rewriting scraped instants: without it, a feed publishing midnight-UTC
// timestamps would be pushed a further 20 hours.
func TestParseEventDate_FullTimestampIsNeverReanchored(t *testing.T) {
	midnightUTC, err := parseEventDate("2026-01-25T00:00:00Z", nil, nil, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), midnightUTC)

	withOffset, err := parseEventDate("2026-01-25T20:00:00-07:00", nil, nil, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), withOffset)
}

func TestParseEventDate_UnparseableTime(t *testing.T) {
	showTime := "doors at 7"
	result, err := parseEventDate("2026-01-25", &showTime, nil, "AZ")
	assert.NoError(t, err)
	// An unparseable time is still ignored, but the date no longer falls through
	// as bare UTC midnight — it takes the same venue-local evening anchor a
	// missing time takes. A scraper emitting an unknown format now produces an
	// imprecise instant instead of one that is wrong by a calendar day.
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_UnknownStateDefaultsToPhoenix(t *testing.T) {
	showTime := "8:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime, nil, "XX")
	assert.NoError(t, err)
	// Unknown state defaults to America/Phoenix (UTC-7)
	// 8:00 PM Phoenix = 3:00 AM UTC next day
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), result)
}

// PSY-1873: the venue's own zone outranks the state map, which answers
// America/Phoenix for every state name it does not carry. VenueConfig is
// US-only today, so this is the guard that keeps the anchor and the slug (which
// already reads the venue zone) from diverging the day a non-US room is added.
func TestParseEventDate_VenueTimezoneOutranksStateMap(t *testing.T) {
	london := "Europe/London"

	// Date-only anchors at 20:00 venue-local. In GMT that is 20:00Z; the state
	// map would have made it 20:00 Phoenix, i.e. 03:00Z the NEXT day.
	dateOnly, err := parseEventDate("2026-01-23", nil, &london, "England")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 23, 20, 0, 0, 0, time.UTC), dateOnly)

	stateOnly, err := parseEventDate("2026-01-23", nil, nil, "England")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 24, 3, 0, 0, 0, time.UTC), stateOnly)

	// A stated wall-clock time reads in the same zone.
	showTime := "9:00 pm"
	stated, err := parseEventDate("2026-01-23", &showTime, &london, "England")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 23, 21, 0, 0, 0, time.UTC), stated)
}

// A US venue must not move: its stored zone and its state-map zone agree.
func TestParseEventDate_VenueTimezoneMatchesStateForUSVenue(t *testing.T) {
	phoenix := "America/Phoenix"
	showTime := "8:00 pm"

	withZone, err := parseEventDate("2026-01-25", &showTime, &phoenix, "AZ")
	assert.NoError(t, err)
	stateOnly, err := parseEventDate("2026-01-25", &showTime, nil, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, stateOnly, withZone)
}

// A venue row carrying an unloadable zone falls through to the state map rather
// than to UTC, matching utils.EventLocation's documented precedence.
func TestParseEventDate_MalformedVenueTimezoneFallsBackToState(t *testing.T) {
	bogus := "Mars/Olympus"
	showTime := "8:00 pm"

	result, err := parseEventDate("2026-01-25", &showTime, &bogus, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), result)
}

// resolveShowTimes must anchor doors/music in the SAME zone parseEventDate
// anchors event_date in, or a non-US show's stripe contradicts its own date.
func TestResolveShowTimes_UsesVenueTimezone(t *testing.T) {
	london := "Europe/London"
	doorsAt, musicAt := resolveShowTimes("2026-01-23", strPtr("7:00 pm"), strPtr("8:00 pm"), &london, "England")

	assert.NotNil(t, doorsAt)
	assert.NotNil(t, musicAt)
	assert.Equal(t, time.Date(2026, 1, 23, 19, 0, 0, 0, time.UTC), doorsAt.UTC())
	assert.Equal(t, time.Date(2026, 1, 23, 20, 0, 0, 0, time.UTC), musicAt.UTC())
}

func TestParseEventDate_DSTAwareState(t *testing.T) {
	// California in summer (PDT = UTC-7), vs winter (PST = UTC-8)
	showTime := "8:00 pm"

	// January (PST = UTC-8): 8:00 PM = 4:00 AM UTC next day
	winter, err := parseEventDate("2026-01-25", &showTime, nil, "CA")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 26, 4, 0, 0, 0, time.UTC), winter)

	// July (PDT = UTC-7): 8:00 PM = 3:00 AM UTC next day
	summer, err := parseEventDate("2026-07-15", &showTime, nil, "CA")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC), summer)
}

// =============================================================================
// UNIT TESTS — parseClockTime
// =============================================================================

func TestParseClockTime_SourceShapes(t *testing.T) {
	// Each case names the discovery provider whose formatter emits that shape,
	// so the corpus this parser has to cover stays visible from the test.
	cases := []struct {
		name   string
		raw    string
		hour   int
		minute int
		ok     bool
	}{
		// ticketweb.parseTime returns the site's own casing verbatim.
		{"ticketweb lowercase pm", "6:30 pm", 18, 30, true},
		{"ticketweb uppercase pm", "7:00 PM", 19, 0, true},
		// seetickets/emptybottle formatTime normalize to "H:MM AM/PM".
		{"seetickets doors", "8:00 PM", 20, 0, true},
		{"emptybottle start", "9:30 PM", 21, 30, true},
		// jsonld/wix extractTime derive the same shape from an ISO datetime.
		{"jsonld noon", "12:00 PM", 12, 0, true},
		{"jsonld midnight", "12:00 AM", 0, 0, true},
		{"jsonld early am", "1:15 AM", 1, 15, true},
		// Spacing and punctuation a raw passthrough can leak.
		{"no space before meridiem", "6:30PM", 18, 30, true},
		{"surrounding whitespace", "  7:00 pm  ", 19, 0, true},
		{"dotted meridiem", "7:00 p.m.", 19, 0, true},
		// Scraped HTML carries &nbsp;. ticketweb.parseTime captures the whole
		// clock including the separator, so the U+00A0 arrives verbatim.
		{"non-breaking space", "7:00\u00a0PM", 19, 0, true},
		{"tab separator", "7:00\tPM", 19, 0, true},
		// A feed handing a 24-hour clock straight through.
		{"24 hour evening", "19:00", 19, 0, true},
		{"24 hour midnight", "00:30", 0, 30, true},
		{"24 hour late", "23:45", 23, 45, true},

		// Not a time: the formatTime fallbacks return the raw text unchanged
		// when their regex misses, so unreadable strings reach this parser.
		{"prose", "doors at 7", 0, 0, false},
		{"bare hour", "8PM", 0, 0, false},
		{"empty", "", 0, 0, false},
		{"tbd", "TBD", 0, 0, false},
		{"range", "9pm - 1am", 0, 0, false},
		// A 24-hour clock wearing a meridiem is self-contradictory. The old
		// Sscanf parser turned this into hour 31 and rolled it into the next
		// day; refusing it keeps a fabricated instant out of the column.
		{"24 hour with meridiem", "19:00 pm", 0, 0, false},
		// A bare 1-12 clock could be either half of the day. emptybottle's
		// formatTime returns its input unchanged on a regex miss, so a
		// ".start-time" of "7:00" for a 7 PM show arrives here verbatim;
		// reading it as 7 AM would fabricate the time.
		{"ambiguous bare hour", "7:00", 0, 0, false},
		{"ambiguous bare noon", "12:00", 0, 0, false},
		{"ambiguous bare one", "1:00", 0, 0, false},
		{"hour zero with meridiem", "0:30 am", 0, 0, false},
		{"minute out of range", "7:75 pm", 0, 0, false},
		{"hour out of range", "25:00", 0, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hour, minute, ok := parseClockTime(tc.raw)
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.Equal(t, tc.hour, hour)
				assert.Equal(t, tc.minute, minute)
			}
		})
	}
}

// =============================================================================
// UNIT TESTS — resolveShowTimes
// =============================================================================

func strPtr(s string) *string { return &s }

func TestResolveShowTimes_BothStated_AZ(t *testing.T) {
	doorsAt, musicAt := resolveShowTimes("2026-01-25", strPtr("6:30 pm"), strPtr("7:00 pm"), nil, "AZ")
	// Phoenix is UTC-7 year round.
	assert.Equal(t, time.Date(2026, 1, 26, 1, 30, 0, 0, time.UTC), *doorsAt)
	assert.Equal(t, time.Date(2026, 1, 26, 2, 0, 0, 0, time.UTC), *musicAt)
}

func TestResolveShowTimes_MusicAtAgreesWithEventDate(t *testing.T) {
	// event_date and music_at both come off the stated show time, so they must
	// name the same instant. Anchoring them separately is the drift risk this
	// asserts against.
	showTime := "8:00 pm"
	eventDate, err := parseEventDate("2026-01-25", &showTime, nil, "NY")
	assert.NoError(t, err)

	_, musicAt := resolveShowTimes("2026-01-25", nil, &showTime, nil, "NY")
	assert.Equal(t, eventDate, *musicAt)
}

func TestResolveShowTimes_DoorsOnlyWritesNothing(t *testing.T) {
	// event_date comes off the SHOW time. Without one it is a midnight-UTC
	// placeholder that renders as the previous day in every US venue zone, so a
	// doors time anchored to the stated day would put the stripe and the date a
	// calendar day apart. The show keeps its date-only rendering instead.
	doorsAt, musicAt := resolveShowTimes("2026-06-15", strPtr("7:00 PM"), nil, nil, "IL")
	assert.Nil(t, doorsAt)
	assert.Nil(t, musicAt)
}

func TestResolveShowTimes_DoorsOnlyWithUnreadableShowTimeWritesNothing(t *testing.T) {
	doorsAt, musicAt := resolveShowTimes("2026-06-15", strPtr("7:00 PM"), strPtr("TBD"), nil, "IL")
	assert.Nil(t, doorsAt)
	assert.Nil(t, musicAt)
}

func TestResolveShowTimes_MusicOnly(t *testing.T) {
	// Empty Bottle's and wix's widgets state a start time only. That is the
	// anchor, so it writes on its own.
	doorsAt, musicAt := resolveShowTimes("2026-06-15", nil, strPtr("8:00 PM"), nil, "IL")
	assert.Nil(t, doorsAt)
	assert.Equal(t, time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC), *musicAt)
}

func TestResolveShowTimes_NeitherStated(t *testing.T) {
	doorsAt, musicAt := resolveShowTimes("2026-06-15", nil, nil, nil, "AZ")
	assert.Nil(t, doorsAt)
	assert.Nil(t, musicAt)
}

func TestResolveShowTimes_UnreadableTimesAreAbsent(t *testing.T) {
	// The site would rather show a date alone than a time it guessed at.
	doorsAt, musicAt := resolveShowTimes("2026-06-15", strPtr("doors at 7"), strPtr("TBD"), nil, "AZ")
	assert.Nil(t, doorsAt)
	assert.Nil(t, musicAt)
}

func TestResolveShowTimes_EmptyStringsAreAbsent(t *testing.T) {
	doorsAt, musicAt := resolveShowTimes("2026-06-15", strPtr(""), strPtr(""), nil, "AZ")
	assert.Nil(t, doorsAt)
	assert.Nil(t, musicAt)
}

func TestResolveShowTimes_UnreadableDoorsStillWritesTheShowTime(t *testing.T) {
	doorsAt, musicAt := resolveShowTimes("2026-06-15", strPtr("doors at 7"), strPtr("8:00 PM"), nil, "AZ")
	assert.Nil(t, doorsAt)
	assert.Equal(t, time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC), *musicAt)
}

func TestResolveShowTimes_ContradictoryPairWritesNeither(t *testing.T) {
	// "Doors 11:00 PM / Show 12:00 AM" is a listing that crossed midnight and
	// dropped the day. Anchoring both to the stated date puts the music 23
	// hours before the doors, so neither is written rather than inferring the
	// rollover the source never stated.
	doorsAt, musicAt := resolveShowTimes("2026-06-15", strPtr("11:00 PM"), strPtr("12:00 AM"), nil, "AZ")
	assert.Nil(t, doorsAt)
	assert.Nil(t, musicAt)
}

func TestResolveShowTimes_SimultaneousPairIsKept(t *testing.T) {
	// Equal times are odd but not contradictory; only music strictly before
	// doors is refused.
	doorsAt, musicAt := resolveShowTimes("2026-06-15", strPtr("8:00 PM"), strPtr("8:00 PM"), nil, "AZ")
	assert.NotNil(t, doorsAt)
	assert.Equal(t, *doorsAt, *musicAt)
}

func TestResolveShowTimes_DSTAwareState(t *testing.T) {
	showTime := strPtr("8:00 pm")

	// January in California is PST, UTC-8.
	_, winter := resolveShowTimes("2026-01-25", nil, showTime, nil, "CA")
	assert.Equal(t, time.Date(2026, 1, 26, 4, 0, 0, 0, time.UTC), *winter)

	// July is PDT, UTC-7.
	_, summer := resolveShowTimes("2026-07-15", nil, showTime, nil, "CA")
	assert.Equal(t, time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC), *summer)
}

func TestResolveShowTimes_UnparseableDateWritesNeither(t *testing.T) {
	doorsAt, musicAt := resolveShowTimes("not-a-date", strPtr("7:00 PM"), strPtr("8:00 PM"), nil, "AZ")
	assert.Nil(t, doorsAt)
	assert.Nil(t, musicAt)
}

func TestResolveShowTimes_AnchorsToTheStatedCalendarDate(t *testing.T) {
	// The date string is the anchor, NOT the computed event_date, which is
	// midnight UTC before a show time is applied and therefore the previous day
	// in every US venue timezone.
	doorsAt, musicAt := resolveShowTimes("2026-06-15", strPtr("7:00 PM"), strPtr("8:00 PM"), nil, "AZ")
	mst := time.FixedZone("MST", -7*3600)
	for name, instant := range map[string]*time.Time{"doors": doorsAt, "music": musicAt} {
		local := instant.In(mst)
		assert.Equal(t, 2026, local.Year(), name)
		assert.Equal(t, time.June, local.Month(), name)
		assert.Equal(t, 15, local.Day(), name)
	}
	assert.Equal(t, 19, doorsAt.In(mst).Hour())
	assert.Equal(t, 20, musicAt.In(mst).Hour())
}

// =============================================================================
// UNIT TESTS — getTimezoneForState
// =============================================================================

// The zone map this package's date parsing depends on, including the date-only
// anchor: a state that silently resolved to the wrong zone would anchor a
// timeless import on the wrong evening.
func TestGetTimezoneForState(t *testing.T) {
	assert.Equal(t, "America/Phoenix", getTimezoneForState("AZ"))
	assert.Equal(t, "America/Phoenix", getTimezoneForState("az"))
	assert.Equal(t, "America/Los_Angeles", getTimezoneForState("CA"))
	assert.Equal(t, "America/Denver", getTimezoneForState("CO"))
	assert.Equal(t, "America/Chicago", getTimezoneForState("TX"))
	assert.Equal(t, "America/New_York", getTimezoneForState("NY"))
	// Unknown state defaults to Phoenix
	assert.Equal(t, "America/Phoenix", getTimezoneForState("XX"))
	assert.Equal(t, "America/Phoenix", getTimezoneForState(""))
}

// =============================================================================
// UNIT TESTS — parseArtistsFromTitle
// =============================================================================

func TestParseArtistsFromTitle_SingleArtist(t *testing.T) {
	result := parseArtistsFromTitle("The National")
	assert.Equal(t, []string{"The National"}, result)
}

func TestParseArtistsFromTitle_CommaSeparated(t *testing.T) {
	result := parseArtistsFromTitle("Artist A, Artist B, Artist C")
	assert.Equal(t, []string{"Artist A", "Artist B", "Artist C"}, result)
}

func TestParseArtistsFromTitle_WithSeparator(t *testing.T) {
	result := parseArtistsFromTitle("Artist A with Artist B")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_WithPlusComma(t *testing.T) {
	// Comma takes priority, so "with" inside a comma segment is preserved
	result := parseArtistsFromTitle("Artist A with Artist B, Artist C")
	assert.Equal(t, []string{"Artist A with Artist B", "Artist C"}, result)
}

func TestParseArtistsFromTitle_SlashSeparator(t *testing.T) {
	result := parseArtistsFromTitle("Artist A / Artist B")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_PipeSeparator(t *testing.T) {
	result := parseArtistsFromTitle("Artist A | Artist B")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_PlusSeparator(t *testing.T) {
	result := parseArtistsFromTitle("Artist A + Artist B")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_AmpersandShortNames(t *testing.T) {
	// Parts <=10 chars: treated as a single artist name
	result := parseArtistsFromTitle("Tom & Jerry")
	assert.Equal(t, []string{"Tom & Jerry"}, result)
}

func TestParseArtistsFromTitle_AmpersandLongNames(t *testing.T) {
	// Parts >10 chars: split into separate artists
	result := parseArtistsFromTitle("The National Band & Radiohead Group")
	assert.Equal(t, []string{"The National Band", "Radiohead Group"}, result)
}

func TestParseArtistsFromTitle_EmptyString(t *testing.T) {
	result := parseArtistsFromTitle("")
	assert.Equal(t, []string{""}, result)
}

func TestParseArtistsFromTitle_WhitespaceTrimmed(t *testing.T) {
	result := parseArtistsFromTitle(" Artist A , Artist B ")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_CommaPriorityOverWith(t *testing.T) {
	// Comma is checked first — "with" inside a comma segment is preserved
	result := parseArtistsFromTitle("A, B with C")
	assert.Equal(t, []string{"A", "B with C"}, result)
}

// =============================================================================
// UNIT TESTS — splitAndTrim
// =============================================================================

func TestSplitAndTrim_Basic(t *testing.T) {
	result := splitAndTrim("a, b, c", ",")
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestSplitAndTrim_FiltersEmpty(t *testing.T) {
	result := splitAndTrim("a,,b", ",")
	assert.Equal(t, []string{"a", "b"}, result)
}

func TestSplitAndTrim_WhitespaceOnlyParts(t *testing.T) {
	result := splitAndTrim("a, , b", ",")
	assert.Equal(t, []string{"a", "b"}, result)
}

func TestSplitAndTrim_NoSeparator(t *testing.T) {
	result := splitAndTrim("abc", ",")
	assert.Equal(t, []string{"abc"}, result)
}

// =============================================================================
// testVenueFinderCreator — lightweight impl of venueFinderCreator for tests
// =============================================================================

// testVenueFinderCreator implements the venueFinderCreator interface using direct
// GORM queries, replicating the core FindOrCreateVenue behavior from VenueService.
type testVenueFinderCreator struct {
	db *gorm.DB
}

func (v *testVenueFinderCreator) FindOrCreateVenue(name, city, state string, address, zipcode *string, txDB *gorm.DB, isAdmin bool) (*catalogm.Venue, bool, error) {
	query := txDB
	if query == nil {
		query = v.db
	}
	if query == nil {
		return nil, false, fmt.Errorf("database not initialized")
	}

	// Check if venue already exists by name and city
	var venue catalogm.Venue
	err := query.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", name, city).First(&venue).Error
	if err == nil {
		// Venue exists — backfill slug if missing
		if venue.Slug == nil {
			baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				query.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			venue.Slug = &slug
			query.Model(&venue).Update("slug", slug)
		}
		return &venue, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Venue doesn't exist, create it
	baseSlug := utils.GenerateVenueSlug(name, city, state)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		query.Model(&catalogm.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	venue = catalogm.Venue{
		Name:  name,
		City:  city,
		State: state,
		Slug:  &slug,
	}
	if address != nil {
		venue.Address = address
	}
	if err := query.Create(&venue).Error; err != nil {
		return nil, false, fmt.Errorf("failed to create venue: %w", err)
	}

	return &venue, true, nil
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type DiscoveryIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *DiscoveryService
}

func (suite *DiscoveryIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	venueSvc := &testVenueFinderCreator{db: suite.testDB.DB}
	suite.svc = NewDiscoveryService(suite.testDB.DB, venueSvc)
}

func (suite *DiscoveryIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *DiscoveryIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestDiscoveryIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(DiscoveryIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) makeEvent(id, title, venueSlug, date string, artists []string) contracts.DiscoveredEvent {
	return contracts.DiscoveredEvent{
		ID:        id,
		Title:     title,
		Date:      date,
		Venue:     "Valley Bar",
		VenueSlug: venueSlug,
		Artists:   artists,
		ScrapedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// =============================================================================
// ImportEvents tests
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_Success() {
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-001", "The National", "valley-bar", "2026-06-15", []string{"The National"}),
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	suite.Equal(1, result.Imported)
	suite.Equal(0, result.Duplicates)
	suite.Equal(0, result.Errors)

	// Verify show was created
	var show catalogm.Show
	err = suite.db.Where("source_event_id = ?", "evt-001").First(&show).Error
	suite.Require().NoError(err)
	suite.Equal("The National", show.Title)
	suite.Equal(catalogm.ShowStatusApproved, show.Status)
	suite.Equal(catalogm.ShowSourceDiscovery, show.Source)
	suite.NotNil(show.Slug)
}

// =============================================================================
// ImportEvents — doors / music times (PSY-1699)
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_WritesStatedDoorAndMusicTimes() {
	event := suite.makeEvent("evt-times-001", "Sunny Day Real Estate", "valley-bar", "2026-06-15", []string{"Sunny Day Real Estate"})
	event.DoorsTime = strPtr("6:30 pm")
	event.ShowTime = strPtr("7:00 pm")

	result, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-times-001").First(&show).Error)

	// Valley Bar is Phoenix, UTC-7 year round.
	suite.Require().NotNil(show.DoorsAt)
	suite.Require().NotNil(show.MusicAt)
	suite.Equal(time.Date(2026, 6, 16, 1, 30, 0, 0, time.UTC), show.DoorsAt.UTC())
	suite.Equal(time.Date(2026, 6, 16, 2, 0, 0, 0, time.UTC), show.MusicAt.UTC())

	// music_at and event_date both derive from the stated show time.
	suite.Equal(show.EventDate.UTC(), show.MusicAt.UTC())

	// The times reached their columns, so the description must not repeat them.
	if show.Description != nil {
		suite.NotContains(*show.Description, "Doors:")
		suite.NotContains(*show.Description, "Show:")
	}
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_LeavesTimesAbsentWhenSourceIsSilent() {
	event := suite.makeEvent("evt-times-002", "Duster", "valley-bar", "2026-06-16", []string{"Duster"})

	result, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-times-002").First(&show).Error)
	suite.Nil(show.DoorsAt)
	suite.Nil(show.MusicAt)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_UnreadableTimeStaysInTheDescription() {
	event := suite.makeEvent("evt-times-003", "Hovvdy", "valley-bar", "2026-06-17", []string{"Hovvdy"})
	event.DoorsTime = strPtr("doors when we open")
	event.ShowTime = strPtr("8:00 pm")

	result, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-times-003").First(&show).Error)

	// The unreadable doors text is the only surviving record of it, so it keeps
	// its place in the description; the readable show time does not.
	suite.Nil(show.DoorsAt)
	suite.Require().NotNil(show.MusicAt)
	suite.Require().NotNil(show.Description)
	suite.Contains(*show.Description, "Doors: doors when we open")
	suite.NotContains(*show.Description, "Show:")
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_DoorsOnlyListingStaysDateOnly() {
	// event_date comes off the show time. A listing stating only doors leaves it
	// a midnight-UTC placeholder, so writing doors_at would put the stripe a
	// calendar day away from the date the page renders.
	event := suite.makeEvent("evt-times-004", "Duster", "valley-bar", "2026-06-18", []string{"Duster"})
	event.DoorsTime = strPtr("7:00 PM")

	result, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-times-004").First(&show).Error)
	suite.Nil(show.DoorsAt)
	suite.Nil(show.MusicAt)
	// The doors text is still the only record of it, so it stays readable.
	suite.Require().NotNil(show.Description)
	suite.Contains(*show.Description, "Doors: 7:00 PM")
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_UpdateLeavesTimeColumnsAlone() {
	// Re-scrape updates are the backfill case this ticket deferred: this path
	// never moves event_date, so writing a time here would leave the stripe and
	// the date disagreeing, and a scrape stating only one of the two cannot see
	// the stored other half to keep doors <= music.
	event := suite.makeEvent("evt-times-005", "Wednesday", "valley-bar", "2026-06-19", []string{"Wednesday"})

	result, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	event.DoorsTime = strPtr("7:00 PM")
	event.ShowTime = strPtr("8:00 PM")
	_, err = suite.svc.ImportEvents([]contracts.DiscoveredEvent{event}, false, true, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("source_event_id = ?", "evt-times-005").First(&show).Error)
	suite.Nil(show.DoorsAt)
	suite.Nil(show.MusicAt)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_SourceDuplicate() {
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-002", "Radiohead", "valley-bar", "2026-07-01", []string{"Radiohead"}),
	}

	// First import
	result1, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result1.Imported)

	// Second import — same source_venue + source_event_id
	result2, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(0, result2.Imported)
	suite.Equal(1, result2.Duplicates)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_UnknownVenue() {
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-003", "Test Band", "unknown-venue-xyz", "2026-08-01", []string{"Test Band"}),
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Errors)
	suite.Contains(result.Messages[0], "Unknown venue slug")
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_HeadlinerDuplicate() {
	// Import one show first
	events1 := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-004a", "Bon Iver", "valley-bar", "2026-09-01", []string{"Bon Iver"}),
	}
	result1, err := suite.svc.ImportEvents(events1, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result1.Imported)

	// Import another event with the same headliner at the same venue on the same date
	// but different source_event_id — should be blocked as duplicate
	events2 := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-004b", "Bon Iver (Late Show)", "valley-bar", "2026-09-01", []string{"Bon Iver"}),
	}
	result2, err := suite.svc.ImportEvents(events2, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result2.Duplicates)

	// Verify the second show was NOT created
	var count int64
	suite.db.Model(&catalogm.Show{}).Where("source_event_id = ?", "evt-004b").Count(&count)
	suite.Equal(int64(0), count)
}

// TestImportEvents_DuplicateAgainstCuratedOpenerClaimsNoRole covers the bill the
// guard's position arm reaches (PSY-1944): the stored show curated this artist as
// its OPENER at position 0, so the event is still classified a duplicate and the
// message asserts no role for either side.
//
// Classifying it is the correct outcome, not a tolerated false positive: the same
// artist at the same venue at the same instant is the same event, and letting it
// through only trades this DUPLICATE tally for a raw unique-violation ERROR.
func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_DuplicateAgainstCuratedOpenerClaimsNoRole() {
	curated := contracts.DiscoveredEvent{
		ID:        "evt-1944a",
		Title:     "Curated Bill",
		Date:      "2026-10-04",
		Venue:     "Valley Bar",
		VenueSlug: "valley-bar",
		BillingArtists: []contracts.DiscoveredArtist{
			{Name: "Opener Act", SetType: "opener", BillingOrder: 1},
			{Name: "Headline Act", SetType: "headliner", BillingOrder: 2},
		},
		ScrapedAt: time.Now().UTC().Format(time.RFC3339),
	}
	result1, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{curated}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Require().Equal(1, result1.Imported)

	// The curated opener really did land at position 0 with a non-inferred role,
	// so this test exercises the position arm rather than the set_type arm.
	var stored catalogm.ShowArtist
	suite.Require().NoError(suite.db.Where("position = 0").First(&stored).Error)
	suite.Equal("opener", stored.SetType)

	rebilled := contracts.DiscoveredEvent{
		ID:        "evt-1944b",
		Title:     "Rebilled Show",
		Date:      "2026-10-04",
		Venue:     "Valley Bar",
		VenueSlug: "valley-bar",
		BillingArtists: []contracts.DiscoveredArtist{
			{Name: "Opener Act", SetType: "headliner", BillingOrder: 1},
		},
		ScrapedAt: time.Now().UTC().Format(time.RFC3339),
	}
	result2, err := suite.svc.ImportEvents([]contracts.DiscoveredEvent{rebilled}, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result2.Duplicates, "the position arm still catches this bill")
	suite.Equal(0, result2.Errors, "a clean duplicate tally, not a unique-violation error")
	suite.Require().Len(result2.Messages, 1)
	suite.Contains(result2.Messages[0], "DUPLICATE:")
	suite.NotContains(result2.Messages[0], "headliner",
		"the matched row is a curated opener, so the copy must claim no role")

	var count int64
	suite.db.Model(&catalogm.Show{}).Where("source_event_id = ?", "evt-1944b").Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_RejectedShowSkipped() {
	// Create a rejected show at Valley Bar on a specific date
	venue := &catalogm.Venue{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)

	// 20:00 Phoenix on 2026-10-01, the instant a date-only import of that date
	// now produces — the rejected-show check matches on the full timestamp.
	show := &catalogm.Show{
		Title:     "Old Rejected Show",
		EventDate: time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC),
		Status:    catalogm.ShowStatusRejected,
		Source:    catalogm.ShowSourceUser,
	}
	err = suite.db.Create(show).Error
	suite.Require().NoError(err)

	showVenue := catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}
	err = suite.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", showVenue.ShowID, showVenue.VenueID).Error
	suite.Require().NoError(err)

	// Try to import an event at the same venue and date
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-005", "Some New Band", "valley-bar", "2026-10-01", []string{"Some New Band"}),
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Rejected)
	suite.Contains(result.Messages[0], "REJECTED")
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_DryRun() {
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-006", "Dry Run Band", "valley-bar", "2026-11-01", []string{"Dry Run Band"}),
	}

	result, err := suite.svc.ImportEvents(events, true, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	// In dry run, nothing is actually imported but message says "WOULD IMPORT"
	suite.Contains(result.Messages[0], "WOULD IMPORT")

	// Verify nothing was actually created
	var count int64
	suite.db.Model(&catalogm.Show{}).Where("source_event_id = ?", "evt-006").Count(&count)
	suite.Zero(count)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_PendingStatus() {
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-pending-1", "Pending Band", "valley-bar", "2026-11-15", []string{"Pending Band"}),
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusPending)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	// Verify show was created with pending status
	var show catalogm.Show
	err = suite.db.Where("source_event_id = ?", "evt-pending-1").First(&show).Error
	suite.Require().NoError(err)
	suite.Equal(catalogm.ShowStatusPending, show.Status)
}

// =============================================================================
// CheckEvents tests
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_Found() {
	// Import an event first
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-check-1", "Check Band", "valley-bar", "2026-12-01", []string{"Check Band"}),
	}
	_, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	// Check it
	checkInputs := []contracts.CheckEventInput{
		{ID: "evt-check-1", VenueSlug: "valley-bar"},
	}
	result, err := suite.svc.CheckEvents(checkInputs)
	suite.Require().NoError(err)

	status, ok := result.Events["evt-check-1"]
	suite.True(ok, "event should be found")
	suite.True(status.Exists)
	suite.Equal("approved", status.Status)
	suite.NotZero(status.ShowID)
}

func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_NotFound() {
	checkInputs := []contracts.CheckEventInput{
		{ID: "evt-nonexistent", VenueSlug: "valley-bar"},
	}
	result, err := suite.svc.CheckEvents(checkInputs)
	suite.Require().NoError(err)
	suite.Empty(result.Events)
}

func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_EmptyInput() {
	result, err := suite.svc.CheckEvents([]contracts.CheckEventInput{})
	suite.Require().NoError(err)
	suite.Empty(result.Events)
}

// =============================================================================
// BillingArtists import tests (PSY-30)
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_WithBillingArtists() {
	events := []contracts.DiscoveredEvent{
		{
			ID:        "evt-billing-1",
			Title:     "Main Act with Support",
			Date:      "2026-12-15",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Main Act", "Support Band", "Opener Band"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Main Act", SetType: "headliner", BillingOrder: 1},
				{Name: "Support Band", SetType: "support", BillingOrder: 2},
				{Name: "Opener Band", SetType: "opener", BillingOrder: 3},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	// Verify show was created
	var show catalogm.Show
	err = suite.db.Where("source_event_id = ?", "evt-billing-1").First(&show).Error
	suite.Require().NoError(err)

	// Verify show_artists have correct set_type and position
	var showArtists []catalogm.ShowArtist
	err = suite.db.Where("show_id = ?", show.ID).Order("position").Find(&showArtists).Error
	suite.Require().NoError(err)
	suite.Require().Len(showArtists, 3)

	// Main Act: headliner, position 0 (billing_order 1 → position 0)
	suite.Equal("headliner", showArtists[0].SetType)
	suite.Equal(0, showArtists[0].Position)

	// Support Band: normalized "support" → "direct_support", position 1 (billing_order 2 → position 1)
	suite.Equal("direct_support", showArtists[1].SetType)
	suite.Equal(1, showArtists[1].Position)

	// Opener Band: "opener", position 2 (billing_order 3 → position 2)
	suite.Equal("opener", showArtists[2].SetType)
	suite.Equal(2, showArtists[2].Position)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_FallbackWithoutBillingArtists() {
	// When BillingArtists is empty, the fallback infers ONLY the headliner
	// from bill order (PSY-1673). Positions past 0 get the neutral default:
	// list order is evidence of billing order, not of an opening slot.
	events := []contracts.DiscoveredEvent{
		{
			ID:        "evt-no-billing-1",
			Title:     "Legacy Import",
			Date:      "2026-12-20",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"First Band", "Second Band"},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	// Verify show_artists have default set_type
	var show catalogm.Show
	err = suite.db.Where("source_event_id = ?", "evt-no-billing-1").First(&show).Error
	suite.Require().NoError(err)

	var showArtists []catalogm.ShowArtist
	err = suite.db.Where("show_id = ?", show.ID).Order("position").Find(&showArtists).Error
	suite.Require().NoError(err)
	suite.Require().Len(showArtists, 2)

	suite.Equal("headliner", showArtists[0].SetType)
	suite.Equal(0, showArtists[0].Position)
	suite.Equal("performer", showArtists[1].SetType)
	suite.Equal(1, showArtists[1].Position)
}

// A slot the vocabulary cannot model is still a STATEMENT about the act, so it
// must not be overwritten by the position-0 headliner inference. Before this
// guard, "host" normalized to empty and the first-billed act was promoted to
// headliner -- a stronger false assertion than the "opener" default this
// ticket exists to remove.
func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_UnmappableSetTypeAtPositionZeroIsNotPromoted() {
	events := []contracts.DiscoveredEvent{
		{
			ID:        "evt-host-first-1",
			Title:     "Hosted Night",
			Date:      "2026-12-28",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"The Host", "Actual Headliner"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "The Host", SetType: "host", BillingOrder: 1},
				{Name: "Actual Headliner", SetType: "headliner", BillingOrder: 2},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show catalogm.Show
	err = suite.db.Where("source_event_id = ?", "evt-host-first-1").First(&show).Error
	suite.Require().NoError(err)

	var showArtists []catalogm.ShowArtist
	err = suite.db.Where("show_id = ?", show.ID).Order("position").Find(&showArtists).Error
	suite.Require().NoError(err)
	suite.Require().Len(showArtists, 2)

	// The stated-but-unmappable host defaults; it is NOT crowned.
	suite.Equal("performer", showArtists[0].SetType)
	suite.Equal("headliner", showArtists[1].SetType)
}

// The inference still fires when the source stated NOTHING at all -- that is
// the ordinary billing convention and the only slot this path may infer.
func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_SilentSourceStillInfersHeadlinerAtPositionZero() {
	events := []contracts.DiscoveredEvent{
		{
			ID:        "evt-silent-first-1",
			Title:     "Silent Billing",
			Date:      "2026-12-29",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Top Of Bill", "Also Playing"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Top Of Bill", BillingOrder: 1},
				{Name: "Also Playing", BillingOrder: 2},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show catalogm.Show
	err = suite.db.Where("source_event_id = ?", "evt-silent-first-1").First(&show).Error
	suite.Require().NoError(err)

	var showArtists []catalogm.ShowArtist
	err = suite.db.Where("show_id = ?", show.ID).Order("position").Find(&showArtists).Error
	suite.Require().NoError(err)
	suite.Require().Len(showArtists, 2)

	suite.Equal("headliner", showArtists[0].SetType)
	suite.Equal("performer", showArtists[1].SetType)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_WithSpecialGuestAndDJ() {
	events := []contracts.DiscoveredEvent{
		{
			ID:        "evt-special-1",
			Title:     "Complex Bill",
			Date:      "2026-12-25",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Main Act", "Guest Artist", "DJ Spinz"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Main Act", SetType: "headliner", BillingOrder: 1},
				{Name: "Guest Artist", SetType: "special_guest", BillingOrder: 2},
				{Name: "DJ Spinz", SetType: "dj", BillingOrder: 3},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show catalogm.Show
	err = suite.db.Where("source_event_id = ?", "evt-special-1").First(&show).Error
	suite.Require().NoError(err)

	var showArtists []catalogm.ShowArtist
	err = suite.db.Where("show_id = ?", show.ID).Order("position").Find(&showArtists).Error
	suite.Require().NoError(err)
	suite.Require().Len(showArtists, 3)

	suite.Equal("headliner", showArtists[0].SetType)
	suite.Equal("special_guest", showArtists[1].SetType)
	suite.Equal("dj", showArtists[2].SetType) // dj is a first-class slot
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_HeadlinerDuplicate_WithBillingArtists() {
	// Import a show with billing data
	events1 := []contracts.DiscoveredEvent{
		{
			ID:        "evt-billing-001",
			Title:     "Big Show Night",
			Date:      "2026-11-01",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Star Band", "Opener"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Star Band", SetType: "headliner", BillingOrder: 1},
				{Name: "Opener", SetType: "opener", BillingOrder: 2},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	result1, err := suite.svc.ImportEvents(events1, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result1.Imported)

	// Import another event with the same headliner via billing data but different source_event_id
	events2 := []contracts.DiscoveredEvent{
		{
			ID:        "evt-billing-002",
			Title:     "Big Show Night (Late)",
			Date:      "2026-11-01",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Star Band"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Star Band", SetType: "headliner", BillingOrder: 1},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	result2, err := suite.svc.ImportEvents(events2, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result2.Duplicates)

	// Verify the second show was NOT created
	var count int64
	suite.db.Model(&catalogm.Show{}).Where("source_event_id = ?", "evt-billing-002").Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_HeadlinerDuplicate_Position0Match() {
	// Import a show where the headliner is assigned position=0 but set_type is just "performer"
	// This simulates shows created without explicit headliner tagging
	venue := &catalogm.Venue{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	suite.Require().NoError(suite.db.Create(venue).Error)

	artist := &catalogm.Artist{Name: "Position Zero Band"}
	slug := utils.GenerateArtistSlug(artist.Name)
	artist.Slug = &slug
	suite.Require().NoError(suite.db.Create(artist).Error)

	// Store the existing show at the exact event_date a date-only import
	// produces: parseEventDate anchors a bare date at 20:00 in the venue's zone,
	// so 2026-11-15 at a Phoenix (UTC-7) venue is 2026-11-16T03:00Z. The dedup
	// key is the FULL timestamp (PSY-559), so the fixture must share the
	// import's exact event_date to be a true duplicate — a different time-of-day
	// would be a distinct (matinee/evening) show.
	show := &catalogm.Show{
		Title:     "Existing Show",
		EventDate: time.Date(2026, 11, 16, 3, 0, 0, 0, time.UTC),
		Status:    catalogm.ShowStatusApproved,
		Source:    catalogm.ShowSourceUser,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID).Error)
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'performer')",
		show.ID, artist.ID).Error)

	// Now try to import an event with the same artist at the same venue at the
	// same exact event_date (date-only import → 20:00 venue-local, matching the
	// fixture above)
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-pos0-001", "Position Zero Band Live", "valley-bar", "2026-11-15", []string{"Position Zero Band"}),
	}
	result, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Duplicates)
	suite.Contains(result.Messages[0], "DUPLICATE")
}

// =============================================================================
// UNIT TESTS — resolveHeadlinerName
// =============================================================================

func TestResolveHeadlinerName_BillingArtists_ExplicitHeadliner(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{
		BillingArtists: []contracts.DiscoveredArtist{
			{Name: "Opener", SetType: "opener", BillingOrder: 2},
			{Name: "Headliner Band", SetType: "headliner", BillingOrder: 1},
		},
		Artists: []string{"Opener", "Headliner Band"},
	}
	assert.Equal(t, "Headliner Band", svc.resolveHeadlinerName(event))
}

func TestResolveHeadlinerName_BillingArtists_ByBillingOrder(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{
		BillingArtists: []contracts.DiscoveredArtist{
			{Name: "Second Act", SetType: "performer", BillingOrder: 2},
			{Name: "First Act", SetType: "performer", BillingOrder: 1},
		},
	}
	// Should return the one with lowest billing order
	assert.Equal(t, "First Act", svc.resolveHeadlinerName(event))
}

func TestResolveHeadlinerName_BillingArtists_NoHeadlinerNoOrder(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{
		BillingArtists: []contracts.DiscoveredArtist{
			{Name: "First Entry", SetType: "performer"},
			{Name: "Second Entry", SetType: "performer"},
		},
	}
	// Should return first entry when no explicit headliner or billing order
	assert.Equal(t, "First Entry", svc.resolveHeadlinerName(event))
}

func TestResolveHeadlinerName_FallbackToArtistsList(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{
		Artists: []string{"Main Band", "Support Band"},
	}
	assert.Equal(t, "Main Band", svc.resolveHeadlinerName(event))
}

func TestResolveHeadlinerName_NoArtists(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{}
	assert.Equal(t, "", svc.resolveHeadlinerName(event))
}

// =============================================================================
// CheckEvents artist-fetch batching (PSY-757)
// =============================================================================

// artistQueryCounter counts queries that touch show_artists on the suite DB.
// CheckEvents fetches artist names via db.Raw(...).Scan(...). GORM's Scan path
// runs through Rows(), which fires the "gorm:row" callback (NOT the query/raw
// callbacks), so the counter hooks Row(). This lets the regression tests below
// assert the artist fetch is O(1) — exactly one query — regardless of how many
// events are checked.
var artistQueryCounter atomic.Int64

// registerArtistQueryCounterOnce attaches the counter callback to the shared
// suite DB exactly once (GORM rejects duplicate callback names).
var registerArtistQueryCounterOnce sync.Once

func registerArtistQueryCounter(db *gorm.DB) {
	registerArtistQueryCounterOnce.Do(func() {
		_ = db.Callback().Row().After("gorm:row").Register("test:count_artist_query", func(tx *gorm.DB) {
			if tx.Statement != nil && strings.Contains(tx.Statement.SQL.String(), "show_artists") {
				artistQueryCounter.Add(1)
			}
		})
	})
}

// TestCheckEvents_BatchesArtistFetch seeds N shows (each with M artists) that
// all match by source key, then asserts CheckEvents issues exactly ONE Raw
// query for artist names regardless of N — and that every event still gets its
// correct, position-ordered artist list. Before PSY-757 this was one Raw query
// per show (N), an N+1 on the discovery hot path.
func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_BatchesArtistFetch() {
	const numShows = 50

	events := make([]contracts.DiscoveredEvent, 0, numShows)
	expectedArtists := make(map[string][]string, numShows)
	for i := 0; i < numShows; i++ {
		eventID := fmt.Sprintf("evt-batch-%d", i)
		artists := []string{
			fmt.Sprintf("Headliner %d", i),
			fmt.Sprintf("Support %d", i),
			fmt.Sprintf("Opener %d", i),
		}
		events = append(events, suite.makeEvent(
			eventID,
			fmt.Sprintf("Batch Show %d", i),
			"valley-bar",
			fmt.Sprintf("2026-12-%02d", (i%28)+1),
			artists,
		))
		expectedArtists[eventID] = artists
	}

	_, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	checkInputs := make([]contracts.CheckEventInput, 0, numShows)
	for _, e := range events {
		checkInputs = append(checkInputs, contracts.CheckEventInput{ID: e.ID, VenueSlug: "valley-bar"})
	}

	registerArtistQueryCounter(suite.db)
	artistQueryCounter.Store(0)

	result, err := suite.svc.CheckEvents(checkInputs)
	suite.Require().NoError(err)

	// O(1) artist queries: exactly one fetch for all 50 shows' artists.
	suite.Equal(int64(1), artistQueryCounter.Load(),
		"CheckEvents must batch artist fetches into a single query regardless of event count")

	// Correctness: every event resolves to its own position-ordered artist list.
	suite.Require().Len(result.Events, numShows)
	for eventID, want := range expectedArtists {
		status, ok := result.Events[eventID]
		suite.Require().True(ok, "event %s should be found", eventID)
		suite.Require().NotNil(status.CurrentData)
		suite.Equal(want, status.CurrentData.Artists, "artist names/order for %s", eventID)
	}
}

// TestCheckEvents_BatchesArtistFetch_FallbackPath verifies the venue+date
// fallback path (manually-created shows with no source key) also uses the
// single batched artist fetch — one Raw query for all fallback matches.
func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_BatchesArtistFetch_FallbackPath() {
	const numShows = 10

	// Seed shows via the normal import (gives them a source key + artists),
	// then strip source_venue/source_event_id so CheckEvents can only match
	// them through the venue+date fallback branch.
	events := make([]contracts.DiscoveredEvent, 0, numShows)
	expectedArtists := make(map[string][]string, numShows)
	for i := 0; i < numShows; i++ {
		eventID := fmt.Sprintf("evt-fallback-%d", i)
		artists := []string{fmt.Sprintf("FB Headliner %d", i), fmt.Sprintf("FB Opener %d", i)}
		date := fmt.Sprintf("2026-11-%02d", i+1)
		events = append(events, suite.makeEvent(eventID, fmt.Sprintf("FB Show %d", i), "valley-bar", date, artists))
		expectedArtists[date] = artists
	}

	_, err := suite.svc.ImportEvents(events, false, false, catalogm.ShowStatusApproved)
	suite.Require().NoError(err)

	// Remove the source key so only the date fallback can match.
	suite.Require().NoError(
		suite.db.Model(&catalogm.Show{}).
			Where("source_event_id LIKE ?", "evt-fallback-%").
			Updates(map[string]interface{}{"source_venue": nil, "source_event_id": nil}).Error,
	)

	checkInputs := make([]contracts.CheckEventInput, 0, numShows)
	for date := range expectedArtists {
		// Date-only input with no ID match forces the venue+date fallback.
		checkInputs = append(checkInputs, contracts.CheckEventInput{ID: "no-source-" + date, VenueSlug: "valley-bar", Date: date})
	}

	registerArtistQueryCounter(suite.db)
	artistQueryCounter.Store(0)

	result, err := suite.svc.CheckEvents(checkInputs)
	suite.Require().NoError(err)

	// One query for all fallback-matched shows' artists.
	suite.Equal(int64(1), artistQueryCounter.Load(),
		"fallback path must batch artist fetches into a single query")

	suite.Require().Len(result.Events, numShows)
	for date, want := range expectedArtists {
		status, ok := result.Events["no-source-"+date]
		suite.Require().True(ok, "fallback event for %s should be found", date)
		suite.Require().NotNil(status.CurrentData)
		suite.Equal(want, status.CurrentData.Artists, "artist names/order for %s", date)
	}
}

// The venue+date fallback window is the VENUE'S calendar day, not a UTC one.
//
// A Phoenix venue's evening sits on the following UTC day, so a UTC
// midnight-to-midnight window reported the previous night's show under the date
// asked for and missed the date's own show entirely — an off-by-one-night answer
// in the scraper UI's "already have this?" column.
func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_FallbackWindowIsVenueLocalDay() {
	venue := &catalogm.Venue{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	suite.Require().NoError(suite.db.Create(venue).Error)

	// 20:00 Phoenix on 2026-12-04 — stored as 2026-12-05T03:00Z, i.e. the UTC
	// day AFTER its own calendar date. This is where every US evening show sits.
	show := &catalogm.Show{
		Title:     "Venue Local Night",
		EventDate: time.Date(2026, 12, 5, 3, 0, 0, 0, time.UTC),
		Status:    catalogm.ShowStatusApproved,
		Source:    catalogm.ShowSourceUser,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID).Error)

	result, err := suite.svc.CheckEvents([]contracts.CheckEventInput{
		{ID: "local-day-hit", VenueSlug: "valley-bar", Date: "2026-12-04"},
		{ID: "local-day-miss", VenueSlug: "valley-bar", Date: "2026-12-05"},
	})
	suite.Require().NoError(err)

	hit, ok := result.Events["local-day-hit"]
	suite.Require().True(ok, "the show must be found under its own venue-local date")
	suite.True(hit.Exists, "2026-12-04 is the night this show happens on")

	// And it must NOT also answer for the following date, which is the failure
	// the old UTC window produced: one show reported under two different nights.
	if miss, ok := result.Events["local-day-miss"]; ok {
		suite.False(miss.Exists, "2026-12-05 is a different night and has no show")
	}
}
