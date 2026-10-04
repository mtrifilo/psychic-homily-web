package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/venueaddress"
)

// =============================================================================
// Acceptance rules and report (pure)
// =============================================================================

func lincolnHallCandidate() geo.PlaceCandidate {
	return geo.PlaceCandidate{
		Category: "amenity", Type: "music_venue", Name: "Lincoln Hall",
		DisplayName: "Lincoln Hall, 2424, North Lincoln Avenue, Lincoln Park, Chicago, Cook County, Illinois, 60614, United States",
		Latitude:    41.9258, Longitude: -87.6493,
		HouseNumber: "2424", Road: "North Lincoln Avenue",
		Localities: []string{"Chicago", "Lincoln Park"}, StateCode: "US-IL", CountryCode: "us",
	}
}

func TestAcceptPlaceCandidate(t *testing.T) {
	venue := &catalogm.Venue{Name: "Lincoln Hall", City: "Chicago", State: "IL"}
	tests := []struct {
		name   string
		mutate func(*geo.PlaceCandidate)
		venue  *catalogm.Venue
		cc     string
		ok     bool
		street string
	}{
		{name: "venue-like place in the city", ok: true, street: "2424 North Lincoln Avenue", cc: "US"},
		{name: "a building counts", mutate: func(c *geo.PlaceCandidate) { c.Category, c.Type = "building", "yes" }, ok: true, street: "2424 North Lincoln Avenue"},
		{name: "leisure=music_venue counts", mutate: func(c *geo.PlaceCandidate) { c.Category, c.Type = "leisure", "music_venue" }, ok: true, street: "2424 North Lincoln Avenue"},
		{name: "a road is refused", mutate: func(c *geo.PlaceCandidate) { c.Category, c.Type = "highway", "residential" }},
		{name: "a shop is refused", mutate: func(c *geo.PlaceCandidate) { c.Category, c.Type = "shop", "music" }},
		{name: "an unnamed feature is refused", mutate: func(c *geo.PlaceCandidate) { c.Name = "" }},
		{name: "another name is refused", mutate: func(c *geo.PlaceCandidate) { c.Name = "Schubas Tavern" }},
		{name: "another city is refused", mutate: func(c *geo.PlaceCandidate) {
			c.Localities = []string{"Evanston"}
			c.DisplayName = "Lincoln Hall, 1 Main St, Evanston, Cook County, Illinois, United States"
		}},
		{name: "a borough in the display name matches", venue: &catalogm.Venue{Name: "Lincoln Hall", City: "Lincoln Park", State: "IL"}, ok: true, street: "2424 North Lincoln Avenue"},
		{name: "another state is refused", mutate: func(c *geo.PlaceCandidate) { c.StateCode = "US-WI" }},
		{name: "another country is refused", mutate: func(c *geo.PlaceCandidate) { c.CountryCode = "ca" }, cc: "US"},
		{name: "no house number is refused", mutate: func(c *geo.PlaceCandidate) { c.HouseNumber = "" }},
		{name: "street-first country", mutate: func(c *geo.PlaceCandidate) {
			c.CountryCode, c.StateCode, c.Road, c.HouseNumber = "de", "", "Hauptstrasse", "12"
		}, venue: &catalogm.Venue{Name: "Lincoln Hall", City: "Chicago", State: ""}, ok: true, street: "Hauptstrasse 12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := lincolnHallCandidate()
			if tt.mutate != nil {
				tt.mutate(&c)
			}
			v := venue
			if tt.venue != nil {
				v = tt.venue
			}
			street, ok, why := AcceptPlaceCandidate(c, v, tt.cc)
			if ok != tt.ok || street != tt.street {
				t.Fatalf("accept = %q ok=%v why=%q, want %q ok=%v", street, ok, why, tt.street, tt.ok)
			}
			if !ok && why == "" {
				t.Fatal("a refusal must say why")
			}
		})
	}
}

func TestPlaceMatchCautions(t *testing.T) {
	v := &catalogm.Venue{Name: "The Lincoln Hall"}
	if got := placeMatchCautions(lincolnHallCandidate(), v); len(got) != 0 {
		t.Fatalf("an exact venue-like match carries no caution, got %v", got)
	}
	cafe := lincolnHallCandidate()
	cafe.Name = "Cafe Racer"
	if got := placeMatchCautions(cafe, &catalogm.Venue{Name: "Café Racer"}); len(got) != 0 {
		t.Fatalf("names equal apart from accents are exact, got %v", got)
	}
	c := lincolnHallCandidate()
	c.Name, c.Category, c.Type = "Lincoln Hall Annex", "building", "university"
	got := placeMatchCautions(c, v)
	if len(got) != 2 || got[0] != CautionPartialName || got[1] != CautionBuilding {
		t.Fatalf("cautions = %v", got)
	}
}

