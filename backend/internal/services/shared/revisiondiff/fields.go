package revisiondiff

import (
	"fmt"
	"reflect"
	"strings"

	"psychic-homily-backend/internal/services/contracts"
)

// Per-entity revision field lists. Order and names are byte-for-byte the same
// as the compute*Changes helpers they replaced (PSY-563), so existing
// revisions.field_changes rows and the History UI render unchanged.
//
// These lists — not the models/admin contributor allowlists — define what a
// revision records. They deliberately differ from the allowlists: festival and
// label track admin-only fields (status, start_date, edition_year, …) that
// contributors cannot edit, and show has no allowlist entry at all. See the
// package doc for why Compare is not driven off the allowlist.
var (
	// ShowFields tracks the scalar show fields the EntityEditDrawer surfaces.
	// Venue and artist association changes are intentionally excluded — relation
	// diffs would need their own schema.
	ShowFields = []Field{
		{Name: "title", Path: "Title"},
		{Name: "event_date", Path: "EventDate"},
		{Name: "city", Path: "City"},
		{Name: "state", Path: "State"},
		{Name: "price", Path: "Price"},
		{Name: "age_requirement", Path: "AgeRequirement"},
		{Name: "description", Path: "Description"},
		{Name: "ticket_url", Path: "TicketURL"},
		{Name: "image_url", Path: "ImageURL"},
	}

	ArtistFields = []Field{
		{Name: "name", Path: "Name"},
		{Name: "city", Path: "City"},
		{Name: "state", Path: "State"},
		{Name: "country", Path: "Country"},
		{Name: "instagram", Path: "Social.Instagram"},
		{Name: "facebook", Path: "Social.Facebook"},
		{Name: "twitter", Path: "Social.Twitter"},
		{Name: "youtube", Path: "Social.YouTube"},
		{Name: "spotify", Path: "Social.Spotify"},
		{Name: "soundcloud", Path: "Social.SoundCloud"},
		{Name: "bandcamp", Path: "Social.Bandcamp"},
		{Name: "website", Path: "Social.Website"},
		{Name: "description", Path: "Description"},
	}

	VenueFields = []Field{
		{Name: "name", Path: "Name"},
		{Name: "address", Path: "Address"},
		{Name: "city", Path: "City"},
		{Name: "state", Path: "State"},
		{Name: "zipcode", Path: "Zipcode"},
		{Name: "capacity", Path: "Capacity"},
		{Name: "instagram", Path: "Social.Instagram"},
		{Name: "facebook", Path: "Social.Facebook"},
		{Name: "twitter", Path: "Social.Twitter"},
		{Name: "youtube", Path: "Social.YouTube"},
		{Name: "spotify", Path: "Social.Spotify"},
		{Name: "soundcloud", Path: "Social.SoundCloud"},
		{Name: "bandcamp", Path: "Social.Bandcamp"},
		{Name: "website", Path: "Social.Website"},
		{Name: "description", Path: "Description"},
		{Name: "image_url", Path: "ImageURL"},
	}

	ReleaseFields = []Field{
		{Name: "title", Path: "Title"},
		{Name: "release_type", Path: "ReleaseType"},
		{Name: "release_year", Path: "ReleaseYear"},
		{Name: "release_date", Path: "ReleaseDate"},
		{Name: "cover_art_url", Path: "CoverArtURL"},
		{Name: "description", Path: "Description"},
	}

	LabelFields = []Field{
		{Name: "name", Path: "Name"},
		{Name: "city", Path: "City"},
		{Name: "state", Path: "State"},
		{Name: "country", Path: "Country"},
		{Name: "founded_year", Path: "FoundedYear"},
		{Name: "status", Path: "Status"},
		{Name: "description", Path: "Description"},
		{Name: "instagram", Path: "Social.Instagram"},
		{Name: "facebook", Path: "Social.Facebook"},
		{Name: "twitter", Path: "Social.Twitter"},
		{Name: "youtube", Path: "Social.YouTube"},
		{Name: "spotify", Path: "Social.Spotify"},
		{Name: "soundcloud", Path: "Social.SoundCloud"},
		{Name: "bandcamp", Path: "Social.Bandcamp"},
		{Name: "website", Path: "Social.Website"},
		{Name: "image_url", Path: "ImageURL"},
	}

	FestivalFields = []Field{
		{Name: "name", Path: "Name"},
		{Name: "series_slug", Path: "SeriesSlug"},
		{Name: "edition_year", Path: "EditionYear"},
		{Name: "description", Path: "Description"},
		{Name: "location_name", Path: "LocationName"},
		{Name: "city", Path: "City"},
		{Name: "state", Path: "State"},
		{Name: "country", Path: "Country"},
		{Name: "start_date", Path: "StartDate"},
		{Name: "end_date", Path: "EndDate"},
		{Name: "website", Path: "Website"},
		{Name: "ticket_url", Path: "TicketURL"},
		{Name: "flyer_url", Path: "FlyerURL"},
		{Name: "status", Path: "Status"},
	}
)

// registry pairs each field list with the struct type it is diffed against, so
// ValidateAll can confirm every Path resolves to a real, supported field.
var registry = []struct {
	name    string
	fields  []Field
	example interface{}
}{
	{"show", ShowFields, contracts.ShowResponse{}},
	{"artist", ArtistFields, contracts.ArtistDetailResponse{}},
	{"venue", VenueFields, contracts.VenueDetailResponse{}},
	{"release", ReleaseFields, contracts.ReleaseDetailResponse{}},
	{"label", LabelFields, contracts.LabelDetailResponse{}},
	{"festival", FestivalFields, contracts.FestivalDetailResponse{}},
}

func init() {
	if err := ValidateAll(); err != nil {
		// A bad field list is a programming error — fail at startup (and in
		// tests) instead of silently dropping the field from every revision.
		panic(err)
	}
}

// ValidateAll checks every registered field list against its struct: each Path
// must resolve to an existing field whose type is one Compare can diff. Returns
// the first problem found, or nil when all lists are well-formed.
func ValidateAll() error {
	for _, e := range registry {
		t := reflect.TypeOf(e.example)
		for _, f := range e.fields {
			ft, err := resolveFieldType(t, f.Path)
			if err != nil {
				return fmt.Errorf("revisiondiff: %s field %q (%s): %w", e.name, f.Name, f.Path, err)
			}
			if !supportedType(ft) {
				return fmt.Errorf("revisiondiff: %s field %q (%s) has unsupported type %s", e.name, f.Name, f.Path, ft)
			}
		}
	}
	return nil
}

// resolveFieldType walks a dot-separated path through nested struct types and
// returns the leaf field's type. Errors when a path segment names a field that
// does not exist or descends through a non-struct.
func resolveFieldType(t reflect.Type, path string) (reflect.Type, error) {
	cur := t
	for _, part := range strings.Split(path, ".") {
		if cur.Kind() != reflect.Struct {
			return nil, fmt.Errorf("cannot descend into non-struct %s at %q", cur.Kind(), part)
		}
		sf, ok := cur.FieldByName(part)
		if !ok {
			return nil, fmt.Errorf("no such field %q", part)
		}
		cur = sf.Type
	}
	return cur, nil
}

// supportedType reports whether diffValue/diffPtr can handle ft.
func supportedType(ft reflect.Type) bool {
	if ft == timeType {
		return true
	}
	switch ft.Kind() {
	case reflect.String, reflect.Int:
		return true
	case reflect.Ptr:
		switch ft.Elem().Kind() {
		case reflect.String, reflect.Float64, reflect.Int:
			return true
		}
	}
	return false
}
