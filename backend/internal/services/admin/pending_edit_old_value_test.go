package admin

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"gorm.io/gorm/schema"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

// entityModels must answer for exactly the entity types that can carry a
// pending edit. A type with no model reaches deriveOldValues and fails every
// submission for that entity; a model for a type that cannot be edited is dead
// weight that reads as coverage.
func TestEntityModelsCoverPendingEditTypes(t *testing.T) {
	for _, entityType := range adminm.ValidPendingEditEntityTypes() {
		if _, ok := entityModels[entityType]; !ok {
			t.Errorf("entity type %q accepts pending edits but has no model in entityModels", entityType)
		}
	}
	for entityType := range entityModels {
		if !adminm.IsValidPendingEditEntityType(entityType) {
			t.Errorf("entityModels has %q, which does not accept pending edits", entityType)
		}
	}
}

// Every allowlisted field must resolve to a column on its entity's model and
// hold a type emitValue can convert. This is the drift guard for the two lists:
// a field added to an allowlist whose column lives somewhere this file cannot
// see would otherwise fail at runtime, on every submission naming it, with the
// fail-closed refusal deriveOldValues raises for an unknown column.
func TestAllowedEditFieldsAreDerivable(t *testing.T) {
	namer := schema.NamingStrategy{}
	for _, entityType := range adminm.ValidPendingEditEntityTypes() {
		allowed, ok := adminm.AllowedEditFields(entityType)
		if !ok {
			t.Fatalf("no allowlist for %s", entityType)
		}
		newModel, ok := entityModels[entityType]
		if !ok {
			t.Fatalf("no model for %s", entityType)
		}
		columns := map[string]reflect.Value{}
		collectColumns(reflect.ValueOf(newModel()).Elem(), namer, columns)

		for field := range allowed {
			value, present := columns[field]
			if !present {
				t.Errorf("%s: allowlisted field %q is not a column on the model", entityType, field)
				continue
			}
			if _, err := emitValue(value); err != nil {
				t.Errorf("%s.%s: %v", entityType, field, err)
			}
		}
	}
}

// The emit rules are revisiondiff's, and the pointer cases are the ones that
// matter: a nil *string emits "" while every other nullable kind emits nil,
// because Rollback writes the emitted value straight back into the column.
func TestEmitValue(t *testing.T) {
	str := "hello"
	num := 42
	f := 1.5

	cases := []struct {
		name  string
		in    interface{}
		want  interface{}
		isNil bool
	}{
		{name: "string", in: "plain", want: "plain"},
		{name: "int", in: 7, want: 7},
		{name: "set string pointer", in: &str, want: "hello"},
		{name: "nil string pointer", in: (*string)(nil), want: ""},
		{name: "set int pointer", in: &num, want: 42},
		{name: "nil int pointer", in: (*int)(nil), isNil: true},
		{name: "set float pointer", in: &f, want: 1.5},
		{name: "nil float pointer", in: (*float64)(nil), isNil: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := emitValue(reflect.ValueOf(tc.in))
			if err != nil {
				t.Fatalf("emitValue: %v", err)
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

// A claim is compared against the derived value across the encodings the two
// sides really use: JSON hands a number back as float64 while the entity yields
// an int, and an empty field is spelled null by the edit drawer and "" by the
// inline editors.
func TestSameFieldValue(t *testing.T) {
	cases := []struct {
		name    string
		claim   interface{}
		current interface{}
		want    bool
	}{
		{"identical strings", "a", "a", true},
		{"different strings", "a", "b", false},
		{"null claim on empty column", nil, "", true},
		{"blank claim on empty column", "", "", true},
		{"null claim on set column", nil, "a", false},
		{"blank claim on set column", "", "a", false},
		{"claim on cleared column", "a", "", false},
		{"json number against int", float64(550), 550, true},
		{"different numbers", float64(551), 550, false},
		{"null claim on unset number", nil, nil, true},
		{"number claim on unset number", float64(550), nil, false},
		{"null claim on set number", nil, 550, false},
		{"string claim on number column", "550", 550, false},
		{"bool", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameFieldValue(tc.claim, tc.current); got != tc.want {
				t.Fatalf("sameFieldValue(%#v, %#v) = %v, want %v", tc.claim, tc.current, got, tc.want)
			}
		})
	}
}

// An unverified venue does not publish its address, so a submitter could not
// have observed it and a claim about it is not evidence of anything. Verifying
// the venue publishes it again and the claim becomes meaningful.
func TestVenueWithheldEditFields(t *testing.T) {
	addr := "123 Real St"
	zip := "85004"

	unverified := &catalogm.Venue{Address: &addr, Zipcode: &zip}
	if got := unverified.WithheldEditFields(); len(got) != 2 {
		t.Fatalf("unverified venue: got %v, want address and zipcode", got)
	}

	verified := &catalogm.Venue{Address: &addr, Zipcode: &zip, Verified: true}
	if got := verified.WithheldEditFields(); len(got) != 0 {
		t.Fatalf("verified venue: got %v, want none", got)
	}

	empty := &catalogm.Venue{}
	if got := empty.WithheldEditFields(); len(got) != 0 {
		t.Fatalf("venue with no address: got %v, want none", got)
	}
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

// The stored old_value is the entity's, not the submitter's. A NULL text column
// is stored as "" rather than the null the drawer sent, which is the shape
// revisiondiff records for the same column and therefore the shape Rollback
// writes back.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_DerivesOldValueFromEntity() {
	user := s.createTestUser()
	artist := s.createTestArtist("Derive Me")

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "name", OldValue: "Derive Me", NewValue: "Derived"},
			{Field: "description", OldValue: nil, NewValue: "a blurb"},
		},
		Summary: "fill it in",
	})
	s.Require().NoError(err)

	stored := s.storedChanges(resp.ID)
	s.Equal("Derive Me", stored["name"].OldValue)
	s.Equal("", stored["description"].OldValue, "a NULL text column is stored as the empty string, not null")
}

