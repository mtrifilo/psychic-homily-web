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
	wantKey := "https://venue.example/calendar | ticket:tickets.example | ticket:other.example | ticket:third.example | ticket:fourth.example"
	if key := pageLookupKey(got, true); key != wantKey {
		t.Fatalf("pageLookupKey = %q", key)
	}
	// A later event on the same vendor keeps the key; schema-only runs key apart.
	later := pageSources(v, "https://venue.example/calendar", []string{
		"https://tickets.example/z", "https://other.example/y", "https://third.example/x", "https://fourth.example/w",
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
		DryRun:      true,
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
		"# Venue address backfill (DRY RUN)",
		"| page | 2 | 1 | 50% | 1 | 0 | 0 | 0 |",
		"| name | 1 | 1 | 100% |",
		"| name_search | 1 |",
		"Pipe \\| Venue",
		"2424 North Lincoln Avenue (OSM: Lincoln Hall)",
		"Would write: 2. Written: 0.",
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
// Backfill runs against a real database (VenueServiceIntegrationTestSuite)
// =============================================================================

// stubPageFinder answers per venue name; onCall runs before answering so a
// test can land a concurrent write mid-run.
type stubPageFinder struct {
	results map[string]venueaddress.Result
	err     error
	calls   int
	sources map[string][]venueaddress.Source
	onCall  func(name string)
}

func (s *stubPageFinder) Find(_ context.Context, v venueaddress.Venue, sources []venueaddress.Source) (venueaddress.Result, error) {
	s.calls++
	if s.sources == nil {
		s.sources = map[string][]venueaddress.Source{}
	}
	s.sources[v.Name] = sources
	if s.onCall != nil {
		s.onCall(v.Name)
	}
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

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_DryRunLooksUpButWritesNothing() {
	page := suite.seedAddresslessVenue("Page Venue", "Chicago", "IL", "https://page.example", 48*time.Hour)
	named := suite.seedAddresslessVenue("Lincoln Hall", "Chicago", "IL", "", 72*time.Hour)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{"Page Venue": pageHit("1 Main St", "https://page.example")}}
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln Hall": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop)}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal(2, report.Candidates)
	suite.Equal(2, report.WouldWrite)
	suite.Equal(0, report.Written)
	suite.Equal(1, report.Phases["page"].Hits)
	suite.Equal(1, report.Phases["page"].NoSource, "the named venue has no page")
	suite.Equal(1, report.Phases["name"].Hits)
	suite.Equal(map[string]int{geo.PrecisionRooftop: 1, geo.PrecisionNameSearch: 1}, report.Precision)
	suite.Require().Len(report.Rows, 2)
	suite.Equal("Page Venue", report.Rows[0].Name, "the venue with the sooner show goes first")
	suite.Equal("2424 North Lincoln Avenue", report.Rows[1].Address)

	for _, id := range []uint{page.ID, named.ID} {
		v := suite.loadVenue(id)
		suite.Nil(v.Address, "dry run must not write")
		suite.Nil(v.StreetLatitude)
		suite.Empty(suite.lookups(id), "dry run must not record lookups")
	}
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ConfirmWritesBothPhases() {
	page := suite.seedAddresslessVenue("Page Venue", "Chicago", "IL", "https://page.example", 48*time.Hour)
	named := suite.seedAddresslessVenue("Lincoln Hall", "Chicago", "IL", "", 72*time.Hour)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{"Page Venue": pageHit("1 Main St", "https://page.example")}}
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln Hall": {lincolnHallCandidate()}}}
	geocoder := hitStub(41.9, -87.6, geo.PrecisionRooftop)
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places, Geocoder: geocoder}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(2, report.Written)

	v := suite.loadVenue(page.ID)
	suite.Require().NotNil(v.Address)
	suite.Equal("1 Main St", *v.Address)
	suite.Require().NotNil(v.GeocodePrecision)
	suite.Equal(geo.PrecisionRooftop, *v.GeocodePrecision)
	suite.Equal("1 Main St, Chicago, IL", *v.GeocodedAddress)
	suite.InDelta(41.85, *v.Latitude, 1e-6, "the city centroid is never touched")
	l := suite.lookups(page.ID)["page"]
	suite.Equal(catalogm.VenueAddressOutcomeHit, l.Outcome)
	suite.Equal("https://page.example", *l.Source)

	n := suite.loadVenue(named.ID)
	suite.Require().NotNil(n.Address)
	suite.Equal("2424 North Lincoln Avenue", *n.Address)
	suite.Equal(geo.PrecisionNameSearch, *n.GeocodePrecision)
	suite.InDelta(41.9258, *n.StreetLatitude, 1e-6)
	suite.InDelta(41.85, *n.Latitude, 1e-6, "the city centroid is never touched")
	suite.True(streetGeocodeFresh(n), "the stored key matches the new address, so the point is served")

	// The street-geocode sweep treats both as already attempted: no lookups.
	sweepGeocoder := hitStub(0, 0, geo.PrecisionRooftop)
	sweep, err := BackfillVenueStreetGeocodes(context.Background(), suite.db, sweepGeocoder, StreetGeocodeOptions{})
	suite.Require().NoError(err)
	suite.Equal(0, sweepGeocoder.calls)
	suite.Equal(2, sweep.Unchanged)

	// A second backfill run selects nothing: both venues now have an address.
	report2, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(0, report2.Candidates)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_NeverOverwritesAnAddress() {
	addr := "500 Existing Ave"
	kept := &catalogm.Venue{Name: "Has Address", City: "Chicago", State: "IL", Address: &addr}
	suite.Require().NoError(suite.db.Create(kept).Error)
	racing := suite.seedAddresslessVenue("Racing Venue", "Chicago", "IL", "https://race.example", 0)

	// While the lookup is in flight, an editor gives the venue an address.
	finder := &stubPageFinder{
		results: map[string]venueaddress.Result{"Racing Venue": pageHit("1 Main St", "https://race.example")},
		onCall: func(string) {
			suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", racing.ID).
				Update("address", "77 Editor St").Error)
		},
	}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop)}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Candidates, "a venue with an address is never selected")
	suite.Equal(0, report.Written)
	suite.Contains(strings.Join(report.Rows[0].Notes, " "), "venue changed since it was read")

	suite.Equal("77 Editor St", *suite.loadVenue(racing.ID).Address)
	suite.Equal("500 Existing Ave", *suite.loadVenue(kept.ID).Address)
	suite.Empty(suite.lookups(racing.ID), "a skipped write records nothing")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_MissesAreRecordedAndSkippedUntilInputsChange() {
	v := suite.seedAddresslessVenue("Nowhere Club", "Chicago", "IL", "https://nowhere.example", 0)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{}}
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: places}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Phases["page"].Misses)
	suite.Equal(1, report.Phases["name"].Misses)
	l := suite.lookups(v.ID)
	suite.Equal("https://nowhere.example | [schema-only]", l["page"].LookupKey)
	suite.Equal("Nowhere Club, Chicago, IL [US]", l["name"].LookupKey)
	suite.Equal(catalogm.VenueAddressOutcomeMiss, l["name"].Outcome)

	// Unchanged inputs: both phases skipped, no network calls, no limit spent.
	report2, err := run.Run(context.Background(), VenueAddressBackfillOptions{Limit: 1})
	suite.Require().NoError(err)
	suite.Equal(1, finder.calls)
	suite.Equal(1, places.calls)
	suite.Equal(0, report2.Processed)
	suite.Equal(1, report2.Phases["page"].SkippedMemo)
	suite.Equal(1, report2.Phases["name"].SkippedMemo)

	// A new website changes the page key: the page phase runs again, the name
	// phase stays skipped.
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", v.ID).
		Update("website", "https://nowhere.example/contact").Error)
	_, err = run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(2, finder.calls)
	suite.Equal(1, places.calls)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ErrorsAreNotRecorded() {
	v := suite.seedAddresslessVenue("Flaky Venue", "Chicago", "IL", "https://flaky.example", 0)
	pageless := suite.seedAddresslessVenue("Pageless Venue", "Chicago", "IL", "", 0)
	run := &VenueAddressBackfill{
		DB:     suite.db,
		Pages:  &stubPageFinder{err: errors.New("page timed out")},
		Places: &stubPlaces{err: errors.New("nominatim: status 503")},
	}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Phases["page"].Errors)
	suite.Equal(1, report.Phases["name"].Errors, "only the pageless venue reaches the name search")
	suite.Len(report.Errors, 2)
	suite.Empty(suite.lookups(v.ID), "an error must stay retryable")
	suite.Empty(suite.lookups(pageless.ID), "an error must stay retryable")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_FiltersOrderAndLimit() {
	suite.seedAddresslessVenue("Later Show", "Chicago", "IL", "", 30*24*time.Hour)
	suite.seedAddresslessVenue("No Show", "Chicago", "IL", "", 0)
	suite.seedAddresslessVenue("Far Show", "Chicago", "IL", "", 200*24*time.Hour)
	suite.seedAddresslessVenue("Soon Show", "Chicago", "IL", "", 24*time.Hour)
	suite.seedAddresslessVenue("Elsewhere", "Milwaukee", "WI", "", 24*time.Hour)
	places := &stubPlaces{}
	run := &VenueAddressBackfill{DB: suite.db, Places: places}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{DryRun: true, OnlyUpcoming: true, City: "chicago"})
	suite.Require().NoError(err)
	suite.Equal(2, report.Candidates, "a show beyond 90 days does not make a venue upcoming")
	suite.Equal([]string{"Soon Show", "Later Show"}, rowNames(report))

	report, err = run.Run(context.Background(), VenueAddressBackfillOptions{DryRun: true, City: "Chicago", Limit: 3})
	suite.Require().NoError(err)
	suite.Equal(4, report.Candidates)
	suite.True(report.LimitHit)
	suite.Equal([]string{"Soon Show", "Later Show", "No Show"}, rowNames(report),
		"upcoming venues first by show date, the rest by id")
}

