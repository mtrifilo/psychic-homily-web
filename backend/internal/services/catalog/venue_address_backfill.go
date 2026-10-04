package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/venueaddress"
)

// Venue address backfill: give a venue with an empty address one, so the
// street-geocode columns can place it off its city centroid.
//
// It runs in two steps. Run looks addresses up and writes nothing: phase
// "page" reads the venue's own pages (website, registered ingest source, then
// an upcoming show's ticket page) and geocodes what it finds with the same
// structured search the street-geocode sweep uses; phase "name" searches
// OpenStreetMap for the venue's name in its city and takes the address and
// point of a matched venue-like place, labelled PrecisionNameSearch. Its
// report goes to a person. Apply (venue_address_apply.go) then writes the rows
// of that reviewed report and records its misses, with no lookups of its own,
// so what is written is exactly what was reviewed.
//
// Invariants:
//   - a non-empty address is never overwritten: only venues whose address is
//     empty are selected, and every write is scoped to the row still holding
//     the empty address it was read with (streetGeocodeUpdateScope);
//   - the city-centroid latitude/longitude columns are never written;
//   - a venue whose address was cleared after it had one (by an earlier
//     backfill, or by an editor in its revision history) is never refilled;
//   - a recorded miss is skipped while the key of what was tried still
//     matches and the miss is younger than missMemoTTL;
//   - a row the report marks REVIEW is written only when the reviewer set its
//     approve_review; every row for an unverified venue is REVIEW, because an
//     unverified venue is often a private home and an earlier removal of its
//     address is not reliably visible in its revision history.

// VenueAddressBackfillOptions configures a lookup run.
type VenueAddressBackfillOptions struct {
	// Limit caps the venues that make network calls this run (0 = no limit).
	// Venues skipped on recorded misses do not count against it.
	Limit int
	// OnlyUpcoming restricts the run to venues with an approved, uncancelled
	// show in the next upcomingWindow. Without it those venues still go first.
	OnlyUpcoming bool
	// City restricts the run to one city (case-insensitive exact match).
	City string
	// Now anchors "upcoming" and the miss TTL; zero means time.Now().
	Now time.Time
	// Timeout caps one venue's Nominatim work in each phase, limiter waits
	// included. Zero uses defaultStreetGeocodeBackfillTimeout.
	Timeout time.Duration
}

// upcomingWindow is how far ahead a show makes its venue "upcoming".
const upcomingWindow = 90 * 24 * time.Hour

// missMemoTTL is how long a recorded miss is honoured. Pages are redesigned,
// certificates renewed, and OSM edited; after this a venue is tried again even
// though nothing about it changed.
const missMemoTTL = 90 * 24 * time.Hour

// lookupRulesVersion leads every lookup key. Changing what the phases accept
// (the venue-like place types, name matching, street rules, the prompt) must
// bump it, or venues refused under the old rules stay skipped as misses.
const lookupRulesVersion = "v1"

// maxTicketHosts bounds how many ticket pages, each on a distinct host, are
// offered per venue. Ticket pages are the last page source, and the finder
// reads at most venueaddress.MaxReadTicketPages of them; the spare hosts stand
// in for vendors whose pages turn out to be walled.
const maxTicketHosts = 4

// PageAddressFinder finds a venue's address on its pages;
// *venueaddress.Finder is the production implementation.
type PageAddressFinder interface {
	Find(ctx context.Context, v venueaddress.Venue, sources []venueaddress.Source) (venueaddress.Result, error)
}

// VenueAddressError is the row outcome of a lookup that failed. It is never
// recorded, so the next run tries again.
const VenueAddressError = "error"

// Geocode outcomes of a page-phase hit, which Apply stores beside the
// address: the point, a miss memo, or nothing (the sweep geocodes it later).
const (
	GeocodeHit  = "hit"
	GeocodeMiss = "miss"
	GeocodeNone = "none"
)