func TestPageSources_OrderAndLoginWalls(t *testing.T) {
	website := "https://www.instagram.com/somevenue"
	v := &catalogm.Venue{Social: catalogm.Social{Website: &website}}
	got := pageSources(v, "https://venue.example/calendar", []string{
		"https://tickets.example/a", "https://tickets.example/b", "https://other.example/c", "not a url",
		"https://third.example/d", "https://fourth.example/e", "https://fifth.example/f",
	})
	want := []venueaddress.Source{
		{URL: "https://venue.example/calendar", Kind: venueaddress.SourceIngest},
		{URL: "https://tickets.example/a", Kind: venueaddress.SourceTicket},
		{URL: "https://other.example/c", Kind: venueaddress.SourceTicket},
		{URL: "https://third.example/d", Kind: venueaddress.SourceTicket},
		{URL: "https://fourth.example/e", Kind: venueaddress.SourceTicket},
	}
	if len(got) != len(want) {
		t.Fatalf("pageSources = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pageSources[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	wantKey := "v1 | https://venue.example/calendar | ticket:fourth.example | ticket:other.example | ticket:third.example | ticket:tickets.example"
	if key := pageLookupKey(got, true); key != wantKey {
		t.Fatalf("pageLookupKey = %q", key)
	}
	// Later events on the same vendors, in another order, keep the key;
	// schema-only runs key apart.
	later := pageSources(v, "https://venue.example/calendar", []string{
		"https://third.example/x", "https://other.example/y", "https://tickets.example/z", "https://fourth.example/w",
	})
	if key := pageLookupKey(later, true); key != wantKey {
		t.Fatalf("a new event URL on a known vendor changed the key: %q", key)
	}
	if pageLookupKey(got, false) == wantKey {
		t.Fatal("a schema-only run must not share its miss key with an AI run")
	}
}

func TestVenueAddressReport_MarkdownAndJSON(t *testing.T) {
	lat, lng := 41.9, -87.6
	r := &VenueAddressReport{
		GeneratedAt: time.Date(2026, 10, 4, 22, 0, 0, 0, time.UTC),
		Limit:       100,
		Candidates:  3,
		Processed:   2,
		Phases: map[string]*VenueAddressPhaseTotals{
			"page": {Attempted: 2, Hits: 1, Misses: 1},
			"name": {Attempted: 1, Hits: 1},
		},
		Precision:  map[string]int{"rooftop": 1, "name_search": 1},
		WouldWrite: 2,
		Rows: []VenueAddressRow{
			{VenueID: 1, Name: "Pipe | Venue", City: "Chicago", State: "IL", Phase: "page", Outcome: "hit",
				Source: "https://x.example", Address: "1 Main St", Precision: "rooftop", WouldWrite: true},
			{VenueID: 2, Name: "Lincoln Hall", City: "Chicago", State: "IL", Phase: "name", Outcome: "hit",
				Source: "Lincoln Hall, Chicago, IL [US]", Address: "2424 North Lincoln Avenue", MatchedName: "Lincoln Hall",
				Precision: "name_search", Latitude: &lat, Longitude: &lng, WouldWrite: true, Notes: []string{"matched amenity=music_venue"}},
		},
	}
	var md bytes.Buffer
	if err := r.WriteMarkdown(&md); err != nil {
		t.Fatal(err)
	}
	out := md.String()
	for _, want := range []string{
		"# Venue address backfill (lookup report)",
		"| page | 2 | 1 | 50% | 1 | 0 | 0 | 0 |",
		"| name | 1 | 1 | 100% |",
		"| name_search | 1 |",
		"Pipe \\| Venue",
		"2424 North Lincoln Avenue (OSM: Lincoln Hall)",
		"Would write: 2, of which REVIEW: 0.",
		"Nothing was written.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown lacks %q:\n%s", want, out)
		}
	}
	var js bytes.Buffer
	if err := r.WriteJSON(&js); err != nil {
		t.Fatal(err)
	}
	var back VenueAddressReport
	if err := json.Unmarshal(js.Bytes(), &back); err != nil {
		t.Fatalf("JSON does not round-trip: %v", err)
	}
	if back.Phases["name"].Hits != 1 || back.Rows[1].Precision != "name_search" || !back.Rows[1].WouldWrite {
		t.Fatalf("round-tripped report = %+v", back)
	}
}

// =============================================================================
// Lookup and apply against a real database (VenueServiceIntegrationTestSuite)
// =============================================================================

// stubPageFinder answers per venue name and records what it was asked.
type stubPageFinder struct {
	results map[string]venueaddress.Result
	err     error
	calls   int
	sources map[string][]venueaddress.Source
}

func (s *stubPageFinder) Find(_ context.Context, v venueaddress.Venue, sources []venueaddress.Source) (venueaddress.Result, error) {
	s.calls++
	if s.sources == nil {
		s.sources = map[string][]venueaddress.Source{}
	}
	s.sources[v.Name] = sources
	if s.err != nil {
		return venueaddress.Result{}, s.err
	}
	return s.results[v.Name], nil
}

type stubPlaces struct {
	results map[string][]geo.PlaceCandidate
	err     error
	calls   int
}

func (s *stubPlaces) SearchPlaces(_ context.Context, q geo.PlaceQuery) ([]geo.PlaceCandidate, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.results[q.Name], nil
}