func rowNames(r *VenueAddressReport) []string {
	var names []string
	for _, row := range r.Rows {
		names = append(names, row.Name)
	}
	return names
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_PageSourcesFromSourceConfigsAndTickets() {
	v := suite.seedAddresslessVenue("Sourced Venue", "Chicago", "IL", "https://sourced.example", 0)
	src := "https://sourced.example/events"
	suite.Require().NoError(suite.db.Create(&adminm.SourceConfig{EntityType: "venue", EntityID: v.ID, SourceURL: &src}).Error)
	defer suite.db.Exec("DELETE FROM source_configs")
	ticket := "https://tickets.example/e/1"
	show := &catalogm.Show{Title: "t", EventDate: time.Now().Add(400 * 24 * time.Hour), Status: catalogm.ShowStatusApproved, TicketURL: &ticket, Source: catalogm.ShowSourceDiscovery}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: v.ID}).Error)
	cancelledURL := "https://cancelled.example/e/2"
	cancelled := &catalogm.Show{Title: "c", EventDate: time.Now().Add(24 * time.Hour), Status: catalogm.ShowStatusApproved, TicketURL: &cancelledURL, IsCancelled: true}
	suite.Require().NoError(suite.db.Create(cancelled).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: cancelled.ID, VenueID: v.ID}).Error)

	finder := &stubPageFinder{results: map[string]venueaddress.Result{}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder}
	_, err := run.Run(context.Background(), VenueAddressBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal([]venueaddress.Source{
		{URL: "https://sourced.example", Kind: venueaddress.SourceWebsite},
		{URL: "https://sourced.example/events", Kind: venueaddress.SourceIngest},
		{URL: "https://tickets.example/e/1", Kind: venueaddress.SourceTicket},
	}, finder.sources["Sourced Venue"])
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_SchemaAcceptsNameSearchPrecision() {
	v := suite.seedAddresslessVenue("Schema Venue", "Chicago", "IL", "", 0)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", v.ID).
		Update("geocode_precision", geo.PrecisionNameSearch).Error)
	err := suite.db.Model(&catalogm.Venue{}).Where("id = ?", v.ID).Update("geocode_precision", "guess").Error
	suite.Error(err, "the check constraint still refuses values outside the vocabulary")

	suite.Require().NoError(recordAddressLookup(suite.db, v.ID, "name", "k1", "miss", "", ""))
	suite.Require().NoError(recordAddressLookup(suite.db, v.ID, "name", "k2", "hit", "src", "1 Main St"))
	l := suite.lookups(v.ID)["name"]
	suite.Equal("k2", l.LookupKey, "a second attempt replaces the phase row")
	suite.Equal("hit", l.Outcome)
	suite.Error(recordAddressLookup(suite.db, v.ID, "phone", "k", "miss", "", ""), "phase is constrained")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_UnmigratedDatabaseDryRunsButRefusesLive() {
	suite.seedAddresslessVenue("Old Schema Venue", "Chicago", "IL", "", 0)
	tx := suite.db.Begin()
	defer tx.Rollback()
	suite.Require().NoError(tx.Exec("DROP TABLE venue_address_lookups").Error)
	run := &VenueAddressBackfill{DB: tx, Places: &stubPlaces{}}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.True(report.LookupsTableMissing)
	suite.Equal(1, report.Phases["name"].Attempted)

	_, err = run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.ErrorContains(err, "apply the database migrations")
}

// TestAddressBackfill_NominatimStubEndToEnd drives the name phase through the
// real Nominatim client against a local stub server: request shape, parsing,
// acceptance, and the stored result.
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
	suite.Require().Len(report.Rows, 1)
	suite.Contains(report.Rows[0].Notes[0], "rejected highway=residential")

	got := suite.loadVenue(v.ID)
	suite.Require().NotNil(got.Address)
	suite.Equal("2424 North Lincoln Avenue", *got.Address)
	suite.InDelta(41.9258, *got.StreetLatitude, 1e-6)
	suite.Equal(geo.PrecisionNameSearch, *got.GeocodePrecision)
	l := suite.lookups(v.ID)["name"]
	suite.Equal("Lincoln Hall, Chicago, IL [US]", l.LookupKey)
	suite.Contains(*l.Source, "2424, North Lincoln Avenue")
}

