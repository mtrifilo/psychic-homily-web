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
		{Field: "age_requirement", OldValue: "21+", NewValue: "18+"},
		{Field: "description", OldValue: "old desc", NewValue: "new desc"},
		{Field: "ticket_url", OldValue: "https://old.example/tix", NewValue: "https://new.example/tix"},
		{Field: "image_url", OldValue: "https://old.example/flyer.png", NewValue: "https://new.example/flyer.png"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("show diff mismatch:\n got=%#v\nwant=%#v", got, want)
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
	if supportedType(ft) {
		t.Fatal("expected []VenueResponse to be an unsupported diff type")
	}
}
