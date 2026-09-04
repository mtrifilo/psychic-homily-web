package revisiondiff

import (
	"reflect"
	"testing"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
)

func strPtr(s string) *string   { return &s }
func intPtr(i int) *int         { return &i }
func f64Ptr(f float64) *float64 { return &f }

// TestCompare_ShowAllFields exercises the show field list across the value
// kinds it uses (string, *string, *float64, time.Time) and asserts the exact
// FieldChange shape — value Go types included — so output stays byte-identical
// with the old computeShowChanges.
func TestCompare_ShowAllFields(t *testing.T) {
	oldDate := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)
	newDate := time.Date(2026, 5, 2, 21, 0, 0, 0, time.UTC)

	old := &contracts.ShowResponse{
		Title:          "Old Title",
		EventDate:      oldDate,
		City:           strPtr("Phoenix"),
		State:          strPtr("AZ"),
		Price:          f64Ptr(10),
		DoorPrice:      f64Ptr(12),
		AgeRequirement: strPtr("21+"),
		Description:    strPtr("old desc"),
		TicketURL:      strPtr("https://old.example/tix"),
		ImageURL:       strPtr("https://old.example/flyer.png"),
	}
	updated := &contracts.ShowResponse{
		Title:          "New Title",
		EventDate:      newDate,
		City:           strPtr("Mesa"),
		State:          strPtr("CA"),
		Price:          f64Ptr(15),
		DoorPrice:      f64Ptr(20),
		AgeRequirement: strPtr("18+"),
		Description:    strPtr("new desc"),
		TicketURL:      strPtr("https://new.example/tix"),
		ImageURL:       strPtr("https://new.example/flyer.png"),
	}

	got := Compare(old, updated, ShowFields)
	want := []adminm.FieldChange{
		{Field: "title", OldValue: "Old Title", NewValue: "New Title"},
		{Field: "event_date", OldValue: oldDate.Format(time.RFC3339), NewValue: newDate.Format(time.RFC3339)},
		{Field: "city", OldValue: "Phoenix", NewValue: "Mesa"},
		{Field: "state", OldValue: "AZ", NewValue: "CA"},
		{Field: "price", OldValue: float64(10), NewValue: float64(15)},
		{Field: "door_price", OldValue: float64(12), NewValue: float64(20)},
		{Field: "age_requirement", OldValue: "21+", NewValue: "18+"},
		{Field: "description", OldValue: "old desc", NewValue: "new desc"},
		{Field: "ticket_url", OldValue: "https://old.example/tix", NewValue: "https://new.example/tix"},
		{Field: "image_url", OldValue: "https://old.example/flyer.png", NewValue: "https://new.example/flyer.png"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("show diff mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

// TestCompare_ShowOptionalTimes covers the *time.Time field kind: unset reads
// as nil (SQL NULL), never "" and never a zero-date. See optionalTimeValue —
// "" is not a valid TIMESTAMPTZ, and emitting it makes Rollback fail on any
// revision that first populates one of these columns. Each transition a
// nullable timestamp can make is reported exactly once.
func TestCompare_ShowOptionalTimes(t *testing.T) {
	doors := time.Date(2026, 5, 1, 19, 0, 0, 0, time.UTC)
	laterDoors := time.Date(2026, 5, 1, 19, 30, 0, 0, time.UTC)
	music := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)

	timePtr := func(tv time.Time) *time.Time { return &tv }

	tests := []struct {
		name         string
		old, updated *contracts.ShowResponse
		want         []adminm.FieldChange
	}{
		{
			name:    "unset to set",
			old:     &contracts.ShowResponse{},
			updated: &contracts.ShowResponse{DoorsAt: timePtr(doors)},
			want: []adminm.FieldChange{
				{Field: "doors_at", OldValue: nil, NewValue: doors.Format(time.RFC3339)},
			},
		},
		{
			name:    "set to unset",
			old:     &contracts.ShowResponse{MusicAt: timePtr(music)},
			updated: &contracts.ShowResponse{},
			want: []adminm.FieldChange{
				{Field: "music_at", OldValue: music.Format(time.RFC3339), NewValue: nil},
			},
		},
		{
			name:    "moved",
			old:     &contracts.ShowResponse{DoorsAt: timePtr(doors)},
			updated: &contracts.ShowResponse{DoorsAt: timePtr(laterDoors)},
			want: []adminm.FieldChange{
				{Field: "doors_at", OldValue: doors.Format(time.RFC3339), NewValue: laterDoors.Format(time.RFC3339)},
			},
		},
		{
			name:    "unchanged emits nothing",
			old:     &contracts.ShowResponse{DoorsAt: timePtr(doors), MusicAt: timePtr(music)},
			updated: &contracts.ShowResponse{DoorsAt: timePtr(doors), MusicAt: timePtr(music)},
			want:    nil,
		},
		{
			name:    "both unset emits nothing",
			old:     &contracts.ShowResponse{},
			updated: &contracts.ShowResponse{},
			want:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(tc.old, tc.updated, ShowFields)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("diff mismatch:\n got=%#v\nwant=%#v", got, tc.want)
			}
		})
	}
}

