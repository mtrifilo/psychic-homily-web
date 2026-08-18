package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"psychic-homily-backend/internal/config"
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
// Everything here is downstream of ONE fact: the seed's idempotency key is
// each show's generated SLUG, and that slug embeds the show's date and its
// headliner's name. Anything that changes either re-creates the archive on
// the next run instead of skipping it, silently doubling it. Hence:
//
//   - Past dates are FIXED calendar years, not offsets from time.Now(),
//     which would regenerate every slug daily.
//
//   - A show's identity is archiveSeq(year, month, index) — its calendar
//     position — not a running counter, so editing one month's count does
//     not renumber (and re-slug) every later show.
//
//   - Per-show variation is a pure hash (archiveNoise), not math/rand,
//     whose sequence is not guaranteed stable across Go releases. A golden
//     test pins the hash so a change to it is deliberate.
//
//   - The bill roster is all-or-nothing: headliners are chosen modulo the
//     roster length, so a partially-created roster would re-slug the whole
//     archive.
//
// Upcoming shows cannot use fixed dates, so they are keyed by fixed
// non-date slugs and RE-STAMPED with a fresh date on every seed, which is
// what stops them drifting into the past and disturbing the archive.
//
// Note that cmd/seed is not dev-only — see archiveExemplarEnabled.

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
// The salt is MIXED INTO the hash, not added to the input. An additive
// salt would only shift one sequence along itself, so two choices whose
// salts differ by k and share a modulus would track each other exactly k
// shows apart — visible as a repeating stripe down a 50-row page, which is
// the very thing these salts exist to prevent.
//
// The asymmetry worth knowing before changing any of these: saltHeadliner,
// saltBillSize, and saltBillWide feed the show's SLUG (through the
// headliner's name), so changing them re-creates the archive on the next
// seed instead of skipping it. The decoration salts only reshuffle prices
// and badges and are free to change.
const (
	saltHeadliner   = 1
	saltBillWide    = 3
	saltBillSize    = 5
	saltStride      = 7
	saltPriceAbsent = 11
	saltDoorTime    = 17
	saltPrice       = 23
	saltAge         = 37
	saltFlyer       = 53
	saltCancelled   = 71
	saltSoldOut     = 97
)

// archiveNoise is a deterministic integer hash (a splitmix-style bit mix)
// used wherever a show needs a stable arbitrary choice. Same inputs, same
// output, on every machine and every Go release — which is what keeps the
// generated slugs stable and therefore the seed idempotent. Always
// non-negative so callers can use % directly.
func archiveNoise(n, salt int) int {
	x := uint32(n)*2654435761 ^ uint32(salt)*2246822519
	x ^= x >> 15
	x *= 2654435761
	x ^= x >> 13
	x *= 1274126177
	x ^= x >> 16
	return int(x & 0x7fffffff)
}

// archiveSeq is a show's stable identity, derived from WHERE it sits in the
// calendar rather than from how many shows were generated before it.
//
// This is load-bearing for idempotency. Every per-show choice keys off it,
// including the headliner — whose name goes into the slug that guards
// against re-creation. A running counter would make each show's identity
// depend on the counts of every earlier month, so adding one show to an
// early month would renumber every later show, change all their slugs, and
// make the next seed insert a SECOND copy of the rest of the archive
// instead of skipping it. Keyed by (year, month, index), an edit to one
// month stays local to that month.
func archiveSeq(year, month, index int) int {
	return year*10000 + month*100 + index
}

// upcomingSeq numbers the upcoming shows in a range archiveSeq cannot
// produce (its values are year-scaled, so ~20,000,000+).
func upcomingSeq(i int) int { return 1000 + i }

// archiveExemplarEnabled reports whether this fixture should be seeded.
//
// cmd/seed is NOT dev-only: backend/scripts/deploy-stage.sh runs it on
// every stage deploy to install reference data, and psy-deploy-prod's
// --with-db-restore copies stage's catalog into production. The other
// exemplars predate that discovery and add ~10 rows; this one adds a
// verified venue with hundreds of approved shows and its own artists,
// which on stage would outrank every real venue in the graph, the scene
// rollups, and the sitemap — and could reach the public site through a
// restore.
//
// So it opts OUT of deployed environments rather than opting in to
// development: an unset or unrecognised ENVIRONMENT still seeds, which
// keeps every existing local workflow working (including the documented
// dispatch-stack command), while the two environments that are definitely
// not somebody's laptop are excluded.
func archiveExemplarEnabled() bool {
	switch os.Getenv(config.EnvEnvironment) {
	case config.EnvProduction, config.EnvStage:
		return false
	default:
		return true
	}
}

