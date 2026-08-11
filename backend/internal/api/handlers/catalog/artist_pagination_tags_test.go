package catalog

import (
	"reflect"
	"testing"
)

// TestListArtistsPaginationTags pins GET /artists' paging knobs to the house
// tags (PSY-1774).
//
// It is a tag assertion rather than a behavioural one because huma enforces
// these BEFORE the handler runs, and the handler tests in this package call
// handlers directly — so a bound silently deleted from the struct would pass
// every one of them while the endpoint went back to answering with the whole
// catalogue. That is precisely how the endpoint reached 3.17 MB and a 502
// through the proxy: nothing in the suite could tell a missing bound from an
// unenforced one.
//
// The 200 ceiling matches the entity show lists rather than the 100 on
// GET /venues. A page of artists is one row per artist against those lists'
// full show bodies, and the browse page's first-screen seed is sized from this
// same knob, so the headroom is where a page-size change is absorbed.
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
	if got := field.Tag.Get("default"); got != "50" {
		t.Fatalf("Limit default tag = %q, want %q", got, "50")
	}
	if defaultArtistListLimit != 50 {
		t.Fatalf("defaultArtistListLimit = %d, want it equal to the tag's 50", defaultArtistListLimit)
	}
}
