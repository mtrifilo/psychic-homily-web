package catalog

import (
	"reflect"
	"testing"
)

// TestEntityShowsLimitTagsAllow200 locks the per-entity shows endpoints
// (GET /venues/{id}/shows and GET /artists/{id}/shows) to a 200 limit cap so it
// isn't silently lowered again. These endpoints list a single entity's shows and
// can legitimately exceed 50 (e.g. a venue with 60+ upcoming shows). They are
// public reads rather than admin ones, so they don't belong in the admin
// offset-pagination guard even now that both of them offset. See PSY-1031.
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

// TestEntityShowsPaginationTags pins both entity show lists' paging params to
// the house tags (PSY-1750 for venues, PSY-1751 for artists). The offset tag
// matches the admin guard's word for word: an entity's archive is paged by the
// same clients as everything else, and a silently different default or floor
// there is a whole page of shows nobody can reach.
//
// Both are checked in one table for the same reason the limit cap above is: the
// artist and venue archives are one reading surface pointed at two entities, and
// a knob that drifts on one of them is a bug only that entity's readers hit.
//
// Year is bounded on purpose. It is fed straight into a Go time.Date used to
// build the query's coarse UTC bounds, and an unbounded int overflows the
// timestamp range Postgres accepts rather than returning an empty page. The only
// part of the two tags that legitimately differs is the sibling histogram path
// each one points the caller at.
func TestEntityShowsPaginationTags(t *testing.T) {
	const offsetTag = `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	yearTag := func(yearsPath string) string {
		return `query:"year" default:"0" minimum:"0" maximum:"9999" doc:"Filter to a single venue-local calendar year. 0 (default) returns every year. Use ` +
			yearsPath + ` to discover which years have shows."`
	}

	for _, tc := range []struct {
		name    string
		request any
		wantTag map[string]string
	}{
		{"venue shows", GetVenueShowsRequest{}, map[string]string{
			"Offset": offsetTag,
			"Year":   yearTag("/venues/{venue_id}/shows/years"),
		}},
		{"artist shows", GetArtistShowsRequest{}, map[string]string{
			"Offset": offsetTag,
			"Year":   yearTag("/artists/{artist_id}/shows/years"),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestType := reflect.TypeOf(tc.request)

			for _, name := range []string{"Offset", "Year"} {
				field, ok := requestType.FieldByName(name)
				if !ok {
					t.Fatalf("%s is missing %s field", requestType.Name(), name)
				}
				if got := string(field.Tag); got != tc.wantTag[name] {
					t.Fatalf("%s tag mismatch:\ngot:  %s\nwant: %s", name, got, tc.wantTag[name])
				}
			}
		})
	}
}
