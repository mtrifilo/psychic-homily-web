package catalog

import (
	"reflect"
	"testing"
)

// TestEntityShowsLimitTagsAllow200 locks the per-entity shows endpoints
// (GET /venues/{id}/shows and GET /artists/{id}/shows) to a 200 limit cap so it
// isn't silently lowered again. These endpoints list a single entity's shows and
// can legitimately exceed 50 (e.g. a venue with 60+ upcoming shows). They are
// limit/time_filter paged (no offset), so they don't belong in the admin
// offset-pagination guard. See PSY-1031.
func TestEntityShowsLimitTagsAllow200(t *testing.T) {
	// Both endpoints share one limit contract — lock them to it together so the
	// cap can't be silently lowered or drift apart between the two.
	const expectedTag = `query:"limit" default:"20" minimum:"1" maximum:"200" doc:"Maximum number of shows to return (max 200)"`

	for _, tc := range []struct {
		name    string
		request any
	}{
		{"venue shows", GetVenueShowsRequest{}},
		{"artist shows", GetArtistShowsRequest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestType := reflect.TypeOf(tc.request)

			limitField, ok := requestType.FieldByName("Limit")
			if !ok {
				t.Fatalf("%s is missing Limit field", requestType.Name())
			}
			if got := string(limitField.Tag); got != expectedTag {
				t.Fatalf("Limit tag mismatch:\ngot:  %s\nwant: %s", got, expectedTag)
			}
		})
	}
}
