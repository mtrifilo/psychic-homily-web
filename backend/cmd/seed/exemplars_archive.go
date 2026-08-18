package main

import (
	"fmt"
	"log"
	"slices"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"

	"gorm.io/gorm"
)

// Archive exemplar venue (PSY-1843).
//
// PSY-665 gave every entity type ONE rich exemplar, which closed the
// "optional fields are NULL locally" gap. It did not close a second one:
// no seeded venue has enough HISTORY to review the past-shows archive.
// The venue page's archive UI (year strip with per-year counts,
// month-range page labels, 50-per-page pagination) only becomes visible
// once a venue has hundreds of past shows spread across several years, so
// on a fresh dispatch stack that UI could not be visually reviewed at all
// — during PR #1904's review a throwaway venue had to be hand-written
// into a stack with raw SQL to see it. This file makes that fixture
// permanent, idempotent, and conventional.
//
// Design rules (inherited from exemplars.go, plus two specific to history):
//
//   - Additive + idempotent: a new "-exemplar" venue slug, and every show
//     create is guarded by a slug existence check.
//
//   - Self-contained: the bills are EXEMPLAR-STYLE FICTIONAL artists this
//     file creates, never references to real catalog artists, so the seed
//     does not depend on which artists a given database happens to hold.
//     Names vary from one syllable to five words so the archive's bill
//     line exercises wrapping.
//
//   - FIXED past dates, not offsets from time.Now(). The archive's shows
//     are anchored to three literal calendar years (2023-2025), which are
//     past today and stay past forever. Deriving them from "now" would
//     change every generated slug on every run, and the slug guard is what
//     makes the seed idempotent — a date-derived slug would re-create the
//     whole archive on the next day's run. The handful of UPCOMING shows
//     necessarily are relative to now, so they use fixed non-date slugs
//     instead (the same trade-off seedExemplarArtistShows already makes:
//     their dates are set once, on first seed, and are not refreshed).
//
//   - Deterministic variety, no RNG. Per-show variation (bill size, price,
//     start time, badges) comes from archiveNoise, a pure integer hash of
//     the show index. math/rand would also be reproducible within one Go
//     release but is not guaranteed stable across them, and an unstable
//     sequence would silently change the generated slugs — re-creating the
//     archive instead of skipping it.
//
// The slug is documented in backend/db/seeds/README.md alongside the
// PSY-665 exemplar table.

const exemplarArchiveVenueSlug = "chronology-hall-exemplar-phoenix-az"

// exemplarArchiveVenueName is used both for the venue row and as the
// venue half of every generated show slug, so it is named once.
const exemplarArchiveVenueName = "Chronology Hall (Exemplar)"

// phoenixOffset is Arizona's UTC offset. Phoenix does not observe DST, so
// a fixed zone is exactly equivalent to America/Phoenix all year and needs
// no tzdata lookup — the seed can therefore build venue-local evening
// times without depending on the host's zoneinfo being present.
var phoenixOffset = time.FixedZone("MST", -7*60*60)

// archiveShowsPerMonth is the per-month show count for each archived year,
// indexed January..December. Hand-authored rather than generated so the
// review fixture has a deliberate shape:
//
//   - Density RISES toward the most recent year (62 -> 108 -> 190), so the
//     year strip shows an obviously uneven distribution rather than three
//     identical bars.
//   - EVERY year paginates at the frontend's 50-per-page: 2 pages, 3 pages,
//     4 pages. "Multi-page in more than one year" is therefore provable
//     from any year in the strip, not just the densest.
//   - The 360 total also puts the all-years pager past Pagination's
//     FULL_STRIP_MAX_PAGES of 7 (360/50 = 8 pages), which is the only way
//     to see its ellipsis/truncation branch rather than a full numeric run.
//   - Every year has at least one EMPTY month, so the month-range page
//     labels have to skip gaps rather than always spanning consecutive
//     months.
//   - Counts stay at or under 28 per month: archiveDayOfMonth spreads shows
//     over the first 28 days, and the one-show-per-day spacing is what
//     keeps the show_artists dedup index satisfied.
//
// All three years are fully in the past and always will be, so the archive
// never silently acquires an "upcoming" row.
var archiveShowsPerMonth = map[int][12]int{
	2023: {3, 4, 6, 5, 0, 7, 4, 6, 0, 9, 10, 8},           // 62 — two pages
	2024: {8, 7, 11, 9, 10, 8, 0, 9, 12, 14, 11, 9},       // 108 — three pages
	2025: {14, 12, 18, 17, 15, 13, 0, 16, 19, 22, 24, 20}, // 190 — four pages
}