// VenueAddressRow is one phase's outcome for one venue. LookupKey, Address,
// Geocode, Precision, and the coordinates are what Apply writes or records;
// the rest is for the reviewer.
type VenueAddressRow struct {
	VenueID   uint       `json:"venue_id"`
	Name      string     `json:"name"`
	City      string     `json:"city"`
	State     string     `json:"state"`
	Verified  bool       `json:"verified"`
	NextShow  *time.Time `json:"next_show,omitempty"`
	Phase     string     `json:"phase"`
	Outcome   string     `json:"outcome"`
	LookupKey string     `json:"lookup_key"`
	Source    string     `json:"source"` // the page URL a page hit came from, the matched OSM place for a name hit, else the lookup key
	Method    string     `json:"method,omitempty"`
	Address   string     `json:"address,omitempty"`
	// LookedUpAddress is the address the lookup produced, which the geocode
	// fields belong to. Apply compares it with Address to tell a reviewer's
	// correction from the original.
	LookedUpAddress string   `json:"looked_up_address,omitempty"`
	MatchedName     string   `json:"matched_name,omitempty"`
	Geocode         string   `json:"geocode,omitempty"`
	Precision       string   `json:"precision,omitempty"`
	Latitude        *float64 `json:"latitude,omitempty"`
	Longitude       *float64 `json:"longitude,omitempty"`
	Notes           []string `json:"notes,omitempty"`
	// Review: the row carries a REVIEW caution. Apply writes it only when the
	// reviewer set ApproveReview, which every generated report leaves false.
	Review        bool `json:"review,omitempty"`
	ApproveReview bool `json:"approve_review"`
	WouldWrite    bool `json:"would_write"`
}

// VenueAddressPhaseTotals counts one phase's outcomes.
type VenueAddressPhaseTotals struct {
	Attempted   int `json:"attempted"`
	Hits        int `json:"hits"`
	Misses      int `json:"misses"`
	Errors      int `json:"errors"`
	SkippedMemo int `json:"skipped_recorded_miss"`
	NoSource    int `json:"no_source,omitempty"`
}

// VenueAddressReport is the structured outcome of a lookup run, and the input
// Apply takes after review.
type VenueAddressReport struct {
	GeneratedAt  time.Time `json:"generated_at"`
	Limit        int       `json:"limit"`
	OnlyUpcoming bool      `json:"only_upcoming"`
	City         string    `json:"city,omitempty"`
	AIEnabled    bool      `json:"ai_enabled"`
	// LookupsTableMissing: the database predates venue_address_lookups, so
	// the run could not skip recorded misses and Apply cannot run yet.
	LookupsTableMissing bool `json:"lookups_table_missing,omitempty"`
	Candidates          int  `json:"candidates"`
	// SkippedCleared counts venues whose address was cleared after they had
	// one; they are never refilled.
	SkippedCleared int                                 `json:"skipped_cleared,omitempty"`
	Processed      int                                 `json:"processed"`
	LimitHit       bool                                `json:"limit_hit"`
	Phases         map[string]*VenueAddressPhaseTotals `json:"phases"`
	Precision      map[string]int                      `json:"precision"`
	WouldWrite     int                                 `json:"would_write"`
	Review         int                                 `json:"review"`
	Rows           []VenueAddressRow                   `json:"rows"`
	Errors         []string                            `json:"errors,omitempty"`
}

// VenueAddressBackfill holds a run's collaborators.
type VenueAddressBackfill struct {
	DB       *gorm.DB
	Pages    PageAddressFinder   // nil skips the page phase
	Places   geo.PlaceSearcher   // nil skips the name phase
	Geocoder geo.AddressGeocoder // geocodes page-phase addresses; nil leaves them for the sweep
	// AIEnabled says whether Pages has an AI fallback; it is part of the page
	// lookup key.
	AIEnabled bool
}

// addressCandidate is a selected venue plus its next upcoming show.
type addressCandidate struct {
	catalogm.Venue
	NextShow *time.Time `gorm:"column:next_show"`
	// ClearedByEditor: the venue's revision history shows its address going
	// from a value to empty. An unverified venue's revision records its old
	// address as withheld rather than as a value, and that counts too.
	ClearedByEditor bool `gorm:"column:cleared_by_editor"`
}