func (suite *VenueServiceIntegrationTestSuite) seedAddresslessVenue(name, city, state, website string, showIn time.Duration) *catalogm.Venue {
	lat, lng := 41.85, -87.65
	v := &catalogm.Venue{Name: name, City: city, State: state, Verified: true, Latitude: &lat, Longitude: &lng}
	if website != "" {
		v.Social.Website = &website
	}
	suite.Require().NoError(suite.db.Create(v).Error)
	if showIn != 0 {
		show := &catalogm.Show{Title: name + " show", EventDate: time.Now().Add(showIn), Status: catalogm.ShowStatusApproved}
		suite.Require().NoError(suite.db.Create(show).Error)
		suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: v.ID}).Error)
	}
	return v
}

func pageHit(street, url string) venueaddress.Result {
	return venueaddress.Result{
		Found: true, Street: street, City: "Chicago", Method: venueaddress.MethodJSONLD,
		Source: venueaddress.Source{URL: url, Kind: venueaddress.SourceWebsite},
	}
}

func (suite *VenueServiceIntegrationTestSuite) lookups(venueID uint) map[string]catalogm.VenueAddressLookup {
	var rows []catalogm.VenueAddressLookup
	suite.Require().NoError(suite.db.Where("venue_id = ?", venueID).Find(&rows).Error)
	out := map[string]catalogm.VenueAddressLookup{}
	for _, r := range rows {
		out[r.Phase] = r
	}
	return out
}

func rowFor(r *VenueAddressReport, name, phase string) *VenueAddressRow {
	for i := range r.Rows {
		if r.Rows[i].Name == name && r.Rows[i].Phase == phase {
			return &r.Rows[i]
		}
	}
	return nil
}

func rowNames(r *VenueAddressReport) []string {
	var names []string
	for _, row := range r.Rows {
		names = append(names, row.Name)
	}
	return names
}