// seedExemplarArchiveVenue creates the archive exemplar venue itself.
// Returns its ID, or 0 if it was skipped or could not be created.
func seedExemplarArchiveVenue(db *gorm.DB, userID uint) uint {
	if !archiveExemplarEnabled() {
		fmt.Printf("  ⏭️  archive exemplar: skipped (ENVIRONMENT=%s)\n", os.Getenv(config.EnvEnvironment))
		return 0
	}

	var existing catalogm.Venue
	switch err := db.Where("slug = ?", exemplarArchiveVenueSlug).First(&existing).Error; {
	case err == nil:
		return existing.ID
	case !errors.Is(err, gorm.ErrRecordNotFound):
		log.Printf("Warning: failed to probe archive exemplar venue: %v", err)
		return 0
	}

	venue := &catalogm.Venue{
		Name: exemplarArchiveVenueName,
		Slug: strptr(exemplarArchiveVenueSlug),
		// Verified is true below, and Venue.PublicAddress only discloses the
		// address for verified venues — so this string is published. It is
		// therefore a deliberately NON-EXISTENT street: attaching a real
		// Phoenix address to a venue that does not exist would put a real
		// occupant's location on a fictional listing.
		Address: strptr("0000 W Nonesuch Way"),
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
	// A deliberate environment skip already reported itself; say nothing
	// more. Only a MISSING venue in an enabled environment is a warning.
	if !archiveExemplarEnabled() {
		return
	}
	if venueID == 0 {
		log.Printf("Warning: archive exemplar venue missing; skipping its %d shows", archiveTotalShows())
		return
	}

	roster := findOrCreateArchiveBillActs(db)
	// All-or-nothing, deliberately. Every show's headliner is chosen modulo
	// the roster length, so a roster short by even one act would pick
	// different headliners, hence different slugs, hence a SECOND copy of
	// the whole archive on the next seed. A partial fixture is not worth
	// that failure mode.
	if len(roster) != len(archiveBillArtists) {
		log.Printf("Warning: resolved only %d of %d archive bill acts; skipping archive shows rather than seeding a roster that would break idempotency",
			len(roster), len(archiveBillArtists))
		return
	}

	past := seedArchivePastShows(db, venueID, roster)
	upcoming := seedArchiveUpcomingShows(db, venueID, roster)
	total := past.add(upcoming)

	// "created == 0" alone is ambiguous — it means either "already fully
	// seeded" or "every single show failed". Report the three outcomes
	// separately so a failed seed can never print a checkmark.
	fmt.Printf("  ✅ archive exemplar: %d created, %d already present\n", total.created, total.skipped)
	if total.failed > 0 {
		log.Printf("⚠️  archive exemplar: %d shows FAILED to create; the archive is incomplete (see warnings above)", total.failed)
	}
}

// seedOutcome counts the three ways a show can end up after a seed pass.
type seedOutcome struct {
	created int
	skipped int
	failed  int
}

func (o seedOutcome) add(other seedOutcome) seedOutcome {
	return seedOutcome{
		created: o.created + other.created,
		skipped: o.skipped + other.skipped,
		failed:  o.failed + other.failed,
	}
}

// archiveTotalShows is the number of past shows the fixture describes, used
// in operator-facing messages.
func archiveTotalShows() int {
	var total int
	for _, ay := range archiveYears {
		for _, count := range ay.months {
			total += count
		}
	}
	return total
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
func seedArchivePastShows(db *gorm.DB, venueID uint, roster []archiveAct) seedOutcome {
	var out seedOutcome

	for _, ay := range archiveYears {
		for monthIdx, count := range ay.months {
			month := monthIdx + 1
			for i := 0; i < count; i++ {
				seq := archiveSeq(ay.year, month, i)
				clock := archiveDoorTimes[archiveNoise(seq, saltDoorTime)%len(archiveDoorTimes)]
				eventDate := time.Date(
					ay.year, time.Month(month), archiveDayOfMonth(i, count),
					clock.hour, clock.minute, 0, 0, archiveVenueZone,
				).UTC()

				headliner := archiveHeadliner(roster, seq)
				slug := utils.GenerateShowSlug(eventDate, headliner.Name, exemplarArchiveVenueName, exemplarArchiveVenueState)

				out = out.add(createArchiveShow(db, venueID, roster, eventDate, seq, slug, nil))
			}
		}
	}
	return out
}

// archiveDayOfMonth spreads `count` shows across the first 28 days of a
// month, returning the day for the i-th one. Capping at 28 keeps February
// valid without a per-month length table, and the spacing guarantees
// DISTINCT days for any count up to 28: the step 28/count is then >= 1, so
// the truncating division never rounds two indexes onto the same day.
//
// Distinct days are required by the SHOW SLUG, not by the show_artists
// dedup index. utils.GenerateShowSlug formats the date as 2006-01-02 and
// drops the time, so two shows on one calendar day with the same headliner
// generate the same slug and collide on the shows unique index — while the
// show_artists partial index, which is on the raw timestamp, would not
// fire for two shows at different times. Anyone adding early/late sets on
// one night (which this venue's own description advertises) has to make
// the slug unique, not just the instant.
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
func createArchiveShow(
	db *gorm.DB,
	venueID uint,
	roster []archiveAct,
	eventDate time.Time,
	seq int,
	slug string,
	ticketURL *string,
) seedOutcome {
	// A probe error is NOT "not found": treating a dropped connection as
	// absence would send us into a create that then trips the slug unique
	// index, reporting a confusing conflict instead of the real fault.
	var existing catalogm.Show
	switch err := db.Where("slug = ?", slug).First(&existing).Error; {
	case err == nil:
		return seedOutcome{skipped: 1}
	case !errors.Is(err, gorm.ErrRecordNotFound):
		log.Printf("Warning: failed to probe archive show %s: %v", slug, err)
		return seedOutcome{failed: 1}
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
	// A cancelled show does not sell tickets; leaving a live ticket link on
	// one would read as a UI bug rather than fixture variety.
	if show.IsCancelled {
		show.TicketURL = nil
	}

	if err := createArchiveShowWithBill(db, show, venueID, archiveBill(roster, seq)); err != nil {
		log.Printf("Warning: failed to create archive show %s: %v", slug, err)
		return seedOutcome{failed: 1}
	}
	return seedOutcome{created: 1}
}

// decorateArchiveShow applies the deterministic per-show variation: price
// (absent for a minority), age requirement, flyer, and the badges.
//
// Applies to the UPCOMING shows as well as the archived ones — a sold-out
// or cancelled show in the Upcoming section is realistic and worth being
// able to see, so the badges are deliberately not scoped to the archive.
func decorateArchiveShow(show *catalogm.Show, seq int) {
	// ~1 in 7 shows has no price, so the row's price-absent branch stays
	// exercised.
	if archiveNoise(seq, saltPriceAbsent)%7 != 0 {
		price := archivePrices[archiveNoise(seq, saltPrice)%len(archivePrices)]
		show.Price = &price
	}

	switch archiveNoise(seq, saltAge) % 3 {
	case 0:
		show.AgeRequirement = strptr("21+")
	case 1:
		show.AgeRequirement = strptr("All Ages")
		// case 2 leaves it unset — unknown is the common archival case.
	}

	// A flyer on roughly a third of rows, so the with-image and
	// without-image rows appear side by side.
	if archiveNoise(seq, saltFlyer)%3 == 0 {
		show.ImageURL = strptr("/seed-placeholders/show.svg")
	}

	// Mutually exclusive, cancelled winning: a show that was called off is
	// not also "sold out". Only the true case is set — the columns default
	// to false.
	switch {
	case archiveNoise(seq, saltCancelled)%31 == 0:
		show.IsCancelled = true
	case archiveNoise(seq, saltSoldOut)%13 == 0:
		show.IsSoldOut = true
	}
}

// archiveHeadliner returns the act topping show `seq`'s bill. Kept as one
// function so the bill's first slot, the show title, and the slug can never
// disagree about who headlined.
func archiveHeadliner(roster []archiveAct, seq int) archiveAct {
	return roster[archiveBillIndex(seq, 0, len(roster))]
}

// archiveBillIndex picks the roster slot for position `pos` on show `seq`,
// walking the roster by a stride.
//
// The stride must be COPRIME with the roster size or the walk revisits a
// slot, billing the same act twice on one show — a show_artists
// primary-key violation that rolls the whole show back. Rather than assert
// that as a property of the 14-act roster (it is really a property of
// whatever length is passed), archiveStride computes a coprime stride for
// the size actually in play, so distinctness holds by construction.
func archiveBillIndex(seq, pos, rosterSize int) int {
	return (archiveNoise(seq, saltHeadliner)%rosterSize + pos*archiveStride(rosterSize, seq)) % rosterSize
}

// archiveStride returns a step coprime with rosterSize, chosen per show.
//
// Coprimality is the correctness half: a stride sharing a factor with the
// roster size revisits a slot, billing one act twice on a show — a
// show_artists primary-key violation. Varying it per show is the realism
// half: with a single fixed stride the whole bill is a function of the
// headliner alone, so every night the same act headlined it drew the exact
// same support, which reads as cloned rows on a page this fixture exists to
// have looked at.
//
// 1 is coprime with everything, so a stride always exists.
func archiveStride(rosterSize, seq int) int {
	candidates := make([]int, 0, 5)
	for _, stride := range []int{3, 5, 9, 11, 13} {
		if gcd(stride, rosterSize) == 1 {
			candidates = append(candidates, stride)
		}
	}
	if len(candidates) == 0 {
		return 1
	}
	return candidates[archiveNoise(seq, saltStride)%len(candidates)]
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
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
	if archiveNoise(seq, saltBillWide)%11 == 0 {
		size = 6 + archiveNoise(seq, saltBillSize)%3
	} else {
		size = 1 + archiveNoise(seq, saltBillSize)%3
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

// seedArchiveUpcomingShows keeps a few future shows on the venue so the
// Upcoming/Past split has both halves.
//
// These are the one part of the fixture that cannot use fixed dates — a
// hardcoded future date stops being future — so they are keyed by a fixed
// NON-date slug and dated relative to now.
//
// They are REFRESHED rather than skipped when they already exist. Skipping
// (what seedExemplarArtistShows does) freezes their dates at first seed, so
// on any database that outlives the longest offset they drift into the
// past: the Upcoming section empties out AND each drifted show joins the
// past archive, adding a stray year to the year strip and shifting every
// page boundary the README documents. Re-stamping the date on each seed
// keeps the fixture at exactly len(dayOffsets) upcoming and archiveYears'
// worth of past, indefinitely.
func seedArchiveUpcomingShows(db *gorm.DB, venueID uint, roster []archiveAct) seedOutcome {
	dayOffsets := []int{5, 12, 19, 33, 48}
	var out seedOutcome

	for i, offset := range dayOffsets {
		slug := fmt.Sprintf("chronology-hall-exemplar-upcoming-%d", i+1)

		// Anchored to a venue-local 20:00 on the offset day rather than
		// "now + N days", so upcoming rows show an evening time like the
		// archived ones instead of whatever o'clock the seed ran at.
		local := time.Now().In(archiveVenueZone).AddDate(0, 0, offset)
		eventDate := time.Date(
			local.Year(), local.Month(), local.Day(), 20, 0, 0, 0, archiveVenueZone,
		).UTC()

		var existing catalogm.Show
		switch err := db.Where("slug = ?", slug).First(&existing).Error; {
		case err == nil:
			if err := refreshUpcomingShowDate(db, existing.ID, eventDate); err != nil {
				log.Printf("Warning: failed to refresh upcoming show %s: %v", slug, err)
				out = out.add(seedOutcome{failed: 1})
				continue
			}
			out = out.add(seedOutcome{skipped: 1})
			continue
		case !errors.Is(err, gorm.ErrRecordNotFound):
			log.Printf("Warning: failed to probe upcoming show %s: %v", slug, err)
			out = out.add(seedOutcome{failed: 1})
			continue
		}

		out = out.add(createArchiveShow(
			db, venueID, roster, eventDate, upcomingSeq(i), slug,
			strptr("https://tickets.example.com/"+slug),
		))
	}
	return out
}

// refreshUpcomingShowDate moves an existing upcoming show to a new date.
//
// show_artists carries a denormalized copy of event_date (the PSY-576
// dedup columns), so both tables must move together or the show's bill
// rows would keep pointing at the old instant — the same pairing
// ShowService.UpdateShow maintains when a show is rescheduled.
func refreshUpcomingShowDate(db *gorm.DB, showID uint, eventDate time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&catalogm.Show{}).Where("id = ?", showID).
			Update("event_date", eventDate).Error; err != nil {
			return fmt.Errorf("update show date: %w", err)
		}
		if err := tx.Model(&catalogm.ShowArtist{}).Where("show_id = ?", showID).
			Update("event_date", eventDate).Error; err != nil {
			return fmt.Errorf("update bill dedup date: %w", err)
		}
		return nil
	})
}
