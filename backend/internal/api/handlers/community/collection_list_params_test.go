package community

import (
	"reflect"
	"slices"
	"testing"
)

// The query parameters GET /collections and GET /crates publish, pinned so a
// parameter added to or removed from the request struct is a reviewed change
// to this list. Each listed parameter is one ListCollectionsHandler reads.
func TestListCollectionsHandlerRequest_QueryParameterSnapshot(t *testing.T) {
	want := []string{"entity_type", "featured", "limit", "offset", "search", "sort", "tag"}

	var got []string
	typ := reflect.TypeOf(ListCollectionsHandlerRequest{})
	for i := 0; i < typ.NumField(); i++ {
		if name := typ.Field(i).Tag.Get("query"); name != "" {
			got = append(got, name)
		}
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("GET /collections declares %v, want %v", got, want)
	}
}