// roundTrip passes a report through its JSON form, as the reviewed file does.
func (suite *VenueServiceIntegrationTestSuite) roundTrip(r *VenueAddressReport) *VenueAddressReport {
	var buf bytes.Buffer
	suite.Require().NoError(r.WriteJSON(&buf))
	var back VenueAddressReport
	suite.Require().NoError(json.Unmarshal(buf.Bytes(), &back))
	return &back
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_LookupWritesNothing() {
	page := suite.seedAddresslessVenue("Page Venue", "Chicago", "IL", "https://page.example", 48*time.Hour)
	named := suite.seedAddresslessVenue("Lincoln Hall", "Chicago", "IL", "", 72*time.Hour)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{"Page Venue": pageHit("1 Main St", "https://page.example")}}
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln Hall": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop), AIEnabled: true}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(2, report.Candidates)
	suite.Equal(2, report.WouldWrite)
	suite.Equal(1, report.Phases["page"].Hits)
	suite.Equal(1, report.Phases["page"].NoSource, "the named venue has no page")
	suite.Equal(1, report.Phases["name"].Hits)
	suite.Equal(map[string]int{geo.PrecisionRooftop: 1, geo.PrecisionNameSearch: 1}, report.Precision)
	suite.Equal([]string{"Page Venue", "Lincoln Hall"}, rowNames(report), "the venue with the sooner show goes first")
	p := rowFor(report, "Page Venue", "page")
	suite.Equal(GeocodeHit, p.Geocode)
	suite.Equal("v1 | https://page.example", p.LookupKey)
	n := rowFor(report, "Lincoln Hall", "name")
	suite.Equal("2424 North Lincoln Avenue", n.Address)
	suite.Equal("v1 | Lincoln Hall, Chicago, IL [US]", n.LookupKey)
	suite.False(n.ApproveReview, "a generated report never pre-approves")

	for _, id := range []uint{page.ID, named.ID} {
		v := suite.loadVenue(id)
		suite.Nil(v.Address, "the lookup must not write")
		suite.Nil(v.StreetLatitude)
		suite.Empty(suite.lookups(id), "the lookup must not record anything")
	}
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ApplyWritesTheReviewedRowsAndRecordsMisses() {
	page := suite.seedAddresslessVenue("Page Venue", "Chicago", "IL", "https://page.example", 48*time.Hour)
	named := suite.seedAddresslessVenue("Lincoln Hall", "Chicago", "IL", "", 72*time.Hour)
	missed := suite.seedAddresslessVenue("Nowhere Club", "Chicago", "IL", "https://nowhere.example", 96*time.Hour)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{"Page Venue": pageHit("1 Main St", "https://page.example")}}
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln Hall": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop), AIEnabled: true}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)

	pagesBefore, placesBefore := finder.calls, places.calls
	result, err := (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), suite.roundTrip(report))
	suite.Require().NoError(err)
	suite.Equal(2, result.Written)
	suite.Equal(2, result.MissesRecorded, "the missed venue's page and name misses")
	suite.Equal(pagesBefore, finder.calls, "apply makes no lookups")
	suite.Equal(placesBefore, places.calls, "apply makes no lookups")

	v := suite.loadVenue(page.ID)
	suite.Equal("1 Main St", *v.Address)
	suite.Equal(geo.PrecisionRooftop, *v.GeocodePrecision)
	suite.Equal("1 Main St, Chicago, IL", *v.GeocodedAddress)
	suite.InDelta(41.85, *v.Latitude, 1e-6, "the city centroid is never touched")
	suite.Equal("https://page.example", *suite.lookups(page.ID)["page"].Source)

	n := suite.loadVenue(named.ID)
	suite.Equal("2424 North Lincoln Avenue", *n.Address)
	suite.Equal(geo.PrecisionNameSearch, *n.GeocodePrecision)
	suite.InDelta(41.9258, *n.StreetLatitude, 1e-6)
	suite.True(streetGeocodeFresh(n), "the stored key matches the new address, so the point is served")

	m := suite.lookups(missed.ID)
	suite.Equal(catalogm.VenueAddressOutcomeMiss, m["page"].Outcome)
	suite.Equal(catalogm.VenueAddressOutcomeMiss, m["name"].Outcome)

	// The sweep treats both written venues as already attempted.
	sweepGeocoder := hitStub(0, 0, geo.PrecisionRooftop)
	sweep, err := BackfillVenueStreetGeocodes(context.Background(), suite.db, sweepGeocoder, StreetGeocodeOptions{})
	suite.Require().NoError(err)
	suite.Equal(0, sweepGeocoder.calls)
	suite.Equal(2, sweep.Unchanged)

	// The next lookup selects only the missed venue and skips both its phases.
	report2, err := run.Run(context.Background(), VenueAddressBackfillOptions{Limit: 1})
	suite.Require().NoError(err)
	suite.Equal(1, report2.Candidates)
	suite.Equal(0, report2.Processed, "recorded misses spend no limit")
	suite.Equal(1, report2.Phases["page"].SkippedMemo)
	suite.Equal(1, report2.Phases["name"].SkippedMemo)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ApplyRefusals() {
	approved := suite.seedAddresslessVenue("Approved Venue", "Chicago", "IL", "https://a.example", 0)
	refused := suite.seedAddresslessVenue("Refused Venue", "Chicago", "IL", "https://r.example", 0)
	rewebbed := suite.seedAddresslessVenue("Rewebbed Venue", "Chicago", "IL", "https://n.example", 0)
	renamed := suite.seedAddresslessVenue("Renamed Venue", "Chicago", "IL", "https://rn.example", 0)
	filled := suite.seedAddresslessVenue("Filled Venue", "Chicago", "IL", "https://f.example", 0)
	review := suite.seedAddresslessVenue("Lincoln", "Chicago", "IL", "", 0) // partial match of "Lincoln Hall"
	finder := &stubPageFinder{results: map[string]venueaddress.Result{
		"Approved Venue": pageHit("1 Main St", "https://a.example"),
		"Refused Venue":  pageHit("2 Main St", "https://r.example"),
		"Rewebbed Venue": pageHit("3 Main St", "https://n.example"),
		"Renamed Venue":  pageHit("5 Main St", "https://rn.example"),
		"Filled Venue":   pageHit("4 Main St", "https://f.example"),
	}}
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop), AIEnabled: true}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.True(rowFor(report, "Lincoln", "name").Review)

	reviewed := suite.roundTrip(report)
	rowFor(reviewed, "Refused Venue", "page").WouldWrite = false
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", rewebbed.ID).Update("website", "https://new.example").Error)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", renamed.ID).Update("name", "Another Name").Error)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", filled.ID).Update("address", "77 Editor St").Error)
	evil := 999.0
	rowFor(reviewed, "Approved Venue", "page").Latitude = &evil

	result, err := (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), reviewed)
	suite.Require().NoError(err)
	suite.Equal(0, result.Written)
	reasons := map[string]string{}
	for _, r := range result.Rows {
		reasons[r.Name] = r.Reason
	}
	suite.Contains(reasons["Approved Venue"], "coordinates")
	suite.Contains(reasons["Refused Venue"], "would_write is false")
	suite.Contains(reasons["Rewebbed Venue"], "inputs changed")
	suite.Contains(reasons["Renamed Venue"], "name, city, or state changed")
	suite.Contains(reasons["Filled Venue"], "has an address now")
	suite.Contains(reasons["Lincoln"], "approve_review")
	for _, id := range []uint{approved.ID, refused.ID, rewebbed.ID, renamed.ID, review.ID} {
		suite.Nil(suite.loadVenue(id).Address)
	}
	suite.Equal(2, result.Refused, "the refused hit and the unapproved REVIEW row")
	suite.Equal(catalogm.VenueAddressOutcomeMiss, suite.lookups(refused.ID)["page"].Outcome,
		"a refused hit is recorded as a miss, so the next lookup moves past it")
	suite.Equal("77 Editor St", *suite.loadVenue(filled.ID).Address, "a non-empty address is never overwritten")

	// The reviewer accepts the REVIEW row explicitly; then it is written.
	reviewed = suite.roundTrip(report)
	rowFor(reviewed, "Lincoln", "name").ApproveReview = true
	for i := range reviewed.Rows {
		if reviewed.Rows[i].Name != "Lincoln" {
			reviewed.Rows[i].WouldWrite = false
		}
	}
	result, err = (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), reviewed)
	suite.Require().NoError(err)
	suite.Equal(1, result.Written)
	suite.Equal("2424 North Lincoln Avenue", *suite.loadVenue(review.ID).Address)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ApplyStoresThePageGeocodeOutcome() {
	missV := suite.seedAddresslessVenue("Geo Miss", "Chicago", "IL", "https://m.example", 0)
	noneV := suite.seedAddresslessVenue("Geo None", "Chicago", "IL", "https://n.example", 0)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{
		"Geo Miss": pageHit("1 Miss St", "https://m.example"),
		"Geo None": pageHit("3 None St", "https://n.example"),
	}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Geocoder: &stubAddressGeocoder{}, AIEnabled: true}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(GeocodeMiss, rowFor(report, "Geo Miss", "page").Geocode)
	rowFor(report, "Geo None", "page").Geocode = GeocodeNone // as a failed or absent geocode leaves it

	result, err := (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), suite.roundTrip(report))
	suite.Require().NoError(err)
	suite.Equal(2, result.Written)
	m := suite.loadVenue(missV.ID)
	suite.Equal("1 Miss St", *m.Address)
	suite.Equal("1 Miss St, Chicago, IL", *m.GeocodedAddress, "a clean miss stores the miss memo for the new address")
	suite.Nil(m.StreetLatitude)
	n := suite.loadVenue(noneV.ID)
	suite.Equal("3 None St", *n.Address)
	suite.Nil(n.GeocodedAddress, "no geocode outcome stores no memo, so the sweep geocodes it")

	sweepGeocoder := hitStub(41.9, -87.6, geo.PrecisionRooftop)
	_, err = BackfillVenueStreetGeocodes(context.Background(), suite.db, sweepGeocoder, StreetGeocodeOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, sweepGeocoder.calls, "the sweep geocodes the ungeocoded address and skips the recorded miss")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ErrorsAreNotRecordedAndAPageErrorFlagsTheNameMatch() {
	v := suite.seedAddresslessVenue("Lincoln Hall", "Chicago", "IL", "https://flaky.example", 0)
	pageless := suite.seedAddresslessVenue("Pageless Venue", "Chicago", "IL", "", 0)
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln Hall": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: &stubPageFinder{err: errors.New("page timed out")}, Places: places}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Phases["page"].Errors)
	row := rowFor(report, "Lincoln Hall", "name")
	suite.Require().NotNil(row, "a page error does not hold the name search back")
	suite.True(row.Review)
	suite.Contains(row.Notes, CautionPageUnread)

	_, err = (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), suite.roundTrip(report))
	suite.Require().NoError(err)
	suite.Nil(suite.loadVenue(v.ID).Address, "the flagged row needs approve_review")
	suite.Empty(rowFor(report, "Lincoln Hall", "page").Address)
	suite.NotContains(suite.lookups(v.ID), "page", "an error is never recorded")
	suite.Contains(suite.lookups(pageless.ID), "name", "the pageless venue's name miss is recorded")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_MissMemoExpiresAndFollowsInputs() {
	v := suite.seedAddresslessVenue("Nowhere Club", "Chicago", "IL", "https://nowhere.example", 0)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{}}
	places := &stubPlaces{}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal("v1 | https://nowhere.example | [schema-only]", rowFor(report, "Nowhere Club", "page").LookupKey)
	_, err = (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), suite.roundTrip(report))
	suite.Require().NoError(err)

	_, err = run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, finder.calls, "an unchanged venue is skipped")

	// Turning the AI fallback on changes the page key.
	run.AIEnabled = true
	_, err = run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(2, finder.calls)
	suite.Equal(1, places.calls, "the name key did not change")

	// A miss older than the TTL is tried again.
	_, err = run.Run(context.Background(), VenueAddressBackfillOptions{Now: time.Now().Add(missMemoTTL + time.Hour)})
	suite.Require().NoError(err)
	suite.Equal(2, places.calls)
	_ = v
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ClearedAddressesAreNeverRefilled() {
	byBackfill := suite.seedAddresslessVenue("Cleared By Backfill", "Chicago", "IL", "https://a.example", 0)
	byEditor := suite.seedAddresslessVenue("Cleared By Editor", "Chicago", "IL", "https://b.example", 0)
	suite.Require().NoError(recordAddressLookup(suite.db, byBackfill.ID, "page", "k", "hit", "https://a.example", "1 Main St", time.Time{}))
	user := suite.createTestUser()
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO revisions (entity_type, entity_id, user_id, field_changes) VALUES ('venue', ?, ?, ?::jsonb)`,
		byEditor.ID, user.ID, `[{"field":"address","old_value":"2 Main St","new_value":""}]`).Error)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{
		"Cleared By Backfill": pageHit("1 Main St", "https://a.example"),
		"Cleared By Editor":   pageHit("2 Main St", "https://b.example"),
	}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(2, report.SkippedCleared)
	suite.Equal(0, finder.calls)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_FiltersOrderAndLimit() {
	suite.seedAddresslessVenue("Later Show", "Chicago", "IL", "", 30*24*time.Hour)
	suite.seedAddresslessVenue("No Show", "Chicago", "IL", "", 0)
	suite.seedAddresslessVenue("Far Show", "Chicago", "IL", "", 200*24*time.Hour)
	suite.seedAddresslessVenue("Soon Show", "Chicago", "IL", "", 24*time.Hour)
	suite.seedAddresslessVenue("Elsewhere", "Milwaukee", "WI", "", 24*time.Hour)
	run := &VenueAddressBackfill{DB: suite.db, Places: &stubPlaces{}}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{OnlyUpcoming: true, City: "chicago"})
	suite.Require().NoError(err)
	suite.Equal(2, report.Candidates, "a show beyond 90 days does not make a venue upcoming")
	suite.Equal([]string{"Soon Show", "Later Show"}, rowNames(report))

	report, err = run.Run(context.Background(), VenueAddressBackfillOptions{City: "Chicago", Limit: 3})
	suite.Require().NoError(err)
	suite.Equal(4, report.Candidates)
	suite.True(report.LimitHit)
	suite.Equal([]string{"Soon Show", "Later Show", "No Show"}, rowNames(report),
		"upcoming venues first by show date, the rest by id")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_PageSourcesAndTrustedTicketPages() {
	v := suite.seedAddresslessVenue("Sourced Venue", "Chicago", "IL", "https://sourced.example", 0)
	src := "https://sourced.example/events"
	suite.Require().NoError(suite.db.Create(&adminm.SourceConfig{EntityType: "venue", EntityID: v.ID, SourceURL: &src}).Error)
	defer suite.db.Exec("DELETE FROM source_configs")

	admin := suite.createTestUser()
	suite.Require().NoError(suite.db.Model(admin).Update("is_admin", true).Error)
	plain := suite.createTestUser()
	trusted := map[string]*uint{}
	for _, tier := range append([]string{"new_user"}, authm.TrustedTiers...) {
		u := suite.createTestUser()
		suite.Require().NoError(suite.db.Model(u).Update("user_tier", tier).Error)
		trusted[tier] = &u.ID
	}
	shows := []struct {
		url       string
		submitter *uint
		source    catalogm.ShowSource
		cancelled bool
	}{
		{"https://discovery.example/e/1", nil, catalogm.ShowSourceDiscovery, false},
		{"https://admin.example/e/2", &admin.ID, catalogm.ShowSourceUser, false},
		{"https://trusted.example/e/3", trusted["trusted_contributor"], catalogm.ShowSourceUser, false},
		{"https://ambassador.example/e/4", trusted["local_ambassador"], catalogm.ShowSourceUser, false},
		{"https://untrusted.example/e/5", &plain.ID, catalogm.ShowSourceUser, false},
		{"https://newuser.example/e/6", trusted["new_user"], catalogm.ShowSourceUser, false},
		{"https://cancelled.example/e/7", &admin.ID, catalogm.ShowSourceUser, true},
	}
	for i, s := range shows {
		url := s.url
		show := &catalogm.Show{Title: "t", EventDate: time.Now().Add(time.Duration(i+1) * 24 * time.Hour), Status: catalogm.ShowStatusApproved,
			TicketURL: &url, SubmittedBy: s.submitter, Source: s.source, IsCancelled: s.cancelled}
		suite.Require().NoError(suite.db.Create(show).Error)
		suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: v.ID}).Error)
	}

	finder := &stubPageFinder{results: map[string]venueaddress.Result{}}
	_, err := (&VenueAddressBackfill{DB: suite.db, Pages: finder, AIEnabled: true}).Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal([]venueaddress.Source{
		{URL: "https://sourced.example", Kind: venueaddress.SourceWebsite},
		{URL: "https://sourced.example/events", Kind: venueaddress.SourceIngest},
		{URL: "https://discovery.example/e/1", Kind: venueaddress.SourceTicket},
		{URL: "https://admin.example/e/2", Kind: venueaddress.SourceTicket},
		{URL: "https://trusted.example/e/3", Kind: venueaddress.SourceTicket},
		{URL: "https://ambassador.example/e/4", Kind: venueaddress.SourceTicket},
	}, finder.sources["Sourced Venue"], "only shows from the discovery import, admins, and the trusted tiers count")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_SchemaAcceptsNameSearchPrecision() {
	v := suite.seedAddresslessVenue("Schema Venue", "Chicago", "IL", "", 0)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", v.ID).
		Update("geocode_precision", geo.PrecisionNameSearch).Error)
	err := suite.db.Model(&catalogm.Venue{}).Where("id = ?", v.ID).Update("geocode_precision", "guess").Error
	suite.Error(err, "the check constraint still refuses values outside the vocabulary")

	suite.Require().NoError(recordAddressLookup(suite.db, v.ID, "name", "k1", "miss", "", "", time.Time{}))
	suite.Require().NoError(recordAddressLookup(suite.db, v.ID, "name", "k2", "hit", "src", "1 Main St", time.Time{}))
	l := suite.lookups(v.ID)["name"]
	suite.Equal("k2", l.LookupKey, "a second attempt replaces the phase row")
	suite.Equal("hit", l.Outcome)
	suite.Require().NoError(recordAddressLookup(suite.db, v.ID, "name", "k3", "miss", "", "", time.Time{}))
	suite.Equal("hit", suite.lookups(v.ID)["name"].Outcome, "a miss never replaces a recorded hit")
	suite.Error(recordAddressLookup(suite.db, v.ID, "phone", "k", "miss", "", "", time.Time{}), "phase is constrained")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_UnmigratedDatabaseLooksUpButCannotApply() {
	suite.seedAddresslessVenue("Old Schema Venue", "Chicago", "IL", "", 0)
	tx := suite.db.Begin()
	defer tx.Rollback()
	suite.Require().NoError(tx.Exec("DROP TABLE venue_address_lookups").Error)
	run := &VenueAddressBackfill{DB: tx, Places: &stubPlaces{}}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.True(report.LookupsTableMissing)
	suite.Equal(1, report.Phases["name"].Attempted)

	_, err = run.Apply(context.Background(), report)
	suite.ErrorContains(err, "apply the database migrations")
}

// TestAddressBackfill_NominatimStubEndToEnd drives the name phase through the
// real Nominatim client against a local stub server: request shape, parsing,
// acceptance (an exact name beats a partial one ranked first), and the stored
// result after apply.
func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_NominatimStubEndToEnd() {
	v := suite.seedAddresslessVenue("Lincoln Hall", "Chicago", "IL", "", 0)
	var calls int32
	var gotQ, gotCC string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		gotQ, gotCC = r.URL.Query().Get("q"), r.URL.Query().Get("countrycodes")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
		  {"lat":"41.9300","lon":"-87.6400","category":"highway","type":"residential","name":"Lincoln Avenue",
		   "display_name":"Lincoln Avenue, Chicago, Illinois, United States","address":{"road":"Lincoln Avenue","city":"Chicago","ISO3166-2-lvl4":"US-IL","country_code":"us"}},
		  {"lat":"41.9000","lon":"-87.7000","category":"amenity","type":"bar","name":"Lincoln Hall Annex",
		   "display_name":"Lincoln Hall Annex, 9, West Street, Chicago, Illinois, United States","address":{"house_number":"9","road":"West Street","city":"Chicago","ISO3166-2-lvl4":"US-IL","country_code":"us"}},
		  {"lat":"41.925800","lon":"-87.649300","osm_type":"node","category":"amenity","type":"music_venue","name":"Lincoln Hall",
		   "display_name":"Lincoln Hall, 2424, North Lincoln Avenue, Chicago, Illinois, 60614, United States",
		   "address":{"house_number":"2424","road":"North Lincoln Avenue","city":"Chicago","ISO3166-2-lvl4":"US-IL","postcode":"60614","country_code":"us"}}
		]`))
	}))
	defer srv.Close()

	run := &VenueAddressBackfill{DB: suite.db, Places: geo.NewNominatimClient(srv.URL)}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(int32(1), calls)
	suite.Equal("Lincoln Hall, Chicago, IL", gotQ)
	suite.Equal("us", gotCC)
	row := rowFor(report, "Lincoln Hall", "name")
	suite.Require().NotNil(row)
	suite.Contains(row.Notes[0], "rejected highway=residential")
	suite.Equal("2424 North Lincoln Avenue", row.Address, "the exact name wins over a partial match ranked above it")
	suite.False(row.Review)

	_, err = (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), suite.roundTrip(report))
	suite.Require().NoError(err)
	got := suite.loadVenue(v.ID)
	suite.Equal("2424 North Lincoln Avenue", *got.Address)
	suite.InDelta(41.9258, *got.StreetLatitude, 1e-6)
	suite.Equal(geo.PrecisionNameSearch, *got.GeocodePrecision)
	suite.Contains(*suite.lookups(v.ID)["name"].Source, "2424, North Lincoln Avenue")
}