// archiveBillArtists are the fictional acts the archive's bills are drawn
// from. Lengths are deliberately uneven — one-word names through five-word
// names — because the archive row renders the whole bill on one line, and a
// fixture of uniformly medium names would never show how it wraps.
//
// Every name carries the "(Exemplar)" marker, which is what gives each
// artist an "-exemplar" slug through the normal slug funnel (PSY-665's
// convention) and keeps these acts from ever being mistaken for, or
// colliding with, real catalog artists.
var archiveBillArtists = []string{
	"Vane (Exemplar)",
	"Dust (Exemplar)",
	"Kilter (Exemplar)",
	"Halberd Moon (Exemplar)",
	"Saguaro Static (Exemplar)",
	"Coyote Almanac (Exemplar)",
	"Slow Bureau (Exemplar)",
	"Ferrous Wheel (Exemplar)",
	"Palm Reader Union (Exemplar)",
	"Nightjar Parliament (Exemplar)",
	"The Ivory Sextant Society (Exemplar)",
	"Everlasting Radio Transmission Choir (Exemplar)",
	"Ada Vaughn-Reyes and the Long Goodbye (Exemplar)",
	"Moss (Exemplar)",
}

// archivePrices is the ticket-price menu. Most archived shows draw from it;
// a deterministic minority get no price at all, because "price unknown" is
// the common real case for old listings and the archive row has to render
// that absence without leaving a dangling separator.
//
// The leading 0 is deliberate, not padding: the price formatter renders 0
// as "Free" and any other number as "$12.00", so all THREE render branches
// (Free / an amount / absent) need a row in the fixture to be reviewable.
// 12.50 is here for the same reason — it is the only entry that proves the
// two-decimal format, which whole dollars would hide.
var archivePrices = []float64{0, 10, 12, 12.50, 15, 18, 20, 22, 25, 28, 32, 38}

// archiveDoorTimes are venue-LOCAL start hours/minutes. Evening times are
// the point: a UTC-midnight fixture would render as the previous day in
// Phoenix and quietly hide any timezone bug in the archive's date grouping.
var archiveDoorTimes = [][2]int{{19, 0}, {20, 0}, {20, 30}, {21, 0}, {19, 30}}

// archiveNoise is a deterministic integer hash (a splitmix-style bit mix)
// used wherever a show needs a stable arbitrary choice — bill size, price,
// start time, badges. Same input, same output, on every machine and every
// Go release, which is what keeps generated slugs stable and therefore the
// seed idempotent. Always non-negative so callers can use % directly.
func archiveNoise(n int) int {
	x := uint32(n)*2654435761 + 374761393
	x ^= x >> 13
	x *= 1274126177
	x ^= x >> 16
	return int(x & 0x7fffffff)
}

// seedExemplarArchiveVenue creates the archive exemplar venue itself.
// Returns its ID, or 0 if the venue could not be created.
func seedExemplarArchiveVenue(db *gorm.DB, userID uint) uint {
	var existing catalogm.Venue
	if db.Where("slug = ?", exemplarArchiveVenueSlug).First(&existing).Error == nil {
		return existing.ID
	}

	venue := &catalogm.Venue{
		Name:    exemplarArchiveVenueName,
		Slug:    strptr(exemplarArchiveVenueSlug),
		Address: strptr("818 N 2nd St"),
		City:    "Phoenix",
		State:   "AZ",
		Country: strptr("USA"),
		Zipcode: strptr("85004"),
		// Explicit timezone (rather than relying on the state->tz fallback)
		// so the archive's date grouping is exercised against a venue that
		// actually carries one.
		Timezone:    strptr("America/Phoenix"),
		Description: strptr("A mid-size downtown Phoenix room with a long booking history across indie, punk, and experimental bills. Two rooms, early and late sets most weekends.\n\nSeeded as the PSY-1843 archive exemplar: unlike the other exemplars, this venue's point is its BACKLOG. It carries several hundred past shows spread over three calendar years so the past-shows archive renders at full size — year strip with per-year counts, month-range page labels, pagination, sold-out and cancelled badges, prices, and multi-act bills that wrap."),
		ImageURL:    strptr("/seed-placeholders/venue.svg"),
		Social:      fullSocial("chronologyhall"),
		Verified:    true,
	}
	if err := db.Create(venue).Error; err != nil {
		log.Printf("Warning: failed to create archive exemplar venue: %v", err)
		return 0
	}

	applyTags(db, catalogm.TagEntityVenue, venue.ID, userID, []tagSpec{
		{"Indie Rock", "indie-rock", catalogm.TagCategoryGenre},
		{"Punk", "punk", catalogm.TagCategoryGenre},
		{"Experimental", "experimental", catalogm.TagCategoryGenre},
		{"Phoenix", "phoenix", catalogm.TagCategoryLocale},
		{"All Ages", "all-ages", catalogm.TagCategoryOther},
		{"Deep Archive", "deep-archive", catalogm.TagCategoryOther},
	})

	fmt.Printf("  ✅ archive venue exemplar: %s\n", exemplarArchiveVenueSlug)
	return venue.ID
}