// Run looks up addresses for the selected venues and writes nothing. ctx
// cancellation stops it between venues and returns the partial report with
// ctx.Err().
func (b *VenueAddressBackfill) Run(ctx context.Context, opts VenueAddressBackfillOptions) (*VenueAddressReport, error) {
	if b.DB == nil {
		return nil, errors.New("database not initialized")
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultStreetGeocodeBackfillTimeout
	}
	report := &VenueAddressReport{
		GeneratedAt:  now.UTC(),
		Limit:        opts.Limit,
		OnlyUpcoming: opts.OnlyUpcoming,
		City:         opts.City,
		AIEnabled:    b.AIEnabled,
		Phases: map[string]*VenueAddressPhaseTotals{
			catalogm.VenueAddressPhasePage: {},
			catalogm.VenueAddressPhaseName: {},
		},
		Precision: map[string]int{},
	}

	venues, err := b.loadCandidates(ctx, opts, now, nil)
	if err != nil {
		return nil, err
	}
	report.Candidates = len(venues)
	if len(venues) == 0 {
		return report, nil
	}
	ids := make([]uint, len(venues))
	for i := range venues {
		ids[i] = venues[i].ID
	}
	hasLookups, err := b.lookupsTableExists(ctx)
	if err != nil {
		return nil, err
	}
	report.LookupsTableMissing = !hasLookups
	memos := map[uint]map[string]catalogm.VenueAddressLookup{}
	if hasLookups {
		if memos, err = b.loadMemos(ctx, ids); err != nil {
			return nil, err
		}
	}
	ingestPages, err := b.loadIngestPages(ctx, ids)
	if err != nil {
		return nil, err
	}
	ticketPages, err := b.loadTicketPages(ctx, ids, now, time.Time{})
	if err != nil {
		return nil, err
	}

	pageTotals := report.Phases[catalogm.VenueAddressPhasePage]
	nameTotals := report.Phases[catalogm.VenueAddressPhaseName]
	for i := range venues {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("backfill canceled after %d venues: %w", report.Processed, err)
		}
		c := &venues[i]
		v := &c.Venue

		if c.ClearedByEditor || filledBefore(memos, v.ID) {
			report.SkippedCleared++
			continue
		}
		sources := pageSources(v, ingestPages[v.ID], ticketPages[v.ID])
		pageKey := pageLookupKey(sources, b.AIEnabled)
		placeQuery := venuePlaceQuery(v)
		nameKey := nameLookupKey(placeQuery)

		pageAvailable := b.Pages != nil && len(sources) > 0
		pageMemo := pageAvailable && memoMiss(memos, v.ID, catalogm.VenueAddressPhasePage, pageKey, now)
		nameMemo := b.Places != nil && memoMiss(memos, v.ID, catalogm.VenueAddressPhaseName, nameKey, now)
		runPage := pageAvailable && !pageMemo
		runName := b.Places != nil && !nameMemo
		countSkips := func() {
			if pageMemo {
				pageTotals.SkippedMemo++
			}
			if nameMemo {
				nameTotals.SkippedMemo++
			}
		}
		if !runPage && !runName {
			countSkips()
			continue
		}
		if opts.Limit > 0 && report.Processed >= opts.Limit {
			report.LimitHit = true
			break
		}
		report.Processed++
		countSkips()
		if b.Pages != nil && len(sources) == 0 {
			pageTotals.NoSource++
		}

		pageErrored := false
		if runPage {
			pageTotals.Attempted++
			row := b.runPagePhase(ctx, c, sources, pageKey, timeout, report)
			b.tally(report, pageTotals, row)
			if row.Outcome == catalogm.VenueAddressOutcomeHit {
				continue
			}
			pageErrored = row.Outcome == VenueAddressError
		}
		if runName {
			nameTotals.Attempted++
			row := b.runNamePhase(ctx, c, placeQuery, nameKey, timeout, report)
			if pageErrored && row.Outcome == catalogm.VenueAddressOutcomeHit {
				// The venue's own pages were not decided, so a reviewer must
				// choose between this and waiting for them.
				row.Notes = append(row.Notes, CautionPageUnread)
				row.Review = true
			}
			b.tally(report, nameTotals, row)
		}
	}
	return report, nil
}

