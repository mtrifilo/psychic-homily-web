package auth

import (
	"encoding/json"
	"errors"
	"testing"
)

// PSY-386: the home-layout document's rules. These are the whole 422 surface
// of the two endpoints, so each rejection is pinned here rather than only
// through the handler.

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

// An empty section list is "hide nothing, order nothing", which the frontend
// fills from the defaults. It is not an error: rejecting it would make the
// only way to say "back to defaults" a DELETE, and a client that sends both is
// harder to reason about than one that can send either.
func TestHomeLayoutValidate_AcceptsAnEmptySectionList(t *testing.T) {
	layout := &HomeLayout{Version: HomeLayoutVersion, Sections: []HomeLayoutSection{}}
	if err := layout.Validate(); err != nil {
		t.Fatalf("empty section list should validate, got %v", err)
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

// KnownHomeSections hands out a copy, so a caller sorting or truncating the
// result cannot reorder the whitelist every validation reads.
func TestKnownHomeSections_ReturnsACopy(t *testing.T) {
	first := KnownHomeSections()
	if len(first) != len(knownHomeSections) {
		t.Fatalf("expected %d sections, got %d", len(knownHomeSections), len(first))
	}
	first[0] = "tampered"
	if KnownHomeSections()[0] != knownHomeSections[0] {
		t.Errorf("whitelist was mutated through the returned slice")
	}
}