// seedExemplarArchiveShows fills the archive venue's history: the fictional
// bill artists, the fixed-date past shows, and a handful of upcoming shows
// so the venue page's Upcoming/Past split has both halves.
func seedExemplarArchiveShows(db *gorm.DB, venueID uint) {
	if venueID == 0 {
		return
	}

	artistIDs := findOrCreateArchiveBillArtists(db)
	if len(artistIDs) == 0 {
		log.Printf("Warning: no archive bill artists; skipping archive shows")
		return
	}

	created := seedArchivePastShows(db, venueID, artistIDs)
	created += seedArchiveUpcomingShows(db, venueID, artistIDs)

	if created > 0 {
		fmt.Printf("  ✅ archive exemplar: %d shows created\n", created)
	} else {
		fmt.Printf("  ✅ archive exemplar: shows already present\n")
	}
}

// findOrCreateArchiveBillArtists resolves every fictional bill act to an
// artist ID, creating the missing ones. Routed through findOrCreateArtist
// (and so through the single PSY-1254 artist write funnel) rather than a
// raw insert, which is what gives them their "-exemplar" slugs and makes
// re-running a no-op: the funnel dedups on LOWER(name).
//
// The acts are created WITHOUT a location on purpose. A location would
// derive a metro and enrol these fictional bands in the Phoenix scene
// rollup, changing counts on an unrelated page; the archive fixture has no
// need for it.
func findOrCreateArchiveBillArtists(db *gorm.DB) []uint {
	ids := make([]uint, 0, len(archiveBillArtists))
	for _, name := range archiveBillArtists {
		artist, err := findOrCreateArtist(db, name)
		if err != nil {
			log.Printf("Warning: failed to create archive bill artist %s: %v", name, err)
			continue
		}
		ids = append(ids, artist.ID)
	}
	return ids
}

// seedArchivePastShows walks archiveShowsPerMonth and creates one show per
// scheduled slot. Returns the number of shows actually created.
func seedArchivePastShows(db *gorm.DB, venueID uint, artistIDs []uint) int {
	var created int
	// seq numbers every show across the whole archive so archiveNoise gives
	// each one a different variation, and it advances even for shows that
	// already exist — otherwise a partially-seeded archive would resume with
	// a different variation sequence than it started with.
	var seq int

	for _, year := range archiveYearsAscending() {
		months := archiveShowsPerMonth[year]
		for monthIdx, count := range months {
			for i := 0; i < count; i++ {
				seq++
				day := archiveDayOfMonth(i, count)
				clock := archiveDoorTimes[archiveNoise(seq)%len(archiveDoorTimes)]
				eventDate := time.Date(
					year, time.Month(monthIdx+1), day,
					clock[0], clock[1], 0, 0, phoenixOffset,
				).UTC()

				if createArchiveShow(db, venueID, artistIDs, eventDate, seq) {
					created++
				}
			}
		}
	}
	return created
}

// archiveYearsAscending returns the archived years in chronological order.
// Map iteration order is randomized in Go, and the shows must be created in
// a stable order for `seq` (and therefore every show's variation) to be
// reproducible across runs.
func archiveYearsAscending() []int {
	years := make([]int, 0, len(archiveShowsPerMonth))
	for year := range archiveShowsPerMonth {
		years = append(years, year)
	}
	slices.Sort(years)
	return years
}