func TestStubPlacesSatisfiesInterface(t *testing.T) {
	var _ geo.PlaceSearcher = (*stubPlaces)(nil)
	var _ PageAddressFinder = (*stubPageFinder)(nil)
	var _ PageAddressFinder = (*venueaddress.Finder)(nil)
	var _ geo.PlaceSearcher = (*geo.NominatimClient)(nil)
}

// --- round-trip rules added with the review gate ---

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_PageErrorHoldsTheNameSearchBack() {
	v := suite.seedAddresslessVenue("Flaky Page Venue", "Chicago", "IL", "https://flaky.example", 0)
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Flaky Page Venue": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: &stubPageFinder{err: errors.New("timeout")}, Places: places}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(0, places.calls, "the name search waits until the venue's own page is decided")
	suite.Equal(1, report.Phases["page"].Errors)
	suite.Nil(suite.loadVenue(v.ID).Address)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ClearedAfterFillIsNeverRefilled() {
	v := suite.seedAddresslessVenue("Cleared Venue", "Chicago", "IL", "https://cleared.example", 0)
	suite.Require().NoError(recordAddressLookup(suite.db, v.ID, "page", "https://cleared.example", "hit", "https://cleared.example", "1 Main St"))
	finder := &stubPageFinder{results: map[string]venueaddress.Result{"Cleared Venue": pageHit("1 Main St", "https://cleared.example")}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Places: &stubPlaces{}}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.SkippedCleared)
	suite.Equal(0, finder.calls)
	suite.Nil(suite.loadVenue(v.ID).Address)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ReviewMatchesNeedApproval() {
	v := suite.seedAddresslessVenue("Lincoln", "Chicago", "IL", "", 0) // partial match of "Lincoln Hall"
	places := &stubPlaces{results: map[string][]geo.PlaceCandidate{"Lincoln": {lincolnHallCandidate()}}}
	run := &VenueAddressBackfill{DB: suite.db, Places: places}

	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{})
	suite.Require().NoError(err)
	suite.Require().Len(report.Rows, 1)
	suite.True(report.Rows[0].Review)
	suite.False(report.Rows[0].Written, "a REVIEW match is not written without approval")
	suite.Nil(suite.loadVenue(v.ID).Address)

	approved := ApprovalsFromReport(report)
	report, err = run.Run(context.Background(), VenueAddressBackfillOptions{Approved: approved})
	suite.Require().NoError(err)
	suite.True(report.Rows[0].Written)
	suite.Equal("2424 North Lincoln Avenue", *suite.loadVenue(v.ID).Address)
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_ApprovedReportBindsTheLiveRun() {
	a := suite.seedAddresslessVenue("Approved Venue", "Chicago", "IL", "https://a.example", 0)
	b := suite.seedAddresslessVenue("Unlisted Venue", "Chicago", "IL", "https://b.example", 0)
	changed := suite.seedAddresslessVenue("Changed Venue", "Chicago", "IL", "https://c.example", 0)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{
		"Approved Venue": pageHit("1 Main St", "https://a.example"),
		"Unlisted Venue": pageHit("2 Main St", "https://b.example"),
		"Changed Venue":  pageHit("3 New St", "https://c.example"),
	}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder, Geocoder: hitStub(41.9, -87.6, geo.PrecisionRooftop)}
	approved := map[VenueAddressApproval]bool{
		{VenueID: a.ID, Phase: "page", Address: "1 Main St"}:      true,
		{VenueID: changed.ID, Phase: "page", Address: "3 Old St"}: true, // the page now says something else
	}
	report, err := run.Run(context.Background(), VenueAddressBackfillOptions{Approved: approved})
	suite.Require().NoError(err)
	suite.Equal(2, report.Candidates, "only approved venues are considered")
	suite.Equal(1, report.Written)
	suite.Equal("1 Main St", *suite.loadVenue(a.ID).Address)
	suite.Nil(suite.loadVenue(b.ID).Address)
	suite.Nil(suite.loadVenue(changed.ID).Address, "an address other than the approved one is not written")
}

