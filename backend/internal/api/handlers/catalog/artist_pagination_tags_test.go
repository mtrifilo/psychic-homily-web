package catalog

import (
	"reflect"
	"strconv"
	"testing"
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