// tally appends row to the report and updates the counters it touches.
func (b *VenueAddressBackfill) tally(report *VenueAddressReport, totals *VenueAddressPhaseTotals, row VenueAddressRow) {
	switch row.Outcome {
	case catalogm.VenueAddressOutcomeHit:
		totals.Hits++
	case catalogm.VenueAddressOutcomeMiss:
		totals.Misses++
	case VenueAddressError:
		totals.Errors++
	}
	if row.WouldWrite {
		report.WouldWrite++
		precision := row.Precision
		if precision == "" {
			precision = "none"
		}
		report.Precision[precision]++
	}
	if row.WouldWrite && !row.Verified && !row.Review {
		row.Notes = append(row.Notes, CautionUnverified)
		row.Review = true
	}
	if row.Review {
		report.Review++
	}
	report.Rows = append(report.Rows, row)
}

// runPagePhase looks for the venue's address on its pages and, on a hit,
// geocodes it.
func (b *VenueAddressBackfill) runPagePhase(ctx context.Context, c *addressCandidate, sources []venueaddress.Source, key string, timeout time.Duration, report *VenueAddressReport) VenueAddressRow {
	v := &c.Venue
	row := baseRow(c, catalogm.VenueAddressPhasePage, key)

	res, err := b.Pages.Find(ctx, venueaddress.Venue{Name: v.Name, City: v.City, State: v.State}, sources)
	row.Notes = res.Notes
	if err != nil {
		row.Outcome = VenueAddressError
		report.Errors = append(report.Errors, fmt.Sprintf("venue %d %q page phase: %v", v.ID, v.Name, err))
		return row
	}
	if !res.Found {
		row.Outcome = catalogm.VenueAddressOutcomeMiss
		return row
	}

	row.Outcome = catalogm.VenueAddressOutcomeHit
	row.Source = res.Source.URL
	row.Method = res.Method
	row.Address = res.Street
	row.LookedUpAddress = res.Street
	row.WouldWrite = true
	row.Geocode = GeocodeNone
	if res.City == "" {
		row.Notes = append(row.Notes, "the page printed no city beside the street")
	}
	if b.Geocoder == nil {
		row.Notes = append(row.Notes, "no street geocoder configured; the sweep geocodes the address")
		return row
	}
	updated := *v
	updated.Address = &res.Street
	geoRes, ok, gerr := geocodeWithTimeout(ctx, b.Geocoder, streetGeocodeQuery(&updated), res.Street, timeout)
	switch {
	case gerr != nil:
		row.Notes = append(row.Notes, "street geocode failed; the sweep retries it: "+gerr.Error())
	case !ok:
		row.Geocode = GeocodeMiss
		row.Notes = append(row.Notes, "street geocode found no match; a miss memo is stored with the address")
	default:
		row.Geocode = GeocodeHit
		row.Precision = geoRes.Precision
		row.Latitude, row.Longitude = &geoRes.Latitude, &geoRes.Longitude
	}
	return row
}

// runNamePhase searches OpenStreetMap for the venue's name in its city and
// takes the best candidate the acceptance rules pass: an exact name match
// before a partial one, then Nominatim's order.
func (b *VenueAddressBackfill) runNamePhase(ctx context.Context, c *addressCandidate, q geo.PlaceQuery, key string, timeout time.Duration, report *VenueAddressReport) VenueAddressRow {
	v := &c.Venue
	row := baseRow(c, catalogm.VenueAddressPhaseName, key)

	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	cands, err := b.Places.SearchPlaces(searchCtx, q)
	cancel()
	if err != nil {
		row.Outcome = VenueAddressError
		row.Notes = []string{"search failed: " + err.Error()}
		report.Errors = append(report.Errors, fmt.Sprintf("venue %d %q name phase: %v", v.ID, v.Name, err))
		return row
	}

	var accepted *geo.PlaceCandidate
	var street string
	for i := range cands {
		s, ok, why := AcceptPlaceCandidate(cands[i], v, q.CountryCode)
		if !ok {
			row.Notes = append(row.Notes, fmt.Sprintf("rejected %s=%s %q: %s", cands[i].Category, cands[i].Type, cands[i].Name, why))
			continue
		}
		exact := venueaddress.NamesEqual(cands[i].Name, v.Name)
		if accepted == nil || (exact && !venueaddress.NamesEqual(accepted.Name, v.Name)) {
			accepted, street = &cands[i], s
		}
	}
	if len(cands) == 0 {
		row.Notes = append(row.Notes, "no results")
	}
	if accepted == nil {
		row.Outcome = catalogm.VenueAddressOutcomeMiss
		return row
	}

	row.Outcome = catalogm.VenueAddressOutcomeHit
	row.Source = accepted.DisplayName
	row.Address = street
	row.LookedUpAddress = street
	row.MatchedName = accepted.Name
	row.Geocode = GeocodeHit
	row.Precision = geo.PrecisionNameSearch
	row.Latitude, row.Longitude = &accepted.Latitude, &accepted.Longitude
	row.Notes = append(row.Notes, fmt.Sprintf("matched %s=%s %q (%s)", accepted.Category, accepted.Type, accepted.Name, accepted.DisplayName))
	cautions := placeMatchCautions(*accepted, v)
	row.Notes = append(row.Notes, cautions...)
	row.Review = len(cautions) > 0
	row.WouldWrite = true
	return row
}

