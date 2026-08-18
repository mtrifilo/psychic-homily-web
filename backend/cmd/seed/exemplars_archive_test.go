package main

import (
	"testing"
	"time"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// The archive exemplar generates its shows rather than listing them, so the
// constraints it has to satisfy are properties of the generator, not of any
// one row. These tests pin those properties. Each failure corresponds to a
// specific runtime failure of the seed:
//
//   - repeated day in a month  -> two shows share an event_date at one
//     venue, so a shared act trips the show_artists partial unique index
//   - repeated act on a bill   -> duplicate show_artists primary key
//   - a non-past archived year -> a show meant for the archive renders
//     under Upcoming instead
//   - thresholds not met       -> the fixture still seeds, but no longer
//     shows the UI it exists to make reviewable

// testArchiveRoster builds a stand-in roster the size of the real one. The
// SIZE is what the bill-distinctness argument depends on (the stride is
// coprime with it), so the tests must use the real length.
func testArchiveRoster() []archiveAct {
	roster := make([]archiveAct, len(archiveBillArtists))
	for i, name := range archiveBillArtists {
		roster[i] = archiveAct{ID: uint(i + 1), Name: name}
	}
	return roster
}

func TestArchiveDayOfMonthIsDistinctAndInRange(t *testing.T) {
	// Every count the fixture can ask for, up to the 28-day cap the spread
	// relies on.
	for count := 1; count <= 28; count++ {
		seen := map[int]bool{}
		for i := 0; i < count; i++ {
			day := archiveDayOfMonth(i, count)
			if day < 1 || day > 28 {
				t.Fatalf("count=%d i=%d: day %d out of the 1-28 range (29-31 would be invalid in February)", count, i, day)
			}
			if seen[day] {
				t.Fatalf("count=%d: day %d generated twice; two shows would land on the same date at one venue", count, day)
			}
			seen[day] = true
		}
	}
}

func TestArchiveFixtureMeetsItsReviewThresholds(t *testing.T) {
	// This fixture exists to make the archive UI reviewable, and that is
	// purely a function of these counts. Without this test, trimming
	// archiveYears to speed the seed up would silently destroy the reason
	// the fixture is in the tree while every other test still passed.
	const (
		pageSize          = 50 // frontend VENUE_PAST_SHOWS_PAGE_LIMIT
		fullStripMaxPages = 7  // frontend Pagination.FULL_STRIP_MAX_PAGES
	)

	if len(archiveYears) < 3 {
		t.Fatalf("archive spans %d years; want >= 3 so the year strip is meaningfully multi-year", len(archiveYears))
	}

	var grandTotal int
	for _, ay := range archiveYears {
		var yearTotal, emptyMonths int
		for i, count := range ay.months {
			if count < 0 {
				t.Errorf("%d month %d has a negative count %d", ay.year, i+1, count)
			}
			if count > 28 {
				t.Errorf("%d month %d has %d shows; archiveDayOfMonth can only spread 28 distinct days", ay.year, i+1, count)
			}
			if count == 0 {
				emptyMonths++
			}
			yearTotal += count
		}
		grandTotal += yearTotal

		if yearTotal <= pageSize {
			t.Errorf("%d has %d shows; want > %d so that year paginates on its own", ay.year, yearTotal, pageSize)
		}
		if emptyMonths == 0 {
			t.Errorf("%d has no empty month; the month-range page labels then never have to skip a gap", ay.year)
		}
	}

	// Above the full-strip limit the pager renders first/last/current+-1 with
	// ellipses; at or below it renders every page number, and the truncation
	// branch goes unreviewed.
	if pages := (grandTotal + pageSize - 1) / pageSize; pages <= fullStripMaxPages {
		t.Errorf("archive totals %d shows = %d pages; want > %d pages so the pager's ellipsis branch renders", grandTotal, pages, fullStripMaxPages)
	}
}

func TestArchiveYearsAreAllInThePastAndOrdered(t *testing.T) {
	// The archive's dates are hardcoded rather than derived from now(), so a
	// year that has not fully elapsed would put some of its shows in the
	// Upcoming section.
	currentYear := time.Now().In(archiveVenueZone).Year()
	for _, ay := range archiveYears {
		if ay.year >= currentYear {
			t.Errorf("archived year %d is not fully past (current year %d); its December shows would render as upcoming", ay.year, currentYear)
		}
	}

	// Generation order must be stable, or every show's variation and slug
	// changes between runs.
	for i := 1; i < len(archiveYears); i++ {
		if archiveYears[i-1].year >= archiveYears[i].year {
			t.Fatalf("archiveYears is not ascending at index %d (%d then %d)", i, archiveYears[i-1].year, archiveYears[i].year)
		}
	}
}

func TestArchiveBillIsWellFormed(t *testing.T) {
	roster := testArchiveRoster()

	for seq := 1; seq <= 5000; seq++ {
		bill := archiveBill(roster, seq)
		if len(bill) == 0 {
			t.Fatalf("seq=%d produced an empty bill; every show needs a headliner", seq)
		}

		seen := map[uint]bool{}
		var headliners int
		for pos, sa := range bill {
			if seen[sa.ArtistID] {
				t.Fatalf("seq=%d: act %d billed twice; this is a show_artists primary-key violation", seq, sa.ArtistID)
			}
			seen[sa.ArtistID] = true

			if sa.Position != pos {
				t.Fatalf("seq=%d: position %d out of order at index %d", seq, sa.Position, pos)
			}
			if sa.SetType == contracts.SetTypeHeadliner {
				headliners++
			}
		}
		if headliners != 1 {
			t.Fatalf("seq=%d: %d headliners, want exactly 1", seq, headliners)
		}
	}
}

func TestArchiveHeadlinerMatchesBillLeader(t *testing.T) {
	// The show's title and slug both name the headliner while the bill's
	// first row carries the headliner's ID. If these ever disagreed, a show
	// would be titled after an act that is not on it — and because the slug
	// is the idempotency key, the mismatch would be baked in permanently.
	roster := testArchiveRoster()

	for seq := 1; seq <= 5000; seq++ {
		headliner := archiveHeadliner(roster, seq)
		leader := archiveBill(roster, seq)[0]
		if headliner.ID != leader.ArtistID {
			t.Fatalf("seq=%d: headliner %q has ID %d but the bill leads with act %d", seq, headliner.Name, headliner.ID, leader.ArtistID)
		}
	}
}

func TestArchiveBillProducesWideBills(t *testing.T) {
	// The long roster names only earn their keep if some bill is wide enough
	// to wrap. Guards against a tweak to the size distribution quietly
	// flattening every bill to two acts.
	roster := testArchiveRoster()

	var widest int
	for seq := 1; seq <= 360; seq++ {
		if n := len(archiveBill(roster, seq)); n > widest {
			widest = n
		}
	}
	if widest < 6 {
		t.Errorf("widest bill over the archive's range is %d acts; want >= 6 so bill wrapping is exercised", widest)
	}
}

func TestArchiveNoiseIsNonNegative(t *testing.T) {
	// Callers use the result with %, which would index negatively.
	for n := 0; n < 10000; n++ {
		if got := archiveNoise(n); got < 0 {
			t.Fatalf("archiveNoise(%d) = %d, want non-negative", n, got)
		}
	}
}

func TestArchiveNoiseMatchesGoldenValues(t *testing.T) {
	// archiveNoise decides each show's bill, and the bill's headliner is
	// baked into the show SLUG — the seed's idempotency key. If this hash
	// changes, a re-seed stops recognising the archive it already wrote and
	// creates a second copy of all 360 shows. Changing these numbers is a
	// breaking change to existing seeded databases, not a refactor.
	golden := map[int]int{
		0:      34894294,
		1:      1522722351,
		42:     106555328,
		360:    1436287145,
		100000: 1368675157,
	}
	for n, want := range golden {
		if got := archiveNoise(n); got != want {
			t.Errorf("archiveNoise(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestArchiveVenueZoneMatchesTheVenueRow(t *testing.T) {
	// The fixture builds event times in archiveVenueZone while the archive's
	// histograms bucket by the zone resolved from the VENUE ROW. Asserting
	// the two agree is the point — a test that round-tripped archiveVenueZone
	// against itself would pass even if the venue carried a different zone.
	fromVenueRow := utils.EventLocation(strptr(exemplarArchiveVenueTimezone), exemplarArchiveVenueState)
	if archiveVenueZone.String() != fromVenueRow.String() {
		t.Fatalf("archiveVenueZone is %s but the venue row resolves to %s", archiveVenueZone, fromVenueRow)
	}
}

func TestArchiveEveningTimesKeepTheirCalendarSlot(t *testing.T) {
	// Shows are authored as venue-local evening times and stored in UTC,
	// while the year/month histograms bucket by venue-local date. A 20:00
	// Phoenix show is 03:00 UTC the NEXT day, so a zone disagreement would
	// leak December shows into January. This asserts the round-trip holds at
	// the year boundary, where it is most fragile.
	for _, clock := range archiveDoorTimes {
		local := time.Date(2025, time.December, 31, clock.hour, clock.minute, 0, 0, archiveVenueZone)
		back := local.UTC().In(archiveVenueZone)

		if back.Year() != 2025 || back.Month() != time.December || back.Day() != 31 {
			t.Errorf("%02d:%02d round-tripped to %s, want 2025-12-31", clock.hour, clock.minute, back.Format("2006-01-02"))
		}
		if clock.hour < 12 {
			t.Errorf("%02d:%02d is not an evening time", clock.hour, clock.minute)
		}
	}
}
