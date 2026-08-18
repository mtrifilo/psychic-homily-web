package main

import (
	"testing"
	"time"
)

// The archive exemplar generates its shows rather than listing them, so the
// database constraints it has to satisfy are properties of the generator,
// not of any one row. These tests pin those properties. Each failure here
// corresponds to a specific runtime failure of the seed:
//
//   - repeated day in a month  -> two shows share an event_date at one
//     venue, so a shared act trips the show_artists partial unique index
//   - repeated act on a bill   -> duplicate show_artists primary key
//   - a non-past archived year -> a show meant for the archive shows up
//     under Upcoming instead

func TestArchiveDayOfMonthIsDistinctAndInRange(t *testing.T) {
	// Every count the fixture can actually ask for, plus the full range up
	// to the 28-day cap the spread relies on.
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

func TestArchiveShowsPerMonthStaysWithinDaySpread(t *testing.T) {
	for year, months := range archiveShowsPerMonth {
		for i, count := range months {
			if count > 28 {
				t.Errorf("%d month %d has %d shows; archiveDayOfMonth can only spread 28 distinct days", year, i+1, count)
			}
			if count < 0 {
				t.Errorf("%d month %d has a negative count %d", year, i+1, count)
			}
		}
	}
}

func TestArchiveYearsAreAllInThePast(t *testing.T) {
	// The archive's whole purpose is a PAST-shows fixture, and its dates are
	// hardcoded rather than derived from now(). A year that has not fully
	// elapsed would put some of its shows in the Upcoming section.
	currentYear := time.Now().In(phoenixOffset).Year()
	for year := range archiveShowsPerMonth {
		if year >= currentYear {
			t.Errorf("archived year %d is not fully past (current year %d); its December shows would render as upcoming", year, currentYear)
		}
	}
}

func TestArchiveYearsAscendingIsSortedAndComplete(t *testing.T) {
	years := archiveYearsAscending()
	if len(years) != len(archiveShowsPerMonth) {
		t.Fatalf("got %d years, want %d", len(years), len(archiveShowsPerMonth))
	}
	for i := 1; i < len(years); i++ {
		if years[i-1] >= years[i] {
			t.Fatalf("years not ascending: %v — an unstable order would give each show a different variation on every run", years)
		}
	}
	for _, year := range years {
		if _, ok := archiveShowsPerMonth[year]; !ok {
			t.Errorf("year %d is not in archiveShowsPerMonth", year)
		}
	}
}

func TestArchiveBillHasDistinctArtists(t *testing.T) {
	// Stand-in artist IDs: the roster size is what the distinctness argument
	// (stride coprime with the roster) actually depends on.
	artistIDs := make([]uint, len(archiveBillArtists))
	for i := range artistIDs {
		artistIDs[i] = uint(i + 1)
	}

	for seq := 1; seq <= 5000; seq++ {
		bill := archiveBill(artistIDs, seq)
		if len(bill) == 0 {
			t.Fatalf("seq=%d produced an empty bill; every show needs a headliner", seq)
		}

		seen := map[uint]bool{}
		for pos, sa := range bill {
			if seen[sa.ArtistID] {
				t.Fatalf("seq=%d: artist %d billed twice; this is a show_artists primary-key violation", seq, sa.ArtistID)
			}
			seen[sa.ArtistID] = true

			if sa.Position != pos {
				t.Fatalf("seq=%d: position %d out of order at index %d", seq, sa.Position, pos)
			}
		}
	}
}

func TestArchiveBillHasExactlyOneHeadliner(t *testing.T) {
	artistIDs := make([]uint, len(archiveBillArtists))
	for i := range artistIDs {
		artistIDs[i] = uint(i + 1)
	}

	for seq := 1; seq <= 5000; seq++ {
		var headliners int
		for _, sa := range archiveBill(artistIDs, seq) {
			if sa.SetType == "headliner" {
				headliners++
			}
		}
		if headliners != 1 {
			t.Fatalf("seq=%d: %d headliners, want exactly 1", seq, headliners)
		}
	}
}

func TestArchiveBillProducesWideBills(t *testing.T) {
	// The long roster names only earn their keep if some bill is actually
	// wide enough to wrap. Guards against a future tweak to the size
	// distribution quietly flattening every bill to two acts.
	artistIDs := make([]uint, len(archiveBillArtists))
	for i := range artistIDs {
		artistIDs[i] = uint(i + 1)
	}

	var widest int
	for seq := 1; seq <= 360; seq++ {
		if n := len(archiveBill(artistIDs, seq)); n > widest {
			widest = n
		}
	}
	if widest < 6 {
		t.Errorf("widest bill over the archive's range is %d acts; want >= 6 so bill wrapping is exercised", widest)
	}
}

func TestArchiveNoiseIsNonNegativeAndDeterministic(t *testing.T) {
	// Callers use the result with %, which would panic or index negatively
	// on a negative value.
	for n := 0; n < 10000; n++ {
		if got := archiveNoise(n); got < 0 {
			t.Fatalf("archiveNoise(%d) = %d, want non-negative", n, got)
		}
	}
}

func TestArchiveNoiseMatchesGoldenValues(t *testing.T) {
	// Golden values, not a self-comparison. archiveNoise is what decides
	// each show's bill, and the bill's headliner is baked into the show
	// SLUG — which is the seed's idempotency key. If this hash ever changes,
	// a re-seed stops recognising the archive it already wrote and creates a
	// second copy of all 360 shows. Changing these numbers is therefore a
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

func TestArchiveNoiseIsWellDistributed(t *testing.T) {
	// The badge, price, and bill-size choices all key off small moduli, so a
	// hash that collapsed onto a few residues would make the fixture
	// uniform. Check the moduli the file actually uses.
	for _, mod := range []int{3, 7, 11, 13, 31} {
		counts := make([]int, mod)
		for n := 0; n < 4000; n++ {
			counts[archiveNoise(n)%mod]++
		}
		for residue, c := range counts {
			if c == 0 {
				t.Errorf("mod %d: residue %d never occurs", mod, residue)
			}
		}
	}
}

func TestArchiveVenueLocalDatesKeepTheirCalendarSlot(t *testing.T) {
	// Shows are authored as venue-local evening times and stored in UTC,
	// while the archive's year/month histograms bucket by venue-local date.
	// A 20:00 Phoenix show is 03:00 UTC the NEXT day, so if the histogram
	// and the fixture ever disagreed about the zone, December shows would
	// leak into January. This asserts the round-trip the fixture assumes.
	for _, clock := range archiveDoorTimes {
		local := time.Date(2025, time.December, 31, clock[0], clock[1], 0, 0, phoenixOffset)
		stored := local.UTC()
		back := stored.In(phoenixOffset)

		if back.Year() != 2025 || back.Month() != time.December || back.Day() != 31 {
			t.Errorf("%02d:%02d: round-tripped to %s, want 2025-12-31", clock[0], clock[1], back.Format("2006-01-02"))
		}
		if clock[0] < 12 {
			t.Errorf("%02d:%02d is not an evening time", clock[0], clock[1])
		}
	}
}