func TestStubPlacesSatisfiesInterface(t *testing.T) {
	var _ geo.PlaceSearcher = (*stubPlaces)(nil)
	var _ PageAddressFinder = (*stubPageFinder)(nil)
	var _ PageAddressFinder = (*venueaddress.Finder)(nil)
	var _ geo.PlaceSearcher = (*geo.NominatimClient)(nil)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ApplyRowChecks() {
	page := suite.seedAddresslessVenue("Corrected Page", "Chicago", "IL", "https://c.example", 0)
	name := suite.seedAddresslessVenue("Lincoln Hall", "Chicago", "IL", "", 0)
	bad := suite.seedAddresslessVenue("Bad Values", "Chicago", "IL", "https://b.example", 0)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{
		"Corrected Page": pageHit("1 Mian St", "https://c.example"),
		"Bad Values":     pageHit("2 Main St", "https://b.example"),
	}}
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln Hall": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop), AIEnabled: true}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)

	reviewed := suite.roundTrip(report)
	rowFor(reviewed, "Corrected Page", "page").Address = "1 Main St"
	rowFor(reviewed, "Lincoln Hall", "name").Address = "2425 North Lincoln Avenue"
	rowFor(reviewed, "Bad Values", "page").Precision = geo.PrecisionNameSearch
	result, err := (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), reviewed)
	suite.Require().NoError(err)
	reasons := map[string]string{}
	for _, r := range result.Rows {
		reasons[r.Name] = r.Reason
	}
	suite.Equal(1, result.Written)
	p := suite.loadVenue(page.ID)
	suite.Equal("1 Main St", *p.Address, "a corrected page address is written")
	suite.Nil(p.GeocodedAddress, "without the geocode of the uncorrected address; the sweep geocodes it")
	suite.Nil(p.StreetLatitude)
	suite.Contains(reasons["Lincoln Hall"], "cannot be corrected")
	suite.Nil(suite.loadVenue(name.ID).Address)
	suite.Contains(reasons["Bad Values"], "precision")
	suite.Nil(suite.loadVenue(bad.ID).Address)

	// A report with two rows for one venue and phase is refused whole.
	dup := suite.roundTrip(report)
	dup.Rows = append(dup.Rows, dup.Rows[0])
	_, err = (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), dup)
	suite.ErrorContains(err, "two rows")

	// So is a stale one.
	stale := suite.roundTrip(report)
	stale.GeneratedAt = time.Now().Add(-maxReportAge - time.Hour)
	_, err = (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), stale)
	suite.ErrorContains(err, "older than")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ApplyUnverifiedAndClearedSinceTheReport() {
	unverified := suite.seedAddresslessVenue("Lincoln Hall", "Chicago", "IL", "", 0)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", unverified.ID).Update("verified", false).Error)
	cleared := suite.seedAddresslessVenue("Cleared Later", "Chicago", "IL", "https://cl.example", 0)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{"Cleared Later": pageHit("1 Main St", "https://cl.example")}}
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln Hall": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop), AIEnabled: true}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	u := rowFor(report, "Lincoln Hall", "name")
	suite.True(u.Review)
	suite.Contains(u.Notes, CautionUnverified)

	// An editor clears the address between the lookup and the apply; the
	// revision of an unverified venue records the old value as withheld.
	user := suite.createTestUser()
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO revisions (entity_type, entity_id, user_id, field_changes) VALUES ('venue', ?, ?, ?::jsonb)`,
		cleared.ID, user.ID, `[{"field":"address","old_value":"","new_value":"","old_value_withheld":true}]`).Error)

	reviewed := suite.roundTrip(report)
	rowFor(reviewed, "Lincoln Hall", "name").Review = false // a reviewer cannot unflag an unverified venue
	result, err := (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), reviewed)
	suite.Require().NoError(err)
	suite.Equal(0, result.Written)
	suite.Nil(suite.loadVenue(unverified.ID).Address)
	suite.Nil(suite.loadVenue(cleared.ID).Address)
	reasons := map[string]string{}
	for _, r := range result.Rows {
		reasons[r.Name] = r.Reason
	}
	suite.Contains(reasons["Lincoln Hall"], "approve_review")
	suite.Contains(reasons["Cleared Later"], "cleared")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ApplyRebuildsTheTicketPagesTheReportSaw() {
	v := suite.seedAddresslessVenue("Ticketed Venue", "Chicago", "IL", "https://t.example", 0)
	admin := suite.createTestUser()
	suite.Require().NoError(suite.db.Model(admin).Update("is_admin", true).Error)
	// A show that was upcoming when the lookup ran and has passed since.
	ticket := "https://vendor.example/e/1"
	show := &catalogm.Show{Title: "t", EventDate: time.Now().Add(-24 * time.Hour), Status: catalogm.ShowStatusApproved,
		TicketURL: &ticket, SubmittedBy: &admin.ID, CreatedAt: time.Now().Add(-72 * time.Hour)}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: v.ID}).Error)

	finder := &stubPageFinder{results: map[string]venueaddress.Result{"Ticketed Venue": pageHit("1 Main St", "https://t.example")}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop), AIEnabled: true}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{Now: time.Now().Add(-48 * time.Hour)})
	suite.Require().NoError(err)
	suite.Contains(rowFor(report, "Ticketed Venue", "page").LookupKey, "ticket:vendor.example")

	result, err := (&VenueAddressBackfill{DB: suite.db}).Apply(context.Background(), suite.roundTrip(report))
	suite.Require().NoError(err)
	suite.Equal(1, result.Written, "a show passing after the lookup does not change the row's key")
	suite.Equal("1 Main St", *suite.loadVenue(v.ID).Address)
}
