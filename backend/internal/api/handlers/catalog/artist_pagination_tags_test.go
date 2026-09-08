package catalog

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// TestListArtistsPaginationTags pins GET /artists' paging knobs to the house
// tags (PSY-1774).
//
// It is a tag assertion rather than a behavioural one because huma enforces
// these BEFORE the handler runs, and the handler tests in this package call
// handlers directly — so a bound silently deleted from the struct would pass
// every one of them while the endpoint went back to answering with the whole
// catalogue, which is the state this endpoint had to be rescued from.
//
// The 200 ceiling matches the entity show lists rather than the 100 on
// GET /venues. A page of artists is one row per artist against those lists'
// full show bodies, and the browse page's first-screen seed is sized from this
// same knob, so the headroom is where a page-size change is absorbed.
//
// GET /venues, the structural twin of this request, has no such guard. Adding
// one belongs with a change to that endpoint, not to this one.
func TestListArtistsPaginationTags(t *testing.T) {
	wantTag := map[string]string{
		"Limit":  `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Maximum number of artists to return (max 200)"`,
		"Offset": `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`,
	}

	requestType := reflect.TypeOf(ListArtistsRequest{})

	// Errorf, not Fatalf: map iteration order is randomised, so aborting on the
	// first mismatch would report an arbitrary one of two drifted knobs and make
	// the failure output differ run to run.
	for name, want := range wantTag {
		field, ok := requestType.FieldByName(name)
		if !ok {
			t.Errorf("%s is missing the %s field", requestType.Name(), name)
			continue
		}
		if got := string(field.Tag); got != want {
			t.Errorf("%s tag mismatch:\ngot:  %s\nwant: %s", name, got, want)
		}
	}
}

// The `tags=` param carries a huma maxLength, which is the bound that makes an
// enormous filter cheap: it is enforced BEFORE the handler runs, so the string
// is never split into 60,000 slugs in the first place. The slug cap that
// follows it only bounds the query. Pinned here because deleting the tag is
// invisible to every handler test in this package — they bypass huma.
func TestListArtistsBoundsTheTagsParamLength(t *testing.T) {
	field, ok := reflect.TypeOf(ListArtistsRequest{}).FieldByName("Tags")
	if !ok {
		t.Fatal("ListArtistsRequest is missing the Tags field")
	}
	if got := field.Tag.Get("maxLength"); got != "512" {
		t.Fatalf("Tags maxLength = %q, want %q — without it an arbitrarily long tags= is parsed in full before any cap applies", got, "512")
	}
}

// The handler substitutes its own default for a zero Limit, for the callers
// that bypass huma. That substitution is only correct while it equals the tag's
// default — otherwise an HTTP caller and a direct caller get different page
// sizes from the same "unspecified" request, and the browse page's first-screen
// seed (which is sized from the HTTP default) would miss its cache entry.
func TestListArtistsDefaultLimitMatchesItsTag(t *testing.T) {
	field, ok := reflect.TypeOf(ListArtistsRequest{}).FieldByName("Limit")
	if !ok {
		t.Fatal("ListArtistsRequest is missing the Limit field")
	}
	// Compared against the CONSTANT, not a repeated literal: the tag string is
	// already pinned verbatim by the test above, so the only fact left to prove
	// is that the two sources of the default agree.
	if got, want := field.Tag.Get("default"), strconv.Itoa(defaultArtistListLimit); got != want {
		t.Fatalf("Limit default tag = %q, want %q (defaultArtistListLimit)", got, want)
	}
}

// The `missing=` filter's enum tag is what rejects an unrecognised value on the
// wire, before the handler runs. Pinned against the constant the handler reads:
// the two are one contract, and a tag deleted from the struct is invisible to
// every handler test in this package.
func TestListArtistsMissingFilterEnumMatchesItsConstant(t *testing.T) {
	field, ok := reflect.TypeOf(ListArtistsRequest{}).FieldByName("Missing")
	if !ok {
		t.Fatal("ListArtistsRequest is missing the Missing field")
	}
	if got := field.Tag.Get("query"); got != "missing" {
		t.Errorf("Missing query name = %q, want %q", got, "missing")
	}
	if got := field.Tag.Get("enum"); got != artistMissingListen {
		t.Errorf("Missing enum tag = %q, want %q (artistMissingListen)", got, artistMissingListen)
	}
}

