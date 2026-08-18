package main

import (
	"fmt"
	"log"
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
// The venue page's archive UI only becomes visible once a venue has
// hundreds of past shows over several years, so on a fresh dispatch stack
// it could not be reviewed at all.
//
// Operator-facing detail (what this fixture contains, and the URLs to
// point a screenshot pass at) lives in backend/db/seeds/README.md. The
// comments here cover the reasoning a person EDITING this file needs.
//
// Two constraints shape most of what follows:
//
//   - Past dates are FIXED calendar years, not offsets from time.Now().
//     Each show's slug embeds its date and the slug is the idempotency
//     key, so now()-derived dates would generate new slugs on every run
//     and re-create the whole archive daily. Upcoming shows cannot work
//     that way, so they use fixed non-date slugs instead — the same
//     trade-off seedExemplarArtistShows already makes, meaning their dates
//     are set once on first seed and never refreshed.
//
//   - Per-show variation is a pure hash (archiveNoise), not math/rand,
//     whose sequence is not guaranteed stable across Go releases. An
//     unstable sequence would change the generated slugs and duplicate the
//     archive instead of skipping it. A golden test pins the hash so that
//     stays a deliberate decision rather than an accident.

const (
	exemplarArchiveVenueSlug     = "chronology-hall-exemplar-phoenix-az"
	exemplarArchiveVenueName     = "Chronology Hall (Exemplar)"
	exemplarArchiveVenueState    = "AZ"
	exemplarArchiveVenueTimezone = "America/Phoenix"
)

// archiveVenueZone is the venue's local zone, resolved through the SAME
// helper the render path uses (utils.EventLocation) rather than a
// hand-built fixed offset.
//
// That matters because the archive's year and month histograms bucket by
// venue-local date: the fixture and the app must agree on the zone or a
// 20:00 show on Dec 31 (03:00 UTC Jan 1) lands in the wrong year. Sharing
// the resolver makes disagreement impossible by construction instead of by
// a comment asserting two definitions match.
var archiveVenueZone = utils.EventLocation(strptr(exemplarArchiveVenueTimezone), exemplarArchiveVenueState)

// archiveYear is one archived calendar year and its per-month show counts,
// indexed January..December.
type archiveYear struct {
	year   int
	months [12]int
}

// archiveYears is the fixture's shape. A slice, not a map, so creation
// order is lexical and stable — the shows must be generated in a fixed
// order for each one's `seq` (and therefore its variation and slug) to be
// reproducible across runs.
//
// The counts are chosen against the archive UI's actual thresholds, not
// picked round. Changing them is what this fixture IS, so
// TestArchiveFixtureMeetsItsReviewThresholds pins the properties below
// rather than leaving them as prose that a future trim would silently
// falsify:
//
//   - Density rises toward the most recent year, so the year strip shows an
//     uneven distribution rather than three identical bars.
//   - EVERY year exceeds the frontend's 50-per-page limit (2, 3, and 4
//     pages), so multi-page is provable from any year in the strip.
//   - The 360 total clears Pagination's FULL_STRIP_MAX_PAGES of 7 (8 pages),
//     which is the only way to render its ellipsis branch.
//   - Every year has at least one EMPTY month, so the month-range page
//     labels must skip gaps rather than always spanning consecutive months.
//   - No month exceeds 28, the span archiveDayOfMonth can spread over.
//
// All three years are fully past and always will be, so the archive never
// silently acquires an "upcoming" row.
var archiveYears = []archiveYear{
	{2023, [12]int{3, 4, 6, 5, 0, 7, 4, 6, 0, 9, 10, 8}},           // 62 — two pages
	{2024, [12]int{8, 7, 11, 9, 10, 8, 0, 9, 12, 14, 11, 9}},       // 108 — three pages
	{2025, [12]int{14, 12, 18, 17, 15, 13, 0, 16, 19, 22, 24, 20}}, // 190 — four pages
}

// archiveBillArtists are the fictional acts the bills are drawn from.
// Lengths are deliberately uneven — one word through six — because the
// archive renders a whole bill inline with no cap or truncation, so only a
// fixture with long names on a wide bill shows how it wraps.
//
// The "(Exemplar)" marker is what earns each act an "-exemplar" slug
// through the normal slug funnel (PSY-665's convention) and keeps these
// from colliding with real catalog artists.
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

// archiveAct pairs an act's database ID with its name. Keeping them in one
// record (rather than two positionally-aligned slices) is what lets the
// headliner's name and ID come from a single lookup — the show TITLE and
// SLUG both name the headliner, and the slug is the idempotency key, so an
// index skew between two slices would be a silent correctness problem.
type archiveAct struct {
	ID   uint
	Name string
}

// archivePrices is the ticket-price menu. The leading 0 is deliberate: the
// price formatter renders 0 as "Free" and anything else as "$12.00", and a
// deterministic minority of shows get no price at all, so all THREE render
// branches (Free / an amount / absent) appear in the fixture. 12.50 is the
// only entry that proves the two-decimal format, which whole dollars hide.
var archivePrices = []float64{0, 10, 12, 12.50, 15, 18, 20, 22, 25, 28, 32, 38}

// archiveClock is a venue-LOCAL start time. Evening times are the point: a
// UTC-midnight fixture would render as the previous day in Phoenix and
// quietly hide a timezone bug in the archive's date grouping.
type archiveClock struct {
	hour   int
	minute int
}

var archiveDoorTimes = []archiveClock{
	{19, 0}, {20, 0}, {20, 30}, {21, 0}, {19, 30},
}

// Salts for archiveNoise. Each per-show choice draws from its own salt so
// the choices are decorrelated — sharing one would tie, say, "is sold out"
// to "is expensive" and make the fixture look patterned.
//
// The asymmetry worth knowing before changing any of these: saltBillSize
// and saltBillWide (and the unsalted archiveNoise(seq) that picks the
// headliner) feed the show's SLUG, so changing them re-creates the archive
// on the next seed instead of skipping it. The decoration salts only
// reshuffle prices and badges and are free to change.
const (
	saltBillWide    = 3
	saltBillSize    = 5
	saltPriceAbsent = 11
	saltPrice       = 23
	saltAge         = 37
	saltFlyer       = 53
	saltCancelled   = 71
	saltSoldOut     = 97
)

// upcomingSeqBase keeps the upcoming shows' variation sequence disjoint
// from the archive's, so adding an archived show never reshuffles them.
const upcomingSeqBase = 100000

// archiveNoise is a deterministic integer hash (a splitmix-style bit mix)
// used wherever a show needs a stable arbitrary choice. Same input, same
// output, on every machine and every Go release — which is what keeps the
// generated slugs stable and therefore the seed idempotent. Always
// non-negative so callers can use % directly.
func archiveNoise(n int) int {
	x := uint32(n)*2654435761 + 374761393
	x ^= x >> 13
	x *= 1274126177
	x ^= x >> 16
	return int(x & 0x7fffffff)
}

// seedExemplarArchiveVenue creates the archive exemplar venue itself.
// Returns its ID, or 0 if it could not be created.
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
		State:   exemplarArchiveVenueState,
		Country: strptr("USA"),
		Zipcode: strptr("85004"),
		// Set explicitly rather than left to the state->tz fallback, so the
		// archive's date grouping is exercised against a venue that actually
		// carries a timezone (the branch the fallback would otherwise hide).
		Timezone:    strptr(exemplarArchiveVenueTimezone),
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

// seedExemplarArchiveShows fills the venue's history: the fictional bill
// acts, the fixed-date past shows, and a handful of upcoming shows so the
// venue page's Upcoming/Past split has both halves.
func seedExemplarArchiveShows(db *gorm.DB, venueID uint) {
	if venueID == 0 {
		return
	}

	roster := findOrCreateArchiveBillActs(db)
	if len(roster) == 0 {
		log.Printf("Warning: no archive bill acts; skipping archive shows")
		return
	}

	created := seedArchivePastShows(db, venueID, roster)
	created += seedArchiveUpcomingShows(db, venueID, roster)

	if created > 0 {
		fmt.Printf("  ✅ archive exemplar: %d shows created\n", created)
	} else {
		fmt.Printf("  ✅ archive exemplar: shows already present\n")
	}
}

// findOrCreateArchiveBillActs resolves every fictional act, creating the
// missing ones. Routed through findOrCreateArtist (and so the single
// PSY-1254 artist write funnel) rather than a raw insert, which is what
// gives them their "-exemplar" slugs and makes re-running a no-op — the
// funnel dedups on LOWER(name).
//
// The acts are created WITHOUT a location on purpose: a location would
// derive a metro and enrol these fictional bands in the Phoenix scene
// rollup, changing counts on a page unrelated to this fixture.
func findOrCreateArchiveBillActs(db *gorm.DB) []archiveAct {
	roster := make([]archiveAct, 0, len(archiveBillArtists))
	for _, name := range archiveBillArtists {
		artist, err := findOrCreateArtist(db, name)
		if err != nil {
			log.Printf("Warning: failed to create archive bill act %s: %v", name, err)
			continue
		}
		roster = append(roster, archiveAct{ID: artist.ID, Name: name})
	}
	return roster
}

// seedArchivePastShows walks archiveYears and creates one show per
// scheduled slot. Returns the number of shows actually created.
func seedArchivePastShows(db *gorm.DB, venueID uint, roster []archiveAct) int {
	var created int
	// seq numbers every show across the whole archive so each gets a
	// different variation. It advances even for shows that already exist,
	// so resuming a partially-seeded archive continues the same sequence it
	// started with rather than re-deriving a different one.
	var seq int

	for _, ay := range archiveYears {
		for monthIdx, count := range ay.months {
			for i := 0; i < count; i++ {
				seq++
				clock := archiveDoorTimes[archiveNoise(seq)%len(archiveDoorTimes)]
				eventDate := time.Date(
					ay.year, time.Month(monthIdx+1), archiveDayOfMonth(i, count),
					clock.hour, clock.minute, 0, 0, archiveVenueZone,
				).UTC()

				headliner := archiveHeadliner(roster, seq)
				slug := utils.GenerateShowSlug(eventDate, headliner.Name, exemplarArchiveVenueName, exemplarArchiveVenueState)

				if createArchiveShow(db, venueID, roster, eventDate, seq, slug, nil) {
					created++
				}
			}
		}
	}
	return created
}

// archiveDayOfMonth spreads `count` shows across the first 28 days of a
// month, returning the day for the i-th one. Capping at 28 keeps February
// valid without a per-month length table, and the spacing guarantees
// DISTINCT days for any count up to 28: the step 28/count is then >= 1, so
// the truncating division never rounds two indexes onto the same day.
//
// Distinct days matter beyond looking tidy — show_artists carries a partial
// unique index on (artist_id, venue_id, event_date), so two same-day shows
// sharing an act at this venue would be a constraint violation, not just a
// duplicate.
func archiveDayOfMonth(i, count int) int {
	if count <= 1 {
		return 15
	}
	return 1 + (i*28)/count
}

// createArchiveShow creates one show with its venue link and bill, keyed by
// the caller's slug. Both the archived and upcoming paths use it, so the
// show's shape is defined once; they differ only in how the slug is derived
// (date-based vs fixed) and whether a ticket URL applies.
//
// Returns true if a row was created, false if it already existed or failed.
func createArchiveShow(
	db *gorm.DB,
	venueID uint,
	roster []archiveAct,
	eventDate time.Time,
	seq int,
	slug string,
	ticketURL *string,
) bool {
	var existing catalogm.Show
	if db.Where("slug = ?", slug).First(&existing).Error == nil {
		return false
	}

	headliner := archiveHeadliner(roster, seq)
	show := &catalogm.Show{
		Title:     fmt.Sprintf("%s at %s", headliner.Name, exemplarArchiveVenueName),
		Slug:      &slug,
		EventDate: eventDate,
		City:      strptr("Phoenix"),
		State:     strptr(exemplarArchiveVenueState),
		Status:    catalogm.ShowStatusApproved,
		Source:    catalogm.ShowSourceUser,
		TicketURL: ticketURL,
	}
	decorateArchiveShow(show, seq)

	if err := createArchiveShowWithBill(db, show, venueID, archiveBill(roster, seq)); err != nil {
		log.Printf("Warning: failed to create archive show %s: %v", slug, err)
		return false
	}
	return true
}

// decorateArchiveShow applies the deterministic per-show variation: price
// (absent for a minority), age requirement, flyer, and the badges.
func decorateArchiveShow(show *catalogm.Show, seq int) {
	// ~1 in 7 archived shows has no price, so the archive row's
	// price-absent branch stays exercised.
	if archiveNoise(seq+saltPriceAbsent)%7 != 0 {
		price := archivePrices[archiveNoise(seq+saltPrice)%len(archivePrices)]
		show.Price = &price
	}

	switch archiveNoise(seq+saltAge) % 3 {
	case 0:
		show.AgeRequirement = strptr("21+")
	case 1:
		show.AgeRequirement = strptr("All Ages")
		// case 2 leaves it unset — unknown is the common archival case.
	}

	// A flyer on roughly a third of rows, so the with-image and
	// without-image rows appear side by side.
	if archiveNoise(seq+saltFlyer)%3 == 0 {
		show.ImageURL = strptr("/seed-placeholders/show.svg")
	}

	// Mutually exclusive, cancelled winning: a show that was called off is
	// not also "sold out". Only the true case is set — the columns default
	// to false.
	switch {
	case archiveNoise(seq+saltCancelled)%31 == 0:
		show.IsCancelled = true
	case archiveNoise(seq+saltSoldOut)%13 == 0:
		show.IsSoldOut = true
	}
}

// archiveHeadliner returns the act topping show `seq`'s bill. Kept as one
// function so the bill's first slot, the show title, and the slug can never
// disagree about who headlined.
func archiveHeadliner(roster []archiveAct, seq int) archiveAct {
	return roster[archiveBillIndex(seq, 0, len(roster))]
}

// archiveBillIndex picks the roster slot for position `pos` on show `seq`.
// The stride is coprime with the 14-act roster, so the positions on any one
// bill are always DISTINCT acts — an act billed twice on the same show
// would violate the show_artists primary key.
func archiveBillIndex(seq, pos, rosterSize int) int {
	const stride = 3
	return (archiveNoise(seq)%rosterSize + pos*stride) % rosterSize
}

// archiveBill builds the ordered bill for one show, the first act of which
// headlines. Most nights are 1-3 acts, but roughly one in eleven runs 6-8.
//
// Those wide bills are why the roster carries six-word names: the archive
// renders the entire bill inline with no cap, so a fixture of uniformly
// short two-act bills would put every row on one line and prove nothing
// about wrapping.
func archiveBill(roster []archiveAct, seq int) []catalogm.ShowArtist {
	var size int
	if archiveNoise(seq+saltBillWide)%11 == 0 {
		size = 6 + archiveNoise(seq+saltBillSize)%3
	} else {
		size = 1 + archiveNoise(seq+saltBillSize)%3
	}
	if size > len(roster) {
		size = len(roster)
	}

	bill := make([]catalogm.ShowArtist, 0, size)
	for pos := 0; pos < size; pos++ {
		// set_type is CURATED, never inferred from list order (PSY-1673).
		// Headliner is the single inference the vocabulary sanctions, so
		// position 0 gets "headliner" and every other slot the neutral
		// "performer" — asserting "opener" or "direct_support" here would
		// publish a guess as a fact.
		setType := contracts.SetTypeDefault
		if pos == 0 {
			setType = contracts.SetTypeHeadliner
		}
		bill = append(bill, catalogm.ShowArtist{
			ArtistID: roster[archiveBillIndex(seq, pos, len(roster))].ID,
			Position: pos,
			SetType:  setType,
		})
	}
	return bill
}

// createArchiveShowWithBill writes a show, its venue link, and its bill in
// one transaction.
//
// NOT the package's general seed-show helper: it always stamps the
// denormalized show_artists.(event_date, venue_id) columns that the PSY-576
// partial unique index covers, mirroring ShowService.CreateShow's
// syncShowArtistDedupColumns. The other exemplars deliberately leave those
// NULL (the index skips NULL rows, sidestepping the constraint); this
// fixture opts in, because an archive of hundreds of shows at ONE venue is
// exactly the shape that index exists to police, and a fixture that opted
// out would not represent production data. Reuse it elsewhere only if that
// policy is what you want.
//
// One transaction per show, not one for the archive: a failed show logs and
// the seed continues, so a partial archive can be resumed.
func createArchiveShowWithBill(db *gorm.DB, show *catalogm.Show, venueID uint, bill []catalogm.ShowArtist) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(show).Error; err != nil {
			return fmt.Errorf("create show: %w", err)
		}
		if err := tx.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error; err != nil {
			return fmt.Errorf("link venue: %w", err)
		}
		// Index-based so the stamping lands on the slice itself, then one
		// multi-row INSERT rather than one per act.
		for i := range bill {
			bill[i].ShowID = show.ID
			bill[i].EventDate = &show.EventDate
			bill[i].VenueID = &venueID
		}
		if err := tx.Create(&bill).Error; err != nil {
			return fmt.Errorf("link bill: %w", err)
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
// seedExemplarArtistShows already accepts, is that their dates are set on
// first seed and not refreshed. Dispatch stacks are seeded fresh, so in
// practice they are always future.
func seedArchiveUpcomingShows(db *gorm.DB, venueID uint, roster []archiveAct) int {
	dayOffsets := []int{5, 12, 19, 33, 48}
	var created int

	for i, offset := range dayOffsets {
		slug := fmt.Sprintf("chronology-hall-exemplar-upcoming-%d", i+1)

		// Anchored to a venue-local 20:00 on the offset day rather than
		// "now + N days", so upcoming rows show an evening time like the
		// archived ones instead of whatever o'clock the seed ran at.
		local := time.Now().In(archiveVenueZone).AddDate(0, 0, offset)
		eventDate := time.Date(
			local.Year(), local.Month(), local.Day(), 20, 0, 0, 0, archiveVenueZone,
		).UTC()

		if createArchiveShow(db, venueID, roster, eventDate, upcomingSeqBase+i, slug, strptr("https://tickets.example.com/"+slug)) {
			created++
		}
	}
	return created
}
