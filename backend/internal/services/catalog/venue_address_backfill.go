package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/venueaddress"
)

// Venue address backfill: give a venue with an empty address one, so the
// street-geocode columns can place it off its city centroid.
//
// Phase "page" reads the venue's own pages (website, registered ingest source,
// then an upcoming show's ticket page) for a published address and geocodes it
// with the same structured search the street-geocode sweep uses. Phase "name"
// runs only for a venue still without an address: it searches OpenStreetMap
// for the venue's name in its city and takes the address and point of a
// matched venue-like place, labelled PrecisionNameSearch.
//
// Invariants:
//   - a non-empty address is never overwritten: only venues whose address is
//     empty are selected, and every write is scoped to the row still holding
//     the empty address it was read with (streetGeocodeUpdateScope);
//   - the city-centroid latitude/longitude columns are never written;
//   - a dry run performs the same lookups and writes nothing, misses included;
//   - a miss is recorded per phase under the key of what was tried, so a later
//     run skips it until the venue's inputs change; an error is never recorded.

// VenueAddressBackfillOptions configures a run.
type VenueAddressBackfillOptions struct {
	DryRun bool
	// Limit caps the venues that make network calls this run (0 = no limit).
	// Venues skipped on recorded misses do not count against it.
	Limit int
	// OnlyUpcoming restricts the run to venues with an approved, uncancelled
	// show in the next upcomingWindow. Without it those venues still go first.
	OnlyUpcoming bool
	// City restricts the run to one city (case-insensitive exact match).
	City string
	// Now anchors "upcoming"; zero means time.Now().
	Now time.Time
	// Timeout caps one venue's Nominatim work in each phase, limiter waits
	// included. Zero uses defaultStreetGeocodeBackfillTimeout.
	Timeout time.Duration
}

// upcomingWindow is how far ahead a show makes its venue "upcoming".
const upcomingWindow = 90 * 24 * time.Hour

// maxTicketPageSources bounds how many ticket pages, each on a distinct host,
// are tried per venue. Ticket pages are the last page source.
const maxTicketPageSources = 2

// PageAddressFinder finds a venue's address on its pages;
// *venueaddress.Finder is the production implementation.
type PageAddressFinder interface {
	Find(ctx context.Context, v venueaddress.Venue, sources []venueaddress.Source) (venueaddress.Result, error)
}

// Backfill row outcomes: catalogm.VenueAddressOutcomeHit or ...Miss, or this
// one, which is never recorded and is retried next run.
const VenueAddressError = "error"

