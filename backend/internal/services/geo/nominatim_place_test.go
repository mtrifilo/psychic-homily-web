package geo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPlaceQueryTextAndKey(t *testing.T) {
	q := PlaceQuery{Name: " Lincoln Hall ", City: "Chicago", State: "IL", CountryCode: "us"}
	if got := q.Text(); got != "Lincoln Hall, Chicago, IL" {
		t.Errorf("Text() = %q", got)
	}
	if got := q.Key(); got != "Lincoln Hall, Chicago, IL [US]" {
		t.Errorf("Key() = %q", got)
	}
	if got := (PlaceQuery{Name: "Boom", City: "Leeds"}).Key(); got != "Boom, Leeds" {
		t.Errorf("Key() without a country = %q", got)
	}
}

func TestSearchPlaces_FreeFormQueryAndParse(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = map[string]string{}
		for k := range r.URL.Query() {
			got[k] = r.URL.Query().Get(k)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
		  {"lat":"41.925","lon":"-87.649","osm_type":"node","category":"amenity","type":"music_venue","addresstype":"amenity","place_rank":30,
		   "name":"Lincoln Hall","display_name":"Lincoln Hall, 2424, North Lincoln Avenue, Lincoln Park, Chicago, Cook County, Illinois, 60614, United States",
		   "address":{"house_number":"2424","road":"North Lincoln Avenue","suburb":"Lincoln Park","city":"Chicago","state":"Illinois","ISO3166-2-lvl4":"US-IL","postcode":"60614","country_code":"us"}},
		  {"lat":"bad","lon":"-87","category":"highway","type":"residential","name":"Lincoln Avenue"}
		]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	cands, err := c.SearchPlaces(context.Background(), PlaceQuery{Name: "Lincoln Hall", City: "Chicago", State: "IL", CountryCode: "US"})
	if err != nil {
		t.Fatalf("SearchPlaces: %v", err)
	}
	if got["q"] != "Lincoln Hall, Chicago, IL" || got["countrycodes"] != "us" || got["addressdetails"] != "1" || got["format"] != "jsonv2" {
		t.Fatalf("query params = %v", got)
	}
	if len(cands) != 1 {
		t.Fatalf("want the unparseable row dropped, got %d candidates", len(cands))
	}
	cand := cands[0]
	if cand.Name != "Lincoln Hall" || cand.HouseNumber != "2424" || cand.Road != "North Lincoln Avenue" ||
		cand.StateCode != "US-IL" || cand.CountryCode != "us" || cand.Category != "amenity" || cand.Type != "music_venue" {
		t.Fatalf("candidate = %+v", cand)
	}
	if len(cand.Localities) != 2 || cand.Localities[0] != "Chicago" || cand.Localities[1] != "Lincoln Park" {
		t.Fatalf("localities = %v", cand.Localities)
	}
}

func TestSearchPlaces_EmptyNameMakesNoRequest(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()
	cands, err := newTestClient(srv.URL).SearchPlaces(context.Background(), PlaceQuery{City: "Chicago"})
	if err != nil || cands != nil || calls != 0 {
		t.Fatalf("cands=%v err=%v calls=%d", cands, err, calls)
	}
}

func TestSearchPlaces_RetriesThenFails(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	_, err := newTestClient(srv.URL).SearchPlaces(context.Background(), PlaceQuery{Name: "X", City: "Y"})
	if err == nil {
		t.Fatal("want an error for a 400")
	}
	if calls != 1 {
		t.Fatalf("a non-retryable status must not be retried; calls = %d", calls)
	}
}

func TestResolveCountryISO(t *testing.T) {
	tests := []struct{ state, country, want string }{
		{"IL", "", "US"},
		{"England", "United Kingdom", "GB"},
		{"", "Germany", "DE"},
		{"ON", "", "CA"},
		{"", "", ""},
	}
	for _, tt := range tests {
		if got := ResolveCountryISO(tt.state, tt.country); got != tt.want {
			t.Errorf("ResolveCountryISO(%q, %q) = %q, want %q", tt.state, tt.country, got, tt.want)
		}
	}
}