// A value huma's enum did not reject — a caller building the request struct
// directly — fails closed. Ignoring it would answer with the whole city under a
// request that asked for one gap population, which is a wrong answer rather
// than a missing feature.
func TestListArtistsRejectsAnUnknownMissingFilter(t *testing.T) {
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(_ map[string]interface{}, _, _ int) ([]*contracts.ArtistWithShowCountResponse, int64, error) {
			t.Error("the service must not be reached for an unsupported missing filter")
			return nil, 0, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil, nil)

	_, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{Missing: "bandcamp"})
	if err == nil {
		t.Fatal("expected an error for an unsupported missing filter")
	}
	var status huma.StatusError
	if !errors.As(err, &status) {
		t.Fatalf("expected a huma.StatusError, got %T", err)
	}
	if status.GetStatus() != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", status.GetStatus(), http.StatusUnprocessableEntity)
	}
}

// The filter answers for at most ONE complete place. Every other shape is
// refused, because the `cities` parse discards what it cannot read and
// truncates past its cap: answering anyway would scope the list to a subset of
// the places the caller named, under a total the caller compares against the
// sum of their gap counts.
func TestListArtistsRejectsAPlaceItCannotScopeToAScene(t *testing.T) {
	cases := map[string]ListArtistsRequest{
		"single city without a state": {Missing: artistMissingListen, City: "Phoenix"},
		"cities with no comma":        {Missing: artistMissingListen, Cities: "Phoenix"},
		"cities with an empty state":  {Missing: artistMissingListen, Cities: "Phoenix,"},
		"cities with an empty city":   {Missing: artistMissingListen, Cities: ",AZ"},
		"two cities":                  {Missing: artistMissingListen, Cities: "Phoenix,AZ|Tucson,AZ"},
		"one complete, one not":       {Missing: artistMissingListen, Cities: "Phoenix,AZ|Tucson"},
	}
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(_ map[string]interface{}, _, _ int) ([]*contracts.ArtistWithShowCountResponse, int64, error) {
			t.Error("the service must not be reached for a place with no scene")
			return nil, 0, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil, nil)

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.ListArtistsHandler(context.Background(), &req)
			var status huma.StatusError
			if !errors.As(err, &status) {
				t.Fatalf("expected a huma.StatusError, got %v", err)
			}
			if status.GetStatus() != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", status.GetStatus(), http.StatusUnprocessableEntity)
			}
		})
	}
}

// The same shapes are fine WITHOUT the filter: the guard is the filter's, not a
// new bound on the city params.
func TestListArtistsStillAcceptsThoseShapesWithoutTheFilter(t *testing.T) {
	cases := map[string]ListArtistsRequest{
		"stateless city": {City: "Phoenix"},
		"two cities":     {Cities: "Phoenix,AZ|Tucson,AZ"},
		"unparseable":    {Cities: "Phoenix"},
	}
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(_ map[string]interface{}, _, _ int) ([]*contracts.ArtistWithShowCountResponse, int64, error) {
			return nil, 0, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil, nil)

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := h.ListArtistsHandler(context.Background(), &req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// A state with no city names no scene either, but it also names no city, so it
// is a place-less request rather than an unscopeable one: the gap filter still
// applies, across every band in the state.
func TestListArtistsAcceptsAStateOnlyRequestUnderTheFilter(t *testing.T) {
	var got map[string]interface{}
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(filters map[string]interface{}, _, _ int) ([]*contracts.ArtistWithShowCountResponse, int64, error) {
			got = filters
			return nil, 0, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil, nil)

	if _, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{
		Missing: artistMissingListen,
		State:   "AZ",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if engaged, _ := got[catalog.FilterMissingListenLink].(bool); !engaged {
		t.Fatalf("filters = %v, want %s set", got, catalog.FilterMissingListenLink)
	}
}

// The supported value reaches the service as the filter key the browse scope
// reads. The handler and the scope spell that key once
// (catalog.FilterMissingListenLink); this pins that the handler sets it, which
// no other test in this package would notice.
func TestListArtistsPassesTheMissingListenFilterThrough(t *testing.T) {
	var got map[string]interface{}
	mock := &testhelpers.MockArtistService{
		GetArtistsWithShowCountsFn: func(filters map[string]interface{}, _, _ int) ([]*contracts.ArtistWithShowCountResponse, int64, error) {
			got = filters
			return nil, 0, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil, nil)

	if _, err := h.ListArtistsHandler(context.Background(), &ListArtistsRequest{
		Missing: artistMissingListen,
		Cities:  "Phoenix,AZ",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if engaged, _ := got[catalog.FilterMissingListenLink].(bool); !engaged {
		t.Fatalf("filters = %v, want %s set", got, catalog.FilterMissingListenLink)
	}
}