// VenueAddressRow is one phase's outcome for one venue.
type VenueAddressRow struct {
	VenueID     uint       `json:"venue_id"`
	Name        string     `json:"name"`
	City        string     `json:"city"`
	State       string     `json:"state"`
	Verified    bool       `json:"verified"`
	NextShow    *time.Time `json:"next_show,omitempty"`
	Phase       string     `json:"phase"`
	Outcome     string     `json:"outcome"`
	Source      string     `json:"source"` // page URL, or the Nominatim query text
	Method      string     `json:"method,omitempty"`
	Address     string     `json:"address,omitempty"`
	MatchedName string     `json:"matched_name,omitempty"`
	Precision   string     `json:"precision,omitempty"`
	Latitude    *float64   `json:"latitude,omitempty"`
	Longitude   *float64   `json:"longitude,omitempty"`
	Notes       []string   `json:"notes,omitempty"`
	WouldWrite  bool       `json:"would_write"`
	Written     bool       `json:"written"`
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

// VenueAddressReport is the structured outcome of a run.
type VenueAddressReport struct {
	GeneratedAt  time.Time `json:"generated_at"`
	DryRun       bool      `json:"dry_run"`
	Limit        int       `json:"limit"`
	OnlyUpcoming bool      `json:"only_upcoming"`
	City         string    `json:"city,omitempty"`
	AIEnabled    bool      `json:"ai_enabled"`
	// LookupsTableMissing: the database predates venue_address_lookups, so
	// this (dry) run could not skip recorded misses.
	LookupsTableMissing bool                                `json:"lookups_table_missing,omitempty"`
	Candidates          int                                 `json:"candidates"`
	Processed           int                                 `json:"processed"`
	LimitHit            bool                                `json:"limit_hit"`
	Phases              map[string]*VenueAddressPhaseTotals `json:"phases"`
	Precision           map[string]int                      `json:"precision"`
	WouldWrite          int                                 `json:"would_write"`
	Written             int                                 `json:"written"`
	Rows                []VenueAddressRow                   `json:"rows"`
	Errors              []string                            `json:"errors,omitempty"`
}

// VenueAddressBackfill holds a run's collaborators.
type VenueAddressBackfill struct {
	DB       *gorm.DB
	Pages    PageAddressFinder   // nil skips the page phase
	Places   geo.PlaceSearcher   // nil skips the name phase
	Geocoder geo.AddressGeocoder // geocodes page-phase addresses; nil leaves them for the sweep
	// AIEnabled is reported only; it says whether Pages has an AI fallback.
	AIEnabled bool
}

// addressCandidate is a selected venue plus its next upcoming show.
type addressCandidate struct {
	catalogm.Venue
	NextShow *time.Time `gorm:"column:next_show"`
}

// Run executes the backfill. ctx cancellation stops it between venues and
// returns the partial report with ctx.Err().
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
		DryRun:       opts.DryRun,
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

	venues, err := b.loadCandidates(ctx, opts, now)
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
	// A database that predates the lookups table can still be dry-run (there
	// are no recorded misses to honour yet); a live run needs the table and
	// the widened precision constraint, so it refuses to start without them.
	var hasLookups bool
	if err := b.DB.WithContext(ctx).Raw("SELECT to_regclass('venue_address_lookups') IS NOT NULL").Scan(&hasLookups).Error; err != nil {
		return nil, fmt.Errorf("check for venue_address_lookups: %w", err)
	}
	if !hasLookups && !opts.DryRun {
		return nil, errors.New("venue_address_lookups does not exist: apply the database migrations before a live run")
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
	ticketPages, err := b.loadTicketPages(ctx, ids, now)
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

		sources := pageSources(v, ingestPages[v.ID], ticketPages[v.ID])
		pageKey := pageLookupKey(sources)
		placeQuery := geo.PlaceQuery{
			Name:        v.Name,
			City:        v.City,
			State:       v.State,
			CountryCode: geo.ResolveCountryISO(v.State, derefString(v.Country)),
		}
		nameKey := placeQuery.Key()

		pageAvailable := b.Pages != nil && len(sources) > 0
		pageMemo := pageAvailable && memoMiss(memos, v.ID, catalogm.VenueAddressPhasePage, pageKey)
		nameMemo := b.Places != nil && memoMiss(memos, v.ID, catalogm.VenueAddressPhaseName, nameKey)
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

		if runPage {
			pageTotals.Attempted++
			row, found := b.runPagePhase(ctx, c, sources, pageKey, timeout, opts.DryRun, report)
			b.tally(report, pageTotals, row)
			if found {
				continue
			}
		}
		if runName {
			nameTotals.Attempted++
			row := b.runNamePhase(ctx, c, placeQuery, nameKey, timeout, opts.DryRun, report)
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
	if row.Written {
		report.Written++
	}
	report.Rows = append(report.Rows, row)
}

// runPagePhase looks for the venue's address on its pages and, on a hit,
// geocodes it. found reports whether the venue now has (or in a dry run would
// have) an address, which ends its run.
func (b *VenueAddressBackfill) runPagePhase(ctx context.Context, c *addressCandidate, sources []venueaddress.Source, key string, timeout time.Duration, dryRun bool, report *VenueAddressReport) (VenueAddressRow, bool) {
	v := &c.Venue
	row := baseRow(c, catalogm.VenueAddressPhasePage)
	row.Source = key

	res, err := b.Pages.Find(ctx, venueaddress.Venue{Name: v.Name, City: v.City, State: v.State}, sources)
	row.Notes = res.Notes
	if err != nil {
		row.Outcome = VenueAddressError
		report.Errors = append(report.Errors, fmt.Sprintf("venue %d %q page phase: %v", v.ID, v.Name, err))
		return row, false
	}
	if !res.Found {
		row.Outcome = catalogm.VenueAddressOutcomeMiss
		if !dryRun {
			b.recordMiss(report, v, catalogm.VenueAddressPhasePage, key)
		}
		return row, false
	}

	row.Outcome = catalogm.VenueAddressOutcomeHit
	row.Source = res.Source.URL
	row.Method = res.Method
	row.Address = res.Street
	row.WouldWrite = true
	if res.City == "" {
		row.Notes = append(row.Notes, "the page printed no city beside the street")
	}

	// Geocode the address exactly as the sweep would once it is stored, and
	// store the outcome the way the sweep does: a hit, a miss memo, or (on an
	// error) nothing, so the sweep retries.
	updated := *v
	updated.Address = &res.Street
	q := streetGeocodeQuery(&updated)
	updates := map[string]interface{}{}
	if b.Geocoder == nil {
		row.Notes = append(row.Notes, "no street geocoder configured; the sweep geocodes the address")
	} else {
		geoRes, ok, gerr := geocodeWithTimeout(ctx, b.Geocoder, q, res.Street, timeout)
		switch {
		case gerr != nil:
			row.Notes = append(row.Notes, "street geocode failed; the sweep retries it: "+gerr.Error())
		case !ok:
			row.Notes = append(row.Notes, "street geocode found no match; a miss memo is stored with the address")
			updates = streetGeocodeMissColumns(q.Key())
		default:
			row.Precision = geoRes.Precision
			row.Latitude, row.Longitude = &geoRes.Latitude, &geoRes.Longitude
			updates = streetGeocodeHitColumns(geoRes, q.Key())
		}
	}
	if !dryRun {
		b.writeAddress(report, &row, v, res.Street, updates, catalogm.VenueAddressPhasePage, key, res.Source.URL)
	}
	return row, true
}

// runNamePhase searches OpenStreetMap for the venue's name in its city and
// takes the first candidate the acceptance rules pass. A hit stores the
// matched place's point with the name-search precision, keyed to the new
// address so the street-geocode sweep treats it as already attempted.
func (b *VenueAddressBackfill) runNamePhase(ctx context.Context, c *addressCandidate, q geo.PlaceQuery, key string, timeout time.Duration, dryRun bool, report *VenueAddressReport) VenueAddressRow {
	v := &c.Venue
	row := baseRow(c, catalogm.VenueAddressPhaseName)
	row.Source = key

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
		if ok {
			accepted, street = &cands[i], s
			break
		}
		row.Notes = append(row.Notes, fmt.Sprintf("rejected %s=%s %q: %s", cands[i].Category, cands[i].Type, cands[i].Name, why))
	}
	if len(cands) == 0 {
		row.Notes = append(row.Notes, "no results")
	}
	if accepted == nil {
		row.Outcome = catalogm.VenueAddressOutcomeMiss
		if !dryRun {
			b.recordMiss(report, v, catalogm.VenueAddressPhaseName, key)
		}
		return row
	}

	row.Outcome = catalogm.VenueAddressOutcomeHit
	row.Address = street
	row.MatchedName = accepted.Name
	row.Precision = geo.PrecisionNameSearch
	row.Latitude, row.Longitude = &accepted.Latitude, &accepted.Longitude
	row.Notes = append(row.Notes, fmt.Sprintf("matched %s=%s %q (%s)", accepted.Category, accepted.Type, accepted.Name, accepted.DisplayName))
	row.Notes = append(row.Notes, placeMatchCautions(*accepted, v)...)
	row.WouldWrite = true

	if !dryRun {
		updated := *v
		updated.Address = &street
		updates := streetGeocodeHitColumns(geo.AddressResult{
			Latitude: accepted.Latitude, Longitude: accepted.Longitude, Precision: geo.PrecisionNameSearch,
		}, streetGeocodeQuery(&updated).Key())
		b.writeAddress(report, &row, v, street, updates, catalogm.VenueAddressPhaseName, key, accepted.DisplayName)
	}
	return row
}

// writeAddress stores street (plus any street-geocode columns in updates) and
// records the phase hit, in one transaction, only while the row still holds
// the empty address it was read with. The outcome lands on row: Written, or a
// note that the venue changed, or a report error.
func (b *VenueAddressBackfill) writeAddress(report *VenueAddressReport, row *VenueAddressRow, v *catalogm.Venue, street string, updates map[string]interface{}, phase, key, source string) {
	updates["address"] = street
	written := false
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		res := streetGeocodeUpdateScope(tx, v).Updates(updates)
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		written = true
		return recordAddressLookup(tx, v.ID, phase, key, catalogm.VenueAddressOutcomeHit, source, street)
	})
	switch {
	case err != nil:
		report.Errors = append(report.Errors, fmt.Sprintf("venue %d write: %v", v.ID, err))
	case !written:
		row.Notes = append(row.Notes, "not written: the venue changed since it was read")
	default:
		row.Written = true
	}
}

