package community

import (
	"reflect"
	"slices"
	"testing"
)

// GET /collections declares exactly the query parameters ListCollectionsHandler
// reads, so the published API offers no filter that does nothing. A new
// parameter is added here once the handler reads it.
func TestListCollectionsHandlerRequest_DeclaresOnlyParametersTheHandlerReads(t *testing.T) {
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