func baseRow(c *addressCandidate, phase, key string) VenueAddressRow {
	return VenueAddressRow{
		VenueID:   c.ID,
		Name:      c.Name,
		City:      c.City,
		State:     c.State,
		Verified:  c.Verified,
		NextShow:  c.NextShow,
		Phase:     phase,
		LookupKey: key,
		Source:    key,
	}
}

// venuePlaceQuery is the name search for a venue.
func venuePlaceQuery(v *catalogm.Venue) geo.PlaceQuery {
	return geo.PlaceQuery{
		Name:        v.Name,
		City:        v.City,
		State:       v.State,
		CountryCode: geo.ResolveCountryISO(v.State, derefString(v.Country)),
	}
}

// nameLookupKey is the name phase's lookup key: the rules version and the
// search.
func nameLookupKey(q geo.PlaceQuery) string {
	return lookupRulesVersion + " | " + q.Key()
}

func (b *VenueAddressBackfill) lookupsTableExists(ctx context.Context) (bool, error) {
	var ok bool
	if err := b.DB.WithContext(ctx).Raw("SELECT to_regclass('venue_address_lookups') IS NOT NULL").Scan(&ok).Error; err != nil {
		return false, fmt.Errorf("check for venue_address_lookups: %w", err)
	}
	return ok, nil
}

// loadCandidates selects venues with an empty address, venues with an
// upcoming show first (soonest show first), then by id. ids, when non-nil,
// restricts the selection to those venues.
func (b *VenueAddressBackfill) loadCandidates(ctx context.Context, opts VenueAddressBackfillOptions, now time.Time, ids []uint) ([]addressCandidate, error) {
	q := b.DB.WithContext(ctx).
		Table("venues").
		Select(`venues.*, nxt.next_show, EXISTS (
			SELECT 1 FROM revisions r, jsonb_array_elements(r.field_changes) fc
			WHERE r.entity_type = 'venue' AND r.entity_id = venues.id
			  AND fc->>'field' = 'address'
			  AND (COALESCE(BTRIM(fc->>'old_value'), '') <> ''
			       OR fc->>'old_value_withheld' = 'true')
			  AND COALESCE(BTRIM(fc->>'new_value'), '') = ''
		) AS cleared_by_editor`).
		Joins(`LEFT JOIN LATERAL (
			SELECT MIN(s.event_date) AS next_show
			FROM show_venues sv JOIN shows s ON s.id = sv.show_id
			WHERE sv.venue_id = venues.id AND s.status = 'approved' AND NOT s.is_cancelled
			  AND s.event_date >= ? AND s.event_date < ?
		) nxt ON true`, now, now.Add(upcomingWindow)).
		Where("COALESCE(BTRIM(venues.address), '') = ''")
	if city := strings.TrimSpace(opts.City); city != "" {
		q = q.Where("LOWER(venues.city) = LOWER(?)", city)
	}
	if opts.OnlyUpcoming {
		q = q.Where("nxt.next_show IS NOT NULL")
	}
	if ids != nil {
		q = q.Where("venues.id IN ?", append([]uint{0}, ids...)) // 0 keeps the IN list non-empty
	}
	var out []addressCandidate
	err := q.Order("nxt.next_show IS NULL, nxt.next_show, venues.id").Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("load venues: %w", err)
	}
	return out, nil
}