// archiveDayOfMonth spreads `count` shows across the first 28 days of a
// month, returning the day for the i-th one. Capping at 28 keeps February
// valid without a per-month length table, and the spacing guarantees
// DISTINCT days: at most 16 shows per month, so the step never rounds two
// indexes onto the same day. Distinct days matter beyond looking tidy —
// show_artists carries a partial unique index on
// (artist_id, venue_id, event_date), so two same-day shows sharing an act
// at this venue would be a constraint violation, not just a duplicate.
func archiveDayOfMonth(i, count int) int {
	if count <= 1 {
		return 15
	}
	return 1 + (i*28)/count
}

// createArchiveShow creates one archived show with its venue link and bill.
// Returns true if a row was created, false if it already existed or failed.
func createArchiveShow(db *gorm.DB, venueID uint, artistIDs []uint, eventDate time.Time, seq int) bool {
	bill := archiveBill(artistIDs, seq)
	headlinerName := archiveBillArtists[archiveBillIndex(seq, 0, len(artistIDs))]
	slug := utils.GenerateShowSlug(eventDate, headlinerName, exemplarArchiveVenueName, "AZ")

	var existing catalogm.Show
	if db.Where("slug = ?", slug).First(&existing).Error == nil {
		return false
	}

	show := &catalogm.Show{
		Title:     fmt.Sprintf("%s at %s", headlinerName, exemplarArchiveVenueName),
		Slug:      &slug,
		EventDate: eventDate,
		City:      strptr("Phoenix"),
		State:     strptr("AZ"),
		Status:    catalogm.ShowStatusApproved,
		Source:    catalogm.ShowSourceUser,
	}
	decorateArchiveShow(show, seq)

	if err := createShowWithBill(db, show, venueID, bill); err != nil {
		log.Printf("Warning: failed to create archive show %s: %v", slug, err)
		return false
	}
	return true
}

// decorateArchiveShow applies the deterministic per-show variation: price
// (absent for a minority), age requirement, flyer, and the sold-out /
// cancelled badges.
func decorateArchiveShow(show *catalogm.Show, seq int) {
	// ~1 in 7 archived shows has no price, so the archive row's
	// price-absent branch stays exercised.
	if archiveNoise(seq+11)%7 != 0 {
		price := archivePrices[archiveNoise(seq+23)%len(archivePrices)]
		show.Price = &price
	}

	switch archiveNoise(seq+37) % 3 {
	case 0:
		show.AgeRequirement = strptr("21+")
	case 1:
		show.AgeRequirement = strptr("All Ages")
		// case 2 leaves it unset — unknown is the common archival case.
	}

	// A flyer on roughly a third of rows: enough to see the archive's
	// with-image and without-image rows side by side.
	if archiveNoise(seq+53)%3 == 0 {
		show.ImageURL = strptr("/seed-placeholders/show.svg")
	}

	// Badges are mutually exclusive and cancelled wins: a show that was
	// called off is not also "sold out", and rendering both would be a
	// fixture artifact rather than a real state worth reviewing.
	//
	// These two are set to TRUE only. Writing `false` explicitly would be
	// pointless (the columns default to false) and, on GORM's Create path,
	// a false bool is the zero value anyway — so the DB default is what
	// lands. Only the true case needs saying.
	switch {
	case archiveNoise(seq+71)%31 == 0:
		show.IsCancelled = true
	case archiveNoise(seq+97)%13 == 0:
		show.IsSoldOut = true
	}
}

// archiveBillIndex picks the artist slot for position `pos` on show `seq`.
// The stride is coprime with a 14-act roster, so the positions on any one
// bill are always DISTINCT acts — an artist billed twice on the same show
// would violate the show_artists primary key.
func archiveBillIndex(seq, pos, rosterSize int) int {
	const stride = 3
	return (archiveNoise(seq)%rosterSize + pos*stride) % rosterSize
}