// recordMiss records a phase miss so later runs skip it while key holds.
func (b *VenueAddressBackfill) recordMiss(report *VenueAddressReport, v *catalogm.Venue, phase, key string) {
	if err := recordAddressLookup(b.DB, v.ID, phase, key, catalogm.VenueAddressOutcomeMiss, "", ""); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("venue %d record %s miss: %v", v.ID, phase, err))
	}
}

func baseRow(c *addressCandidate, phase string) VenueAddressRow {
	return VenueAddressRow{
		VenueID:  c.ID,
		Name:     c.Name,
		City:     c.City,
		State:    c.State,
		Verified: c.Verified,
		NextShow: c.NextShow,
		Phase:    phase,
	}
}

// loadCandidates selects venues with an empty address, venues with an
// upcoming show first (soonest show first), then by id.
func (b *VenueAddressBackfill) loadCandidates(ctx context.Context, opts VenueAddressBackfillOptions, now time.Time) ([]addressCandidate, error) {
	q := b.DB.WithContext(ctx).
		Table("venues").
		Select("venues.*, nxt.next_show").
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

func memoMiss(memos map[uint]map[string]catalogm.VenueAddressLookup, venueID uint, phase, key string) bool {
	m, ok := memos[venueID][phase]
	return ok && m.Outcome == catalogm.VenueAddressOutcomeMiss && m.LookupKey == key
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
// at most maxTicketPageSources distinct hosts, and the soonest shows of a
// venue rarely span more than a few vendors.
const ticketPagesPerVenue = 10

// loadTicketPages returns, per venue, the ticket URLs of its soonest approved,
// uncancelled upcoming shows, soonest first.
func (b *VenueAddressBackfill) loadTicketPages(ctx context.Context, ids []uint, now time.Time) (map[uint][]string, error) {
	var rows []struct {
		VenueID   uint
		TicketURL string
	}
	err := b.DB.WithContext(ctx).Raw(`
		SELECT venue_id, ticket_url FROM (
			SELECT sv.venue_id, BTRIM(s.ticket_url) AS ticket_url, s.event_date, s.id,
			       ROW_NUMBER() OVER (PARTITION BY sv.venue_id ORDER BY s.event_date, s.id) AS n
			FROM shows s JOIN show_venues sv ON sv.show_id = s.id
			WHERE sv.venue_id IN ? AND s.status = 'approved' AND NOT s.is_cancelled
			  AND s.event_date >= ? AND COALESCE(BTRIM(s.ticket_url), '') <> ''
		) t
		WHERE n <= ?
		ORDER BY venue_id, event_date, id`, ids, now, ticketPagesPerVenue).
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

// loginWalledHosts are never fetched: their pages are behind a login or a
// consent wall, so they cannot be read without an account.
var loginWalledHosts = []string{
	"instagram.com", "facebook.com", "fb.com", "fb.me", "twitter.com", "x.com",
	"tiktok.com", "threads.net",
}

func loginWalled(host string) bool {
	host = strings.ToLower(strings.TrimPrefix(host, "www."))
	for _, h := range loginWalledHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// pageSources orders a venue's pages: its website, its registered ingest
// source, then up to maxTicketPageSources ticket pages on distinct hosts. A
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
		if loginWalled(u.Hostname()) || seen[raw] {
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
		if len(ticketHosts) >= maxTicketPageSources {
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

// pageLookupKey names the set of pages tried; a changed website or a new
// ticket page changes it, which makes a recorded miss stale.
func pageLookupKey(sources []venueaddress.Source) string {
	urls := make([]string, len(sources))
	for i, s := range sources {
		urls[i] = s.URL
	}
	return strings.Join(urls, " | ")
}

// recordAddressLookup upserts the venue's lookup row for one phase.
func recordAddressLookup(db *gorm.DB, venueID uint, phase, key, outcome, source, address string) error {
	row := catalogm.VenueAddressLookup{
		VenueID:     venueID,
		Phase:       phase,
		LookupKey:   key,
		Outcome:     outcome,
		AttemptedAt: time.Now().UTC(),
	}
	if source != "" {
		row.Source = &source
	}
	if address != "" {
		row.Address = &address
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "venue_id"}, {Name: "phase"}},
		DoUpdates: clause.AssignmentColumns([]string{"lookup_key", "outcome", "source", "address", "attempted_at"}),
	}).Create(&row).Error
}