func (b *VenueAddressBackfill) loadMemos(ctx context.Context, ids []uint) (map[uint]map[string]catalogm.VenueAddressLookup, error) {
	var rows []catalogm.VenueAddressLookup
	if err := b.DB.WithContext(ctx).Where("venue_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load address lookups: %w", err)
	}
	out := map[uint]map[string]catalogm.VenueAddressLookup{}
	for _, r := range rows {
		if out[r.VenueID] == nil {
			out[r.VenueID] = map[string]catalogm.VenueAddressLookup{}
		}
		out[r.VenueID][r.Phase] = r
	}
	return out, nil
}

// filledBefore reports whether an earlier Apply wrote this venue an address.
// A selected venue (address empty) with such a row had it cleared since,
// which is an editor's decision the backfill must not undo.
func filledBefore(memos map[uint]map[string]catalogm.VenueAddressLookup, venueID uint) bool {
	for _, m := range memos[venueID] {
		if m.Outcome == catalogm.VenueAddressOutcomeHit {
			return true
		}
	}
	return false
}

// memoMiss reports whether a miss for this venue, phase, and key was recorded
// within missMemoTTL of now.
func memoMiss(memos map[uint]map[string]catalogm.VenueAddressLookup, venueID uint, phase, key string, now time.Time) bool {
	m, ok := memos[venueID][phase]
	return ok && m.Outcome == catalogm.VenueAddressOutcomeMiss && m.LookupKey == key &&
		now.Sub(m.AttemptedAt) < missMemoTTL
}

func (b *VenueAddressBackfill) loadIngestPages(ctx context.Context, ids []uint) (map[uint]string, error) {
	var rows []adminm.SourceConfig
	err := b.DB.WithContext(ctx).
		Where("entity_type = ? AND entity_id IN ? AND source_url IS NOT NULL", "venue", ids).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load source configs: %w", err)
	}
	out := map[uint]string{}
	for _, r := range rows {
		out[r.EntityID] = strings.TrimSpace(*r.SourceURL)
	}
	return out, nil
}

// ticketPagesPerVenue bounds the ticket URLs read per venue: pageSources keeps
// at most maxTicketHosts distinct hosts, and a venue's soonest shows rarely
// span more vendors than this many shows.
const ticketPagesPerVenue = 10

// loadTicketPages returns, per venue, the ticket URLs of its soonest approved,
// uncancelled shows on or after asOf, soonest first, counting only shows an
// admin or a trusted-tier user (authm.TrustedTiers) submitted, or the
// discovery import created. Any signed-in user can submit an approved show
// with any ticket URL, so an untrusted show's ticket page would let its
// submitter choose a venue's address. A non-zero createdBefore leaves out
// shows created after it, so Apply can rebuild the sources a report saw.
func (b *VenueAddressBackfill) loadTicketPages(ctx context.Context, ids []uint, asOf, createdBefore time.Time) (map[uint][]string, error) {
	created := "TRUE"
	args := []interface{}{ids, asOf, catalogm.ShowSourceDiscovery, authm.TrustedTiers}
	if !createdBefore.IsZero() {
		created = "s.created_at <= ?"
		args = append(args, createdBefore)
	}
	args = append(args, ticketPagesPerVenue)
	var rows []struct {
		VenueID   uint
		TicketURL string
	}
	err := b.DB.WithContext(ctx).Raw(`
		SELECT venue_id, ticket_url FROM (
			SELECT sv.venue_id, BTRIM(s.ticket_url) AS ticket_url, s.event_date, s.id,
			       ROW_NUMBER() OVER (PARTITION BY sv.venue_id ORDER BY s.event_date, s.id) AS n
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			LEFT JOIN users u ON u.id = s.submitted_by
			WHERE sv.venue_id IN ? AND s.status = 'approved' AND NOT s.is_cancelled
			  AND s.event_date >= ? AND COALESCE(BTRIM(s.ticket_url), '') <> ''
			  AND (s.source = ? OR u.is_admin OR u.user_tier IN ?)
			  AND `+created+`
		) t
		WHERE n <= ?
		ORDER BY venue_id, event_date, id`, args...).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load ticket pages: %w", err)
	}
	out := map[uint][]string{}
	for _, r := range rows {
		out[r.VenueID] = append(out[r.VenueID], r.TicketURL)
	}
	return out, nil
}