// A number claimed as JSON and derived as an int is the same value, so an
// ordinary numeric edit is not a conflict, and the stored old_value is the
// server's reading of the column.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_DerivesNumericOldValue() {
	user := s.createTestUser()
	venue := s.createTestVenue("Numeric Room")
	capacity := 550
	s.Require().NoError(s.db.Model(venue).Update("capacity", capacity).Error)

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes:    []adminm.FieldChange{{Field: "capacity", OldValue: float64(550), NewValue: float64(600)}},
		Summary:    "bigger room",
	})
	s.Require().NoError(err)

	stored := s.storedChanges(resp.ID)
	s.EqualValues(550, toFloat(s.T(), stored["capacity"].OldValue))
}

// The conflict signal: the submitter composed the edit against a value the
// entity no longer holds.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_RefusesStaleClaim() {
	user := s.createTestUser()
	artist := s.createTestArtist("Current Name")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Name From An Older Page Load", "New Name"),
		Summary:    "rename",
	})
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
	s.Contains(editErr.Message, "name")

	var count int64
	s.Require().NoError(s.db.Model(&adminm.PendingEntityEdit{}).Count(&count).Error)
	s.Zero(count, "a refused submission stores nothing")
}

// A planted previous value is a stale claim like any other: the submitter says
// the column held a hostile URL and it did not.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_RefusesPlantedOldValue() {
	user := s.createTestUser()
	artist := s.createTestArtist("Planted Value")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: makeChanges("spotify",
			"https://spotify-account-verify.evil.test/",
			"https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"),
		Summary: "add their spotify",
	})
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
}

// Filling an empty field is the ordinary case and must not read as a conflict,
// whichever way the client spells "it was empty".
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_AcceptsEitherSpellingOfEmpty() {
	user := s.createTestUser()

	for _, claim := range []interface{}{nil, ""} {
		artist := s.createTestArtist(fmt.Sprintf("Empty Claim %v", claim))
		resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
			EntityType: "artist",
			EntityID:   artist.ID,
			UserID:     user.ID,
			Changes:    []adminm.FieldChange{{Field: "website", OldValue: claim, NewValue: "https://band.example.org"}},
			Summary:    "add their site",
		})
		s.Require().NoError(err, "claim %#v", claim)
		s.Equal("", s.storedChanges(resp.ID)["website"].OldValue)
		s.Require().NoError(s.db.Where("id = ?", resp.ID).Delete(&adminm.PendingEntityEdit{}).Error)
	}
}

// An unverified venue's address is served to nobody, so a submitter's claim
// about it carries no information and must not block the edit. The value stored
// is still the server's: the column, not the claim.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_SkipsConflictOnWithheldAddress() {
	user := s.createTestUser()
	venue := s.createTestVenue("House Show")
	s.Require().NoError(s.db.Model(venue).Update("address", "123 Real St").Error)

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes:    []adminm.FieldChange{{Field: "address", OldValue: nil, NewValue: "456 New St"}},
		Summary:    "they moved",
	})
	s.Require().NoError(err, "an unreadable field cannot produce a stale-value conflict")
	s.Equal("123 Real St", s.storedChanges(resp.ID)["address"].OldValue)
}

// Verifying the venue publishes the address again, so a claim about it becomes
// evidence and a wrong one is a conflict.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_RefusesStaleAddressOnVerifiedVenue() {
	user := s.createTestUser()
	venue := s.createTestVenue("Verified Room")
	s.Require().NoError(s.db.Model(venue).Updates(map[string]interface{}{
		"address":  "123 Real St",
		"verified": true,
	}).Error)

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes:    makeChanges("address", "999 Stale Ave", "456 New St"),
		Summary:    "they moved",
	})
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditStaleValue, editErr.Code)
}

// A field that is not a column on the entity is refused rather than stored with
// the submitter's unverified claim beside it.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_RefusesUnknownColumn() {
	user := s.createTestUser()
	artist := s.createTestArtist("Unknown Column")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("is_admin", "false", "true"),
		Summary:    "nice try",
	})
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditInvalidRequest, editErr.Code)
}

// The entity-existence answer still comes from the create path, now as the
// result of the read the derivation needs anyway.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_MissingEntityStillNotFound() {
	user := s.createTestUser()

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   987654321,
		UserID:     user.ID,
		Changes:    makeChanges("name", "", "New Name"),
		Summary:    "rename a ghost",
	})
	s.Require().Error(err)

	var editErr *apperrors.PendingEditError
	s.Require().ErrorAs(err, &editErr)
	s.Equal(apperrors.CodePendingEditEntityNotFound, editErr.Code)
}

// storedChanges reads back the field changes a pending edit actually stored,
// keyed by field, so a test asserts what is in the row rather than what it
// submitted.
func (s *PendingEditServiceIntegrationTestSuite) storedChanges(editID uint) map[string]adminm.FieldChange {
	var edit adminm.PendingEntityEdit
	s.Require().NoError(s.db.First(&edit, editID).Error)
	var changes []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*edit.FieldChanges, &changes))
	out := make(map[string]adminm.FieldChange, len(changes))
	for _, c := range changes {
		out[c.Field] = c
	}
	return out
}

func toFloat(t *testing.T, v interface{}) float64 {
	t.Helper()
	f, ok := numericValue(v)
	if !ok {
		t.Fatalf("value %#v is not numeric", v)
	}
	return f
}