// TestCompare_ArtistNestedSocials covers the nested Social.* path resolution
// and confirms only the changed nested fields are emitted, with flat output
// names matching the old computeArtistChanges.
func TestCompare_ArtistNestedSocials(t *testing.T) {
	old := &contracts.ArtistDetailResponse{
		Name:    "Band",
		Country: strPtr("USA"),
		Social: contracts.SocialResponse{
			Instagram: strPtr("old_ig"),
			Spotify:   strPtr("https://spotify/old"),
		},
	}
	updated := &contracts.ArtistDetailResponse{
		Name:    "Band",              // unchanged
		Country: strPtr("Australia"), // changed
		Social: contracts.SocialResponse{
			Instagram: strPtr("new_ig"),              // changed
			Spotify:   strPtr("https://spotify/old"), // unchanged
		},
	}

	got := Compare(old, updated, ArtistFields)
	want := []adminm.FieldChange{
		{Field: "country", OldValue: "USA", NewValue: "Australia"},
		{Field: "instagram", OldValue: "old_ig", NewValue: "new_ig"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artist diff mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

// TestCompare_ReleaseIntPointers covers *int (release_year) and string fields,
// asserting int emit type and nil-handling.
func TestCompare_ReleaseIntPointers(t *testing.T) {
	old := &contracts.ReleaseDetailResponse{
		Title:       "LP",
		ReleaseType: "album",
		ReleaseYear: intPtr(2020),
		Description: strPtr("desc"),
	}
	updated := &contracts.ReleaseDetailResponse{
		Title:       "LP",           // unchanged
		ReleaseType: "ep",           // changed
		ReleaseYear: intPtr(2021),   // changed
		Description: strPtr("desc"), // unchanged
	}

	got := Compare(old, updated, ReleaseFields)
	want := []adminm.FieldChange{
		{Field: "release_type", OldValue: "album", NewValue: "ep"},
		{Field: "release_year", OldValue: 2021 - 1, NewValue: 2021},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("release diff mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

// TestCompare_ShowOptionalPrices is the *float64 half of the same rule
// TestCompare_ShowOptionalTimes states for *time.Time: unset reads as nil, so
// Rollback restores SQL NULL rather than a number nobody chose (PSY-1960).
//
// Zero is a real price the ticket line renders as "Free", which is what makes
// this a false PUBLIC claim rather than a wrong internal number: flattening an
// unset door_price to 0 and rolling that edit back publishes "DOOR Free" for a
// show whose door price was simply never recorded. Every transition a nullable
// price can make is asserted, including the two that must emit nothing, so a
// future change cannot buy the nil sentinel at the cost of a phantom diff.
func TestCompare_ShowOptionalPrices(t *testing.T) {
	tests := []struct {
		name         string
		old, updated *contracts.ShowResponse
		want         []adminm.FieldChange
	}{
		{
			name:    "unset to set records nil, not zero",
			old:     &contracts.ShowResponse{},
			updated: &contracts.ShowResponse{DoorPrice: f64Ptr(40)},
			want: []adminm.FieldChange{
				{Field: "door_price", OldValue: nil, NewValue: float64(40)},
			},
		},
		{
			name:    "set to unset records nil as the new value",
			old:     &contracts.ShowResponse{Price: f64Ptr(35)},
			updated: &contracts.ShowResponse{},
			want: []adminm.FieldChange{
				{Field: "price", OldValue: float64(35), NewValue: nil},
			},
		},
		{
			name:    "unset to free is a real change to zero",
			old:     &contracts.ShowResponse{},
			updated: &contracts.ShowResponse{Price: f64Ptr(0)},
			want: []adminm.FieldChange{
				{Field: "price", OldValue: nil, NewValue: float64(0)},
			},
		},
		{
			name:    "free to unset is a real change from zero",
			old:     &contracts.ShowResponse{DoorPrice: f64Ptr(0)},
			updated: &contracts.ShowResponse{},
			want: []adminm.FieldChange{
				{Field: "door_price", OldValue: float64(0), NewValue: nil},
			},
		},
		{
			name:    "both unset emits nothing",
			old:     &contracts.ShowResponse{},
			updated: &contracts.ShowResponse{},
			want:    nil,
		},
		{
			name:    "unchanged emits nothing",
			old:     &contracts.ShowResponse{Price: f64Ptr(35), DoorPrice: f64Ptr(40)},
			updated: &contracts.ShowResponse{Price: f64Ptr(35), DoorPrice: f64Ptr(40)},
			want:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(tc.old, tc.updated, ShowFields)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("price diff mismatch:\n got=%#v\nwant=%#v", got, tc.want)
			}
		})
	}
}

// TestCompare_VenueOptionalCapacity is the *int half of PSY-1960, on the field
// that had the defect long before a price did. capacity is registered in
// NumericEditFieldBounds, so a nil recorded here reaches Rollback and is turned
// into a typed (*int)(nil) by NarrowNumericUpdates rather than a 0.
//
// A separate test from the price one because the two pointer kinds are separate
// branches in diffPtr: fixing float64 and leaving int flattened would pass
// TestCompare_ShowOptionalPrices unchanged.
func TestCompare_VenueOptionalCapacity(t *testing.T) {
	tests := []struct {
		name         string
		old, updated *contracts.VenueDetailResponse
		want         []adminm.FieldChange
	}{
		{
			name:    "unset to set records nil, not zero",
			old:     &contracts.VenueDetailResponse{},
			updated: &contracts.VenueDetailResponse{Capacity: intPtr(250)},
			want: []adminm.FieldChange{
				{Field: "capacity", OldValue: nil, NewValue: 250},
			},
		},
		{
			name:    "set to unset records nil as the new value",
			old:     &contracts.VenueDetailResponse{Capacity: intPtr(250)},
			updated: &contracts.VenueDetailResponse{},
			want: []adminm.FieldChange{
				{Field: "capacity", OldValue: 250, NewValue: nil},
			},
		},
		{
			name:    "both unset emits nothing",
			old:     &contracts.VenueDetailResponse{},
			updated: &contracts.VenueDetailResponse{},
			want:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(tc.old, tc.updated, VenueFields)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("capacity diff mismatch:\n got=%#v\nwant=%#v", got, tc.want)
			}
		})
	}
}

// TestCompare_FestivalNonPtrInt covers the non-pointer int field (edition_year)
// and the non-pointer string fields (status, series_slug, start_date).
func TestCompare_FestivalNonPtrInt(t *testing.T) {
	old := &contracts.FestivalDetailResponse{
		Name:        "Fest",
		SeriesSlug:  "fest",
		EditionYear: 2025,
		StartDate:   "2025-06-01",
		EndDate:     "2025-06-03",
		Status:      "draft",
	}
	updated := &contracts.FestivalDetailResponse{
		Name:        "Fest",       // unchanged
		SeriesSlug:  "fest",       // unchanged
		EditionYear: 2026,         // changed (non-ptr int)
		StartDate:   "2026-06-01", // changed
		EndDate:     "2025-06-03", // unchanged
		Status:      "published",  // changed
	}

	got := Compare(old, updated, FestivalFields)
	want := []adminm.FieldChange{
		{Field: "edition_year", OldValue: 2025, NewValue: 2026},
		{Field: "start_date", OldValue: "2025-06-01", NewValue: "2026-06-01"},
		{Field: "status", OldValue: "draft", NewValue: "published"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("festival diff mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

// TestCompare_NoChanges confirms an identical before/after yields a nil/empty
// slice (so the caller skips RecordRevision).
func TestCompare_NoChanges(t *testing.T) {
	v := &contracts.ShowResponse{Title: "T", City: strPtr("Phoenix")}
	if got := Compare(v, v, ShowFields); len(got) != 0 {
		t.Fatalf("expected no changes, got %#v", got)
	}
}

// TestCompare_NilPtrToValue confirms a nil → value transition on a *string is a
// change emitting "" → value, matching the old ptrToStr semantics.
func TestCompare_NilPtrToValue(t *testing.T) {
	old := &contracts.ShowResponse{Title: "T"} // Description nil
	updated := &contracts.ShowResponse{Title: "T", Description: strPtr("now set")}

	got := Compare(old, updated, ShowFields)
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %#v", got)
	}
	want := adminm.FieldChange{Field: "description", OldValue: "", NewValue: "now set"}
	if got[0] != want {
		t.Fatalf("got %#v, want %#v", got[0], want)
	}
}

// TestValidateAll confirms the production field lists all resolve against their
// structs — this is the guard that a renamed struct field fails loudly.
func TestValidateAll(t *testing.T) {
	if err := ValidateAll(); err != nil {
		t.Fatalf("production field lists failed validation: %v", err)
	}
}

// TestValidate_RejectsUnknownField proves the init/test validation catches a
// path that does not exist on the struct (the failure mode the validation
// exists to prevent — a renamed field silently dropping from every revision).
func TestValidate_RejectsUnknownField(t *testing.T) {
	showT := reflect.TypeOf(contracts.ShowResponse{})
	if _, err := resolveFieldType(showT, "DoesNotExist"); err == nil {
		t.Fatal("expected resolveFieldType to reject a non-existent field path")
	}
	// A nested path through a real struct but a bad leaf must also fail.
	artistT := reflect.TypeOf(contracts.ArtistDetailResponse{})
	if _, err := resolveFieldType(artistT, "Social.NotAReal"); err == nil {
		t.Fatal("expected resolveFieldType to reject a bad nested leaf")
	}
}

// TestValidate_RejectsUnsupportedType proves the validation rejects a field
// whose type Compare cannot diff (e.g. a slice), so an ill-typed list fails at
// init rather than panicking mid-request.
func TestValidate_RejectsUnsupportedType(t *testing.T) {
	showT := reflect.TypeOf(contracts.ShowResponse{})
	ft, err := resolveFieldType(showT, "Venues") // []VenueResponse — unsupported
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if SupportedType(ft) {
		t.Fatal("expected []VenueResponse to be an unsupported diff type")
	}
}

// EmitValue is the one spelling of "the previous value" that both a diff and
// the pending-edit pipeline record through, so its pointer cases are the ones
// that matter: a nil *string emits "" while every other nullable kind emits
// nil, because Rollback writes the emitted value straight back into the column.
func TestEmitValue(t *testing.T) {
	str := "hello"
	num := 42
	f := 1.5
	when := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		in    interface{}
		want  interface{}
		isNil bool
	}{
		{name: "string", in: "plain", want: "plain"},
		{name: "int", in: 7, want: 7},
		{name: "time", in: when, want: when.Format(time.RFC3339)},
		{name: "set string pointer", in: &str, want: "hello"},
		{name: "nil string pointer", in: (*string)(nil), want: ""},
		{name: "set int pointer", in: &num, want: 42},
		{name: "nil int pointer", in: (*int)(nil), isNil: true},
		{name: "set float pointer", in: &f, want: 1.5},
		{name: "nil float pointer", in: (*float64)(nil), isNil: true},
		{name: "set time pointer", in: &when, want: when.Format(time.RFC3339)},
		{name: "nil time pointer", in: (*time.Time)(nil), isNil: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EmitValue(reflect.ValueOf(tc.in))
			if err != nil {
				t.Fatalf("EmitValue: %v", err)
			}
			if tc.isNil {
				if got != nil {
					t.Fatalf("got %#v, want nil", got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// An unsupported type is reported rather than silently emitted as something,
// which is what lets a caller outside a validated field list (the pending-edit
// pipeline reading a GORM model) tell a convertible column from one it must not
// guess at.
func TestEmitValue_RejectsUnsupportedType(t *testing.T) {
	if _, err := EmitValue(reflect.ValueOf([]string{"a"})); err == nil {
		t.Fatal("expected an error for a slice field")
	}
	if _, err := EmitValue(reflect.ValueOf(true)); err == nil {
		t.Fatal("expected an error for a bool field")
	}
}
