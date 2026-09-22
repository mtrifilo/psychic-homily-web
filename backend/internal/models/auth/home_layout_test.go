package auth

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

// The home-layout document's rules. These are the whole 422 surface of the two
// endpoints, so each rejection is pinned here rather than only through the
// handler.

func defaultHomeLayout() *HomeLayout {
	return &HomeLayout{
		Version: HomeLayoutVersion,
		Sections: []HomeLayoutSection{
			{ID: HomeSectionSavedShows, Visible: true},
			{ID: HomeSectionNearbyShows, Visible: true},
			{ID: HomeSectionCommunityStats, Visible: true},
			{ID: HomeSectionCityGraph, Visible: false},
			{ID: HomeSectionRadioShows, Visible: true},
		},
	}
}

func TestHomeLayoutValidate_AcceptsTheShippedDocument(t *testing.T) {
	if err := defaultHomeLayout().Validate(); err != nil {
		t.Fatalf("shipped layout should validate, got %v", err)
	}
}

// A short document is valid: the frontend appends any known section the
// document omits, which is what lets a new section ship without rewriting
// stored rows.
func TestHomeLayoutValidate_AcceptsAPartialDocument(t *testing.T) {
	layout := &HomeLayout{
		Version:  HomeLayoutVersion,
		Sections: []HomeLayoutSection{{ID: HomeSectionRadioShows, Visible: false}},
	}
	if err := layout.Validate(); err != nil {
		t.Fatalf("partial layout should validate, got %v", err)
	}
}

// An empty section list is a well-formed document that happens to place
// nothing, so it validates. It is NOT a reset: storing it leaves home_layout
// non-NULL, so the user still reads as customised. DELETE is the only reset.
func TestHomeLayoutValidate_AcceptsAnEmptySectionList(t *testing.T) {
	layout := &HomeLayout{Version: HomeLayoutVersion, Sections: []HomeLayoutSection{}}
	if err := layout.Validate(); err != nil {
		t.Fatalf("empty section list should validate, got %v", err)
	}
}

// A nil list is accepted on the same terms as an empty one, and marshals to
// `"sections":null`. Pinned because the generated client types the field as
// nullable, so a client can send it and the stored document must stay readable.
func TestHomeLayoutValidate_AcceptsANilSectionList(t *testing.T) {
	layout := &HomeLayout{Version: HomeLayoutVersion}
	if err := layout.Validate(); err != nil {
		t.Fatalf("nil section list should validate, got %v", err)
	}
	encoded, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if string(encoded) != `{"version":1,"sections":null}` {
		t.Errorf("nil sections round trip drifted: %s", encoded)
	}
}

func TestHomeLayoutValidate_Rejections(t *testing.T) {
	tooMany := make([]HomeLayoutSection, 0, len(knownHomeSections)+1)
	for _, id := range knownHomeSections {
		tooMany = append(tooMany, HomeLayoutSection{ID: id, Visible: true})
	}
	tooMany = append(tooMany, HomeLayoutSection{ID: HomeSectionSavedShows, Visible: true})

	cases := []struct {
		name   string
		layout *HomeLayout
	}{
		{"nil document", nil},
		{"missing version", &HomeLayout{Sections: []HomeLayoutSection{}}},
		{"unsupported version", &HomeLayout{Version: 2, Sections: []HomeLayoutSection{}}},
		{"unknown id", &HomeLayout{
			Version:  HomeLayoutVersion,
			Sections: []HomeLayoutSection{{ID: "mixtapes", Visible: true}},
		}},
		{"whitespace around a known id", &HomeLayout{
			Version:  HomeLayoutVersion,
			Sections: []HomeLayoutSection{{ID: " saved_shows ", Visible: true}},
		}},
		{"duplicate id", &HomeLayout{
			Version: HomeLayoutVersion,
			Sections: []HomeLayoutSection{
				{ID: HomeSectionSavedShows, Visible: true},
				{ID: HomeSectionSavedShows, Visible: false},
			},
		}},
		{"more entries than known sections", &HomeLayout{Version: HomeLayoutVersion, Sections: tooMany}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.layout.Validate()
			if err == nil {
				t.Fatalf("expected a rejection")
			}
			// The wrap is what the handler keys its 422 off, so it is part of
			// the contract, not an implementation detail.
			if !errors.Is(err, ErrInvalidHomeLayout) {
				t.Errorf("rejection must wrap ErrInvalidHomeLayout, got %v", err)
			}
		})
	}
}

// The JSON keys are a storage contract shared with the frontend and with every
// document already written, so they are pinned against an accidental rename.
func TestHomeLayoutJSONKeys(t *testing.T) {
	encoded, err := json.Marshal(&HomeLayout{
		Version:  HomeLayoutVersion,
		Sections: []HomeLayoutSection{{ID: HomeSectionCityGraph, Visible: false}},
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	const want = `{"version":1,"sections":[{"id":"city_graph","visible":false}]}`
	if string(encoded) != want {
		t.Errorf("document shape drifted\n got: %s\nwant: %s", encoded, want)
	}
}

// Struct tags must be constant literals, so the OpenAPI enum on
// HomeLayoutSection.id cannot be built from knownHomeSections. This test is
// the join: widening the whitelist without widening the tag would leave the
// API rejecting a section the service accepts, and the regenerated frontend
// types would silently disagree with the server.
func TestHomeSectionEnumTagMatchesVocabulary(t *testing.T) {
	field, ok := reflect.TypeOf(HomeLayoutSection{}).FieldByName("ID")
	if !ok {
		t.Fatalf("HomeLayoutSection.ID must exist")
	}
	if got := field.Tag.Get("enum"); got != homeSectionVocabularyCSV() {
		t.Errorf("enum tag must list exactly the whitelist, in order\n got: %s\nwant: %s",
			got, homeSectionVocabularyCSV())
	}
}

// The same join for the length cap. Without it the schema would keep
// advertising a stale bound after the whitelist grows, so a client would be
// refused at the transport layer for a document the service accepts.
func TestHomeSectionMaxItemsMatchesVocabulary(t *testing.T) {
	field, ok := reflect.TypeOf(HomeLayout{}).FieldByName("Sections")
	if !ok {
		t.Fatalf("HomeLayout.Sections must exist")
	}
	if got := field.Tag.Get("maxItems"); got != strconv.Itoa(len(knownHomeSections)) {
		t.Errorf("schema maxItems is %q but the whitelist holds %d sections",
			got, len(knownHomeSections))
	}
}
