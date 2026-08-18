package main

import (
	"testing"
	"time"

	"psychic-homily-backend/internal/config"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// The archive exemplar generates its shows rather than listing them, so the
// constraints it has to satisfy are properties of the generator, not of any
// one row. These tests pin those properties. Each failure corresponds to a
// specific runtime failure of the seed:
//
//   - repeated day in a month  -> two shows share a slug (the slug is
//     date-only) and collide on the shows unique index
//   - repeated act on a bill   -> duplicate show_artists primary key
//   - unstable show identity   -> slugs change, so a re-seed duplicates the
//     archive instead of skipping it
//   - a non-past archived year -> a show meant for the archive renders
//     under Upcoming instead
//   - thresholds not met       -> the fixture still seeds, but no longer
//     shows the UI it exists to make reviewable

// testArchiveRoster builds a stand-in roster of the given size.
func testArchiveRoster(size int) []archiveAct {
	roster := make([]archiveAct, size)
	for i := 0; i < size; i++ {
		roster[i] = archiveAct{ID: uint(i + 1), Name: archiveBillArtists[i%len(archiveBillArtists)]}
	}
	return roster
}

func testArchiveFullRoster() []archiveAct {
	return testArchiveRoster(len(archiveBillArtists))
}

func TestArchiveDayOfMonthIsDistinctAndInRange(t *testing.T) {
	for count := 1; count <= 28; count++ {
		seen := map[int]bool{}
		for i := 0; i < count; i++ {
			day := archiveDayOfMonth(i, count)
			if day < 1 || day > 28 {
				t.Fatalf("count=%d i=%d: day %d out of the 1-28 range (29-31 would be invalid in February)", count, i, day)
			}
			if seen[day] {
				t.Fatalf("count=%d: day %d generated twice; both shows would generate the same date-only slug", count, day)
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
	if pages := (archiveTotalShows() + pageSize - 1) / pageSize; pages <= fullStripMaxPages {
		t.Errorf("archive totals %d shows = %d pages; want > %d pages so the pager's ellipsis branch renders",
			archiveTotalShows(), pages, fullStripMaxPages)
	}
}

func TestArchiveDocumentedCountsAreAccurate(t *testing.T) {
	// backend/db/seeds/README.md quotes these exact numbers, and a screenshot
	// pass is told which page labels to expect from them. Pin them so the
	// documentation cannot silently go stale — the threshold test above
	// would happily pass with different totals.
	if got := archiveTotalShows(); got != 360 {
		t.Errorf("archive totals %d past shows; README documents 360", got)
	}

	want := map[int]int{2023: 62, 2024: 108, 2025: 190}
	for _, ay := range archiveYears {
		var total int
		for _, count := range ay.months {
			total += count
		}
		if total != want[ay.year] {
			t.Errorf("%d has %d shows; README documents %d", ay.year, total, want[ay.year])
		}
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

	for i := 1; i < len(archiveYears); i++ {
		if archiveYears[i-1].year >= archiveYears[i].year {
			t.Fatalf("archiveYears is not ascending at index %d (%d then %d)", i, archiveYears[i-1].year, archiveYears[i].year)
		}
	}
}

func TestArchiveSeqIsPositionalNotSequential(t *testing.T) {
	// The seed's idempotency key is a slug derived from the headliner, which
	// is derived from the show's seq. If seq depended on how many shows came
	// before it, adding one show to an early month would renumber — and so
	// re-slug — every later show, and the next seed would insert a second
	// copy of all of them instead of skipping.
	//
	// Assert seq depends ONLY on the show's own calendar position.
	if a, b := archiveSeq(2025, 3, 0), archiveSeq(2025, 3, 0); a != b {
		t.Fatalf("archiveSeq is not deterministic: %d vs %d", a, b)
	}

	seen := map[int]string{}
	for _, ay := range archiveYears {
		for monthIdx, count := range ay.months {
			// Deliberately probe one index PAST the current count: that is
			// the show a maintainer would add, and it must not collide with
			// any existing show's identity.
			for i := 0; i <= count; i++ {
				seq := archiveSeq(ay.year, monthIdx+1, i)
				key := time.Date(ay.year, time.Month(monthIdx+1), 1, 0, 0, 0, 0, time.UTC).Format("2006-01") + "#" + string(rune('a'+i%26))
				if prev, dup := seen[seq]; dup {
					t.Fatalf("archiveSeq collision: %d produced by both %s and %s", seq, prev, key)
				}
				seen[seq] = key
			}
		}
	}

	// And it must not collide with the upcoming shows' range.
	for i := 0; i < 10; i++ {
		if _, dup := seen[upcomingSeq(i)]; dup {
			t.Fatalf("upcomingSeq(%d) = %d collides with an archived show's identity", i, upcomingSeq(i))
		}
	}
}

func TestArchiveBillIsWellFormedForEveryRosterSize(t *testing.T) {
	// The roster guard makes the full 14 the only size that reaches the
	// database, but distinctness must hold structurally rather than by that
	// guard alone — a duplicate act on one bill is a show_artists
	// primary-key violation that rolls the whole show back.
	for size := 1; size <= len(archiveBillArtists); size++ {
		roster := testArchiveRoster(size)
		for seq := 1; seq <= 1000; seq++ {
			bill := archiveBill(roster, seq)
			if len(bill) == 0 {
				t.Fatalf("size=%d seq=%d produced an empty bill; every show needs a headliner", size, seq)
			}
			if len(bill) > size {
				t.Fatalf("size=%d seq=%d billed %d acts from a %d-act roster", size, seq, len(bill), size)
			}

			seen := map[uint]bool{}
			var headliners int
			for pos, sa := range bill {
				if seen[sa.ArtistID] {
					t.Fatalf("size=%d seq=%d: act %d billed twice; this is a show_artists primary-key violation", size, seq, sa.ArtistID)
				}
				seen[sa.ArtistID] = true
				if sa.Position != pos {
					t.Fatalf("size=%d seq=%d: position %d out of order at index %d", size, seq, sa.Position, pos)
				}
				if sa.SetType == contracts.SetTypeHeadliner {
					headliners++
				}
			}
			if headliners != 1 {
				t.Fatalf("size=%d seq=%d: %d headliners, want exactly 1", size, seq, headliners)
			}
		}
	}
}

func TestArchiveStrideIsAlwaysCoprime(t *testing.T) {
	for size := 1; size <= 64; size++ {
		for seq := 0; seq < 200; seq++ {
			stride := archiveStride(size, seq)
			if g := gcd(stride, size); g != 1 {
				t.Fatalf("archiveStride(%d, %d) = %d has gcd %d with the roster size; the bill walk would revisit a slot", size, seq, stride, g)
			}
		}
	}
}

func TestArchiveStrideVariesAcrossShows(t *testing.T) {
	// With a single fixed stride the entire bill is determined by the
	// headliner, so every show that act headlined listed identical support.
	// That is the "cloned rows" look this fixture must not have.
	seen := map[int]bool{}
	for seq := 0; seq < 500; seq++ {
		seen[archiveStride(len(archiveBillArtists), seq)] = true
	}
	if len(seen) < 2 {
		t.Errorf("stride took only %d distinct value(s) over 500 shows; bills would be a pure function of the headliner", len(seen))
	}
}

func TestArchiveSameHeadlinerGetsVariedSupport(t *testing.T) {
	// The observable version of the above, asserted on real generated
	// bills: the act that headlines most often must not always bring the
	// same second act.
	roster := testArchiveFullRoster()

	supportByHeadliner := map[uint]map[uint]bool{}
	for _, ay := range archiveYears {
		for monthIdx, count := range ay.months {
			for i := 0; i < count; i++ {
				bill := archiveBill(roster, archiveSeq(ay.year, monthIdx+1, i))
				if len(bill) < 2 {
					continue
				}
				head := bill[0].ArtistID
				if supportByHeadliner[head] == nil {
					supportByHeadliner[head] = map[uint]bool{}
				}
				supportByHeadliner[head][bill[1].ArtistID] = true
			}
		}
	}

	for head, supports := range supportByHeadliner {
		if len(supports) < 2 {
			t.Errorf("headliner %d always draws the same support act; bills would look cloned", head)
		}
	}
}

func TestArchiveHeadlinerMatchesBillLeader(t *testing.T) {
	// The show's title and slug both name the headliner while the bill's
	// first row carries the headliner's ID. If these disagreed, a show would
	// be titled after an act that is not on it — permanently, since the slug
	// is the idempotency key.
	roster := testArchiveFullRoster()

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
	// to wrap.
	roster := testArchiveFullRoster()

	var widest int
	for _, ay := range archiveYears {
		for monthIdx, count := range ay.months {
			for i := 0; i < count; i++ {
				if n := len(archiveBill(roster, archiveSeq(ay.year, monthIdx+1, i))); n > widest {
					widest = n
				}
			}
		}
	}
	if widest < 6 {
		t.Errorf("widest bill across the real archive is %d acts; want >= 6 so bill wrapping is exercised", widest)
	}
}

func TestArchiveDecorationCoversEveryRenderBranch(t *testing.T) {
	// The fixture's job is to make each render branch visible. These were
	// previously asserted only in comments, so a salt or modulus tweak could
	// have removed a branch (every row priced, no cancelled row) without any
	// test noticing.
	var free, priced, absent, soldOut, cancelled, flyers int
	for _, ay := range archiveYears {
		for monthIdx, count := range ay.months {
			for i := 0; i < count; i++ {
				s := &catalogm.Show{}
				decorateArchiveShow(s, archiveSeq(ay.year, monthIdx+1, i))
				switch {
				case s.Price == nil:
					absent++
				case *s.Price == 0:
					free++
				default:
					priced++
				}
				if s.IsSoldOut {
					soldOut++
				}
				if s.IsCancelled {
					cancelled++
				}
				if s.ImageURL != nil {
					flyers++
				}
			}
		}
	}

	for name, got := range map[string]int{
		"price absent": absent, "price Free": free, "price amount": priced,
		"sold out": soldOut, "cancelled": cancelled, "flyer": flyers,
	} {
		if got == 0 {
			t.Errorf("no archived show renders the %q branch; the fixture no longer exercises it", name)
		}
	}
}

func TestArchiveExemplarIsDisabledOnDeployedEnvironments(t *testing.T) {
	// cmd/seed runs on every stage deploy, and stage's catalog can be
	// restored into production. This fixture must never be created there.
	// The polarity matters as much as the exclusion: an unset or unfamiliar
	// ENVIRONMENT must still seed, or every local workflow that does not
	// export ENVIRONMENT would silently lose the fixture.
	for _, tc := range []struct {
		env  string
		want bool
	}{
		{config.EnvProduction, false},
		{config.EnvStage, false},
		{config.EnvDevelopment, true},
		{"", true},
		{"test", true},
	} {
		t.Setenv(config.EnvEnvironment, tc.env)
		if got := archiveExemplarEnabled(); got != tc.want {
			t.Errorf("ENVIRONMENT=%q: archiveExemplarEnabled() = %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestArchiveNoiseIsNonNegative(t *testing.T) {
	// Callers use the result with %, which would index negatively.
	for n := 0; n < 5000; n++ {
		for _, salt := range []int{saltHeadliner, saltPrice, saltCancelled} {
			if got := archiveNoise(n, salt); got < 0 {
				t.Fatalf("archiveNoise(%d, %d) = %d, want non-negative", n, salt, got)
			}
		}
	}
}

func TestArchiveNoiseDecorrelatesSalts(t *testing.T) {
	// The salts are mixed into the hash rather than added to its input
	// precisely so two choices sharing a modulus do not track each other at
	// a fixed offset. An additive salt made the flyer and age-requirement
	// streams identical 16 shows apart, which reads as a stripe down the
	// page. Assert no salt pair correlates at any small offset.
	salts := []int{saltHeadliner, saltBillWide, saltBillSize, saltPriceAbsent, saltDoorTime, saltPrice, saltAge, saltFlyer, saltCancelled, saltSoldOut}
	const samples = 2000

	for _, a := range salts {
		for _, b := range salts {
			if a == b {
				continue
			}
			for offset := 0; offset <= 64; offset++ {
				matches := 0
				for n := 0; n < samples; n++ {
					if archiveNoise(n, a)%3 == archiveNoise(n+offset, b)%3 {
						matches++
					}
				}
				// Independent streams agree ~1/3 of the time. Anything near
				// total agreement means the two are the same sequence.
				if matches > samples*9/10 {
					t.Errorf("salts %d and %d agree on %d/%d samples at offset %d; the streams are correlated", a, b, matches, samples, offset)
				}
			}
		}
	}
}

func TestArchiveNoiseMatchesGoldenValues(t *testing.T) {
	// archiveNoise decides each show's bill, and the bill's headliner is
	// baked into the show SLUG — the seed's idempotency key. If this hash
	// changes, a re-seed stops recognising the archive it already wrote and
	// creates a second copy of all 360 shows. Changing these numbers is a
	// breaking change to existing seeded databases, not a refactor.
	type key struct{ n, salt int }
	golden := map[key]int{
		{0, saltHeadliner}:    1756824724,
		{1, saltHeadliner}:    2136622288,
		{20250301, saltPrice}: 1572519487,
		{20231001, saltFlyer}: 1367879223,
		{1000, saltCancelled}: 1531027741,
	}
	for k, want := range golden {
		if got := archiveNoise(k.n, k.salt); got != want {
			t.Errorf("archiveNoise(%d, %d) = %d, want %d", k.n, k.salt, got, want)
		}
	}
}

func TestArchiveVenueZoneMatchesTheSlugResolver(t *testing.T) {
	// Two independent resolvers decide this fixture's dates:
	// archiveVenueZone builds the timestamps, while utils.GenerateShowSlug
	// resolves the zone from the STATE alone (it ignores the venue's
	// explicit timezone). If they disagreed, a 20:00 show's slug date and
	// its displayed venue-local date would differ, so the archive would be
	// keyed on one day and rendered on another.
	fromState := utils.GetTimezoneForState(exemplarArchiveVenueState)
	if archiveVenueZone.String() != fromState {
		t.Fatalf("archiveVenueZone is %s but GenerateShowSlug resolves %s for state %s",
			archiveVenueZone, fromState, exemplarArchiveVenueState)
	}

	// And catch the tzdata-missing fallback: utils.EventLocation degrades to
	// UTC, under which every evening show would bucket a day late.
	if archiveVenueZone.String() == "UTC" {
		t.Fatal("archiveVenueZone resolved to UTC; venue-local evening times would bucket into the wrong day")
	}
	if archiveVenueZone.String() != exemplarArchiveVenueTimezone {
		t.Errorf("archiveVenueZone is %s, want %s (the zone stamped on the venue row)", archiveVenueZone, exemplarArchiveVenueTimezone)
	}
}

func TestArchiveGeneratedDatesStayInTheirCalendarBucket(t *testing.T) {
	// Shows are authored as venue-local evening times and stored in UTC,
	// while the year/month histograms bucket by venue-local date. A 21:00
	// Phoenix show is 04:00 UTC the NEXT day, so a zone error would move it
	// into the following month — and, in December, the following YEAR,
	// inventing a year in the strip.
	//
	// This walks the real generator output rather than round-tripping a
	// constant, so it fails if the zone, the door times, or the day spread
	// ever conspire to cross a boundary.
	for _, ay := range archiveYears {
		for monthIdx, count := range ay.months {
			month := time.Month(monthIdx + 1)
			for i := 0; i < count; i++ {
				seq := archiveSeq(ay.year, monthIdx+1, i)
				clock := archiveDoorTimes[archiveNoise(seq, saltDoorTime)%len(archiveDoorTimes)]
				stored := time.Date(ay.year, month, archiveDayOfMonth(i, count),
					clock.hour, clock.minute, 0, 0, archiveVenueZone).UTC()

				local := stored.In(archiveVenueZone)
				if local.Year() != ay.year || local.Month() != month {
					t.Fatalf("show %d/%02d #%d stored as %s buckets to %d-%02d in venue-local time",
						ay.year, month, i, stored, local.Year(), local.Month())
				}
				if clock.hour < 12 {
					t.Errorf("%02d:%02d is not an evening time", clock.hour, clock.minute)
				}
			}
		}
	}
}
