package catalog

import (
	"context"
	"reflect"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

// The city-facet endpoints read the same filter vocabulary as the lists beside
// them, so what each handler hands its service has to be what the list handler
// would have built from the same query string. These pin the threading and the
// two deliberate omissions: no place filter reaches the facet, and the tag list
// is bounded exactly where its own list bounds it.

func TestGetVenueCitiesHandler_ThreadsTheTagFilter(t *testing.T) {
	var got contracts.VenueListFilters
	mock := &testhelpers.MockVenueService{
		GetVenueCitiesFn: func(filters contracts.VenueListFilters) ([]*contracts.VenueCityResponse, error) {
			got = filters
			return nil, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)

	_, err := h.GetVenueCitiesHandler(context.Background(), &GetVenueCitiesRequest{
		Tags: "Post-Punk, shoegaze", TagMatch: "any",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got.TagSlugs, []string{"post-punk", "shoegaze"}) {
		t.Errorf("tag slugs: got %v, want [post-punk shoegaze]", got.TagSlugs)
	}
	if !got.TagMatchAny {
		t.Error("tag_match=any should reach the service as TagMatchAny")
	}
}

func TestGetVenueCitiesHandler_UnfilteredSendsNoTags(t *testing.T) {
	var got contracts.VenueListFilters
	mock := &testhelpers.MockVenueService{
		GetVenueCitiesFn: func(filters contracts.VenueListFilters) ([]*contracts.VenueCityResponse, error) {
			got = filters
			return nil, nil
		},
	}
	h := NewVenueHandler(mock, nil, nil, nil)

	if _, err := h.GetVenueCitiesHandler(context.Background(), &GetVenueCitiesRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.TagSlugs) != 0 {
		t.Errorf("an unfiltered request must carry no tag slugs, got %v", got.TagSlugs)
	}
}

func TestGetArtistCitiesHandler_TagFilterDropsTheActivityGate(t *testing.T) {
	var got map[string]interface{}
	mock := &testhelpers.MockArtistService{
		GetArtistCitiesFn: func(filters map[string]interface{}) ([]*contracts.ArtistCityResponse, error) {
			got = filters
			return nil, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil, nil)

	_, err := h.GetArtistCitiesHandler(context.Background(), &GetArtistCitiesRequest{Tags: "shoegaze"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["tag_filter"]; !ok {
		t.Fatal("the tag filter must reach the service")
	}
	// The pairing is the point: the list drops the gate under a tag filter, so a
	// facet that kept it would count a narrower set than the list it filters.
	if skip, _ := got["skip_active_filter"].(bool); !skip {
		t.Error("a tag filter must carry skip_active_filter, as the list handler does")
	}
}

func TestGetArtistCitiesHandler_UnfilteredKeepsTheActivityGate(t *testing.T) {
	var got map[string]interface{}
	mock := &testhelpers.MockArtistService{
		GetArtistCitiesFn: func(filters map[string]interface{}) ([]*contracts.ArtistCityResponse, error) {
			got = filters
			return nil, nil
		},
	}
	h := NewArtistHandler(mock, nil, nil, nil)

	if _, err := h.GetArtistCitiesHandler(context.Background(), &GetArtistCitiesRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["skip_active_filter"]; ok {
		t.Error("an unfiltered facet must keep the activity gate")
	}
}

// The window parameters are declared twice, once per request struct, because
// huma builds an operation's parameters from its request type's OWN fields and
// does not promote an embedded struct's: sharing them through embedding drops
// the window from the published document while both handlers keep working, so
// the generated client stops being able to address a window at all.
//
// Two copies therefore have to be held together by a test rather than by the
// type system. Tag-for-tag, because the bounds and the docs are the contract the
// OpenAPI document publishes, and a facet that accepted a window the list
// refuses is the disagreement this endpoint pair exists to prevent.
func TestShowCalendarWindowParamsMatch(t *testing.T) {
	list := reflect.TypeOf(GetShowsCalendarRequest{})
	facet := reflect.TypeOf(GetShowCitiesRequest{})

	for _, name := range []string{"Year", "Month", "Day", "Days"} {
		listField, ok := list.FieldByName(name)
		if !ok {
			t.Fatalf("GetShowsCalendarRequest lost its %s field", name)
		}
		facetField, ok := facet.FieldByName(name)
		if !ok {
			t.Fatalf("GetShowCitiesRequest lost its %s field", name)
		}
		if listField.Tag != facetField.Tag {
			t.Errorf("%s tags drifted:\n  list:  %s\n  facet: %s", name, listField.Tag, facetField.Tag)
		}
	}
}

func TestGetShowCitiesHandler_ThreadsTheTagFilterAndWindow(t *testing.T) {
	var gotFilters *contracts.UpcomingShowsFilter
	var gotWindow contracts.ShowCalendarWindow
	mock := &testhelpers.MockShowService{
		GetShowCitiesFn: func(
			_ string, filters *contracts.UpcomingShowsFilter, window contracts.ShowCalendarWindow,
		) ([]contracts.ShowCityResponse, error) {
			gotFilters, gotWindow = filters, window
			return nil, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, testhelpers.AllShowsVisible())

	_, err := h.GetShowCitiesHandler(context.Background(), &GetShowCitiesRequest{
		Year: 2026, Month: 9, Day: 15, Days: 3, Tags: "post-punk",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotFilters == nil || !reflect.DeepEqual(gotFilters.TagSlugs, []string{"post-punk"}) {
		t.Fatalf("tag slugs did not reach the service: %+v", gotFilters)
	}
	want := contracts.ShowCalendarWindow{Year: 2026, Month: 9, Day: 15, Days: 3}
	if gotWindow != want {
		t.Errorf("window: got %+v, want %+v", gotWindow, want)
	}
}

// Same refusal, same status as GET /shows/calendar: the facet and the list have
// to disagree about nothing, including which windows are addressable.
func TestGetShowCitiesHandler_RefusesAHalfStatedWindow(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetShowCitiesFn: func(
			_ string, _ *contracts.UpcomingShowsFilter, _ contracts.ShowCalendarWindow,
		) ([]contracts.ShowCityResponse, error) {
			t.Fatal("the service must not be reached for a half-stated window")
			return nil, nil
		},
	}
	h := NewShowHandler(mock, nil, nil, nil, nil, nil, nil, testhelpers.AllShowsVisible())

	_, err := h.GetShowCitiesHandler(context.Background(), &GetShowCitiesRequest{Month: 9})
	testhelpers.AssertHumaError(t, err, 422)
}