func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_TicketPagesOnlyFromTrustedSubmitters() {
	v := suite.seedAddresslessVenue("Ticketed Venue", "Chicago", "IL", "", 0)
	untrusted := suite.createTestUser()
	trusted := suite.createTestUser()
	suite.Require().NoError(suite.db.Model(trusted).Update("user_tier", "trusted_contributor").Error)
	for _, s := range []struct {
		user uint
		url  string
	}{{untrusted.ID, "https://evil.example/e/1"}, {trusted.ID, "https://good.example/e/2"}} {
		u, url := s.user, s.url
		show := &catalogm.Show{Title: "t", EventDate: time.Now().Add(48 * time.Hour), Status: catalogm.ShowStatusApproved, TicketURL: &url, SubmittedBy: &u}
		suite.Require().NoError(suite.db.Create(show).Error)
		suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: v.ID}).Error)
	}
	finder := &stubPageFinder{results: map[string]venueaddress.Result{}}
	run := &VenueAddressBackfill{DB: suite.db, Pages: finder}
	_, err := run.Run(context.Background(), VenueAddressBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal([]venueaddress.Source{{URL: "https://good.example/e/2", Kind: venueaddress.SourceTicket}}, finder.sources["Ticketed Venue"])
}

// missStub and errStub cover the page phase's other geocode outcomes.
func (suite *VenueServiceIntegrationTestSuite) TestAddressBackfill_PageGeocodeMissErrorAndNoGeocoder() {
	missV := suite.seedAddresslessVenue("Geo Miss", "Chicago", "IL", "https://m.example", 0)
	errV := suite.seedAddresslessVenue("Geo Error", "Chicago", "IL", "https://e.example", 0)
	noneV := suite.seedAddresslessVenue("Geo None", "Chicago", "IL", "https://n.example", 0)
	finder := &stubPageFinder{results: map[string]venueaddress.Result{
		"Geo Miss":  pageHit("1 Miss St", "https://m.example"),
		"Geo Error": pageHit("2 Error St", "https://e.example"),
		"Geo None":  pageHit("3 None St", "https://n.example"),
	}}
	miss := &stubAddressGeocoder{}
	_, err := (&VenueAddressBackfill{DB: suite.db, Pages: finder, Geocoder: miss}).Run(context.Background(),
		VenueAddressBackfillOptions{Approved: map[VenueAddressApproval]bool{{VenueID: missV.ID, Phase: "page", Address: "1 Miss St"}: true}})
	suite.Require().NoError(err)
	failing := &stubAddressGeocoder{err: errors.New("nominatim: status 503")}
	_, err = (&VenueAddressBackfill{DB: suite.db, Pages: finder, Geocoder: failing}).Run(context.Background(),
		VenueAddressBackfillOptions{Approved: map[VenueAddressApproval]bool{{VenueID: errV.ID, Phase: "page", Address: "2 Error St"}: true}})
	suite.Require().NoError(err)
	_, err = (&VenueAddressBackfill{DB: suite.db, Pages: finder}).Run(context.Background(),
		VenueAddressBackfillOptions{Approved: map[VenueAddressApproval]bool{{VenueID: noneV.ID, Phase: "page", Address: "3 None St"}: true}})
	suite.Require().NoError(err)

	m := suite.loadVenue(missV.ID)
	suite.Equal("1 Miss St", *m.Address)
	suite.Require().NotNil(m.GeocodedAddress)
	suite.Equal("1 Miss St, Chicago, IL", *m.GeocodedAddress, "a clean miss stores the miss memo for the new address")
	suite.Nil(m.StreetLatitude)
	e := suite.loadVenue(errV.ID)
	suite.Equal("2 Error St", *e.Address)
	suite.Nil(e.GeocodedAddress, "a geocode error stores no memo, so the sweep retries")
	n := suite.loadVenue(noneV.ID)
	suite.Equal("3 None St", *n.Address)
	suite.Nil(n.GeocodedAddress)

	sweepGeocoder := hitStub(41.9, -87.6, geo.PrecisionRooftop)
	_, err = BackfillVenueStreetGeocodes(context.Background(), suite.db, sweepGeocoder, StreetGeocodeOptions{})
	suite.Require().NoError(err)
	suite.Equal(2, sweepGeocoder.calls, "the sweep geocodes the error and no-geocoder venues and skips the recorded miss")
}