// archiveBill builds the ordered bill for one show, the first act of which
// headlines. Most nights are 1-3 acts, but roughly one in eleven is a
// 6-to-8 act bill.
//
// Those big bills are the reason the roster carries five-word names. The
// archive renders the entire bill inline with no cap and no truncation, so
// the only fixture that exercises its wrapping is one with enough long
// names on a single row to overflow the column — a fixture of uniformly
// short two-act bills would render every row on one line and prove nothing.
func archiveBill(artistIDs []uint, seq int) []catalogm.ShowArtist {
	var size int
	if archiveNoise(seq+3)%11 == 0 {
		size = 6 + archiveNoise(seq+5)%3
	} else {
		size = 1 + archiveNoise(seq+5)%3
	}
	if size > len(artistIDs) {
		size = len(artistIDs)
	}

	bill := make([]catalogm.ShowArtist, 0, size)
	for pos := 0; pos < size; pos++ {
		// set_type is CURATED, never inferred from list order (PSY-1673).
		// Headliner is the single inference the vocabulary sanctions, so
		// position 0 gets "headliner" and every other slot gets the neutral
		// "performer" — asserting "opener" or "direct_support" here would
		// publish a guess as a fact.
		setType := contracts.SetTypeDefault
		if pos == 0 {
			setType = contracts.SetTypeHeadliner
		}
		bill = append(bill, catalogm.ShowArtist{
			ArtistID: artistIDs[archiveBillIndex(seq, pos, len(artistIDs))],
			Position: pos,
			SetType:  setType,
		})
	}
	return bill
}

// createShowWithBill writes a show, its venue link, and its bill in one
// transaction.
//
// It stamps the denormalized show_artists.(event_date, venue_id) columns
// that the PSY-576 partial unique index covers, matching what
// ShowService.CreateShow does via syncShowArtistDedupColumns. The other
// exemplars leave those NULL (the index skips NULL rows, which sidesteps
// the constraint); this fixture populates them instead, because an archive
// of hundreds of shows at ONE venue is precisely the shape the dedup index
// exists to police, and a fixture that opted out of it would not represent
// production data.
func createShowWithBill(db *gorm.DB, show *catalogm.Show, venueID uint, bill []catalogm.ShowArtist) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(show).Error; err != nil {
			return fmt.Errorf("create show: %w", err)
		}
		if err := tx.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error; err != nil {
			return fmt.Errorf("link venue: %w", err)
		}
		for _, sa := range bill {
			sa.ShowID = show.ID
			sa.EventDate = &show.EventDate
			sa.VenueID = &venueID
			if err := tx.Create(&sa).Error; err != nil {
				return fmt.Errorf("link artist %d: %w", sa.ArtistID, err)
			}
		}
		return nil
	})
}

// seedArchiveUpcomingShows adds a few future shows so the venue page's
// Upcoming section is populated alongside the archive.
//
// These are the one part of the fixture that cannot use fixed dates — a
// hardcoded future date stops being future — so they are keyed by a fixed
// NON-date slug and dated relative to now. The consequence, which
// seedExemplarArtistShows already accepts for its own upcoming shows, is
// that their dates are set on first seed and not refreshed afterwards.
// Dispatch stacks are seeded fresh, so in practice they are always future.
func seedArchiveUpcomingShows(db *gorm.DB, venueID uint, artistIDs []uint) int {
	dayOffsets := []int{5, 12, 19, 33, 48}
	var created int

	for i, offset := range dayOffsets {
		slug := fmt.Sprintf("chronology-hall-exemplar-upcoming-%d", i+1)
		var existing catalogm.Show
		if db.Where("slug = ?", slug).First(&existing).Error == nil {
			continue
		}

		// Anchor to a venue-local 20:00 on the offset day rather than
		// "now + N days", so upcoming rows show an evening time like the
		// archived ones instead of whatever o'clock the seed ran at.
		local := time.Now().In(phoenixOffset).AddDate(0, 0, offset)
		eventDate := time.Date(
			local.Year(), local.Month(), local.Day(), 20, 0, 0, 0, phoenixOffset,
		).UTC()

		seq := 100000 + i // disjoint from the archive's seq range
		bill := archiveBill(artistIDs, seq)
		headlinerName := archiveBillArtists[archiveBillIndex(seq, 0, len(artistIDs))]

		show := &catalogm.Show{
			Title:     fmt.Sprintf("%s at %s", headlinerName, exemplarArchiveVenueName),
			Slug:      &slug,
			EventDate: eventDate,
			City:      strptr("Phoenix"),
			State:     strptr("AZ"),
			Status:    catalogm.ShowStatusApproved,
			Source:    catalogm.ShowSourceUser,
			TicketURL: strptr("https://tickets.example.com/" + slug),
		}
		decorateArchiveShow(show, seq)

		if err := createShowWithBill(db, show, venueID, bill); err != nil {
			log.Printf("Warning: failed to create archive upcoming show %s: %v", slug, err)
			continue
		}
		created++
	}
	return created
}