// pageSources orders a venue's pages: its website, its registered ingest
// source, then up to maxTicketHosts ticket pages on distinct hosts. A
// URL that does not parse, or whose host is login-walled, is left out, as is a
// repeat of an earlier URL.
func pageSources(v *catalogm.Venue, ingestPage string, ticketPages []string) []venueaddress.Source {
	var out []venueaddress.Source
	seen := map[string]bool{}
	// add returns the lowercased host of the URL it added, or "" when it
	// left the URL out.
	add := func(raw string, kind venueaddress.SourceKind) string {
		raw = strings.TrimSpace(raw)
		u, err := url.Parse(raw)
		if raw == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return ""
		}
		if venueaddress.LoginWalled(u.Hostname()) || seen[raw] {
			return ""
		}
		seen[raw] = true
		out = append(out, venueaddress.Source{URL: raw, Kind: kind})
		return strings.ToLower(u.Hostname())
	}
	add(derefString(v.Social.Website), venueaddress.SourceWebsite)
	add(ingestPage, venueaddress.SourceIngest)
	ticketHosts := map[string]bool{}
	for _, t := range ticketPages {
		if len(ticketHosts) >= maxTicketHosts {
			break
		}
		if u, err := url.Parse(strings.TrimSpace(t)); err != nil || ticketHosts[strings.ToLower(u.Hostname())] {
			continue
		}
		if host := add(t, venueaddress.SourceTicket); host != "" {
			ticketHosts[host] = true
		}
	}
	return out
}

// pageLookupKey names what the page phase tries: the rules version, the
// website and ingest source URLs, the sorted HOSTS of the ticket pages (an
// upcoming show's per-event URL changes every week and the soonest vendor
// rotates; the set of vendors does not), and whether the AI fallback was on.
// A changed website, a new ticket vendor, or turning the AI fallback on
// changes the key, which makes a recorded miss stale.
func pageLookupKey(sources []venueaddress.Source, aiEnabled bool) string {
	parts := []string{lookupRulesVersion}
	var tickets []string
	for _, s := range sources {
		if s.Kind == venueaddress.SourceTicket {
			if u, err := url.Parse(s.URL); err == nil {
				tickets = append(tickets, "ticket:"+strings.ToLower(u.Hostname()))
				continue
			}
		}
		parts = append(parts, s.URL)
	}
	sort.Strings(tickets)
	parts = append(parts, tickets...)
	if !aiEnabled {
		parts = append(parts, "[schema-only]")
	}
	return strings.Join(parts, " | ")
}

// recordAddressLookup upserts the venue's lookup row for one phase, dated
// attemptedAt (zero means now). A miss never replaces a recorded hit: the hit
// is the record that the backfill filled the venue, which keeps a later clear
// from being refilled.
func recordAddressLookup(db *gorm.DB, venueID uint, phase, key, outcome, source, address string, attemptedAt time.Time) error {
	if attemptedAt.IsZero() {
		attemptedAt = time.Now()
	}
	row := catalogm.VenueAddressLookup{
		VenueID:     venueID,
		Phase:       phase,
		LookupKey:   key,
		Outcome:     outcome,
		AttemptedAt: attemptedAt.UTC(),
	}
	if source != "" {
		row.Source = &source
	}
	if address != "" {
		row.Address = &address
	}
	conflict := clause.OnConflict{
		Columns:   []clause.Column{{Name: "venue_id"}, {Name: "phase"}},
		DoUpdates: clause.AssignmentColumns([]string{"lookup_key", "outcome", "source", "address", "attempted_at"}),
	}
	if outcome == catalogm.VenueAddressOutcomeMiss {
		conflict.Where = clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "venue_address_lookups.outcome <> ?", Vars: []interface{}{catalogm.VenueAddressOutcomeHit}},
		}}
	}
	return db.Clauses(conflict).Create(&row).Error
}
