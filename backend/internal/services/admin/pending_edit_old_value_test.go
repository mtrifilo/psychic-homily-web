package admin

import (
	"encoding/json"
	"fmt"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
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
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open dummy connection: %v", err)
	}
	for _, entityType := range adminm.ValidPendingEditEntityTypes() {
		allowed, ok := adminm.AllowedEditFields(entityType)
		if !ok {
			t.Fatalf("no allowlist for %s", entityType)
		}
		newModel, ok := entityModels[entityType]
		if !ok {
			t.Fatalf("no model for %s", entityType)
		}
		columns, err := modelColumns(db, newModel())
		if err != nil {
			t.Fatalf("%s: %v", entityType, err)
		}

		for field := range allowed {
			value, present := columns[field]
			if !present {
				t.Errorf("%s: allowlisted field %q is not a column on the model", entityType, field)
				continue
			}
			if _, err := revisiondiff.EmitValue(value); err != nil {
				t.Errorf("%s.%s: %v", entityType, field, err)
			}
		}
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
// about it carries no information and must not block the edit.
//
// The derived value is the WITHHELD view, not the column. A pending edit is read
// back by its submitter, so deriving from the column would hand any authenticated
// user the street address of a house show by asking to edit it, defeating the
// gate on the live payload rather than mirroring it.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_WithheldAddressNeitherConflictsNorLeaks() {
	user := s.createTestUser()
	venue := s.createTestVenue("House Show")
	s.Require().NoError(s.db.Model(venue).Updates(map[string]interface{}{
		"address": "123 Real St",
		"zipcode": "85031",
	}).Error)

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "address", OldValue: nil, NewValue: "456 New St"},
			{Field: "zipcode", OldValue: nil, NewValue: "85004"},
		},
		Summary: "they moved",
	})
	s.Require().NoError(err, "an unreadable field cannot produce a stale-value conflict")

	stored := s.storedChanges(resp.ID)
	s.Equal("", stored["address"].OldValue, "the withheld address must not be stored or served back")
	s.Equal("", stored["zipcode"].OldValue, "the withheld zipcode must not be stored or served back")

	served := resp.FieldChanges
	for _, c := range served {
		s.NotEqual("123 Real St", c.OldValue)
		s.NotEqual("85031", c.OldValue)
	}
}

// The withheld view is the SAME whatever the column holds, so the conflict
// answer carries no bit about it. A venue with no address on file behaves
// identically to one whose address is withheld.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_WithheldAddressIsNotAnOracle() {
	user := s.createTestUser()

	withAddress := s.createTestVenue("Has An Address")
	s.Require().NoError(s.db.Model(withAddress).Update("address", "123 Real St").Error)
	withoutAddress := s.createTestVenue("Has No Address")

	probe := func(venueID uint) error {
		_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
			EntityType: "venue",
			EntityID:   venueID,
			UserID:     user.ID,
			Changes:    makeChanges("address", "999 Guess Ave", "456 New St"),
			Summary:    "probing",
		})
		return err
	}

	withErr := probe(withAddress.ID)
	withoutErr := probe(withoutAddress.ID)
	s.Require().Error(withErr)
	s.Require().Error(withoutErr)
	s.Equal(withErr.Error(), withoutErr.Error(),
		"the answer must not differ on whether the withheld column is set")
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

// A field the entity's allowlist does not expose is refused rather than stored
// with the submitter's unverified claim beside it.
//
// The unknown-COLUMN branch behind this one cannot be reached from any allowlist
// today, which is what TestAllowedEditFieldsAreDerivable pins; it is the guard
// for an allowlist that gains a name no column answers to.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_RefusesFieldOutsideTheAllowlist() {
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
	s.Contains(editErr.Message, "is not editable", "the allowlist branch is the one that fired")
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

// A name a reporter withholds has to be a field a submission can actually carry,
// or the withholding matches nothing and reads exactly like a gate that works.
func TestWithheldFieldsAreEditable(t *testing.T) {
	for entityType, newModel := range entityModels {
		reporter, ok := newModel().(withheldEditFieldsReporter)
		if !ok {
			continue
		}
		allowed, ok := adminm.AllowedEditFields(entityType)
		if !ok {
			t.Fatalf("no allowlist for %s", entityType)
		}
		// A zero-value model withholds nothing, so ask the model's own list of
		// gated names rather than a verdict about one instance.
		for _, name := range namesWithheldBy(t, entityType) {
			if !allowed[name] {
				t.Errorf("%s: withheld field %q is not editable, so nothing is withheld by naming it",
					entityType, name)
			}
		}
		_ = reporter
	}
}

// namesWithheldBy returns every field name the entity type's gate can withhold.
// Venue is the only entity with a gate; a new one belongs here beside it.
func namesWithheldBy(t *testing.T, entityType string) []string {
	t.Helper()
	if entityType == adminm.PendingEditEntityVenue {
		return catalogm.VenuePrivateFields()
	}
	return nil
}

// release_date is the one allowlisted field whose stored type and its wire form
// could disagree: a DATE column typed as *string, which gorm reads back as an
// RFC3339 timestamp rather than the YYYY-MM-DD the drawer's placeholder shows.
//
// The claim is taken from the model gorm just read, because that IS the value
// the release response passes through, and the property under test is that the
// derivation and the response agree. A derivation that spelled the date any
// other way would 409 every release-date edit forever and no other test would
// notice, so the assertion is the identity rather than a hardcoded format.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_DerivesDateAndYearOnRelease() {
	user := s.createTestUser()
	release := s.createTestRelease("Knife Man")
	s.Require().NoError(s.db.Model(release).Updates(map[string]interface{}{
		"release_date": "2011-10-04",
		"release_year": 2011,
	}).Error)

	var served catalogm.Release
	s.Require().NoError(s.db.First(&served, release.ID).Error)
	s.Require().NotNil(served.ReleaseDate)

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "release",
		EntityID:   release.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "release_date", OldValue: *served.ReleaseDate, NewValue: "2011-10-05"},
			{Field: "release_year", OldValue: float64(2011), NewValue: float64(2012)},
		},
		Summary: "correct the date",
	})
	s.Require().NoError(err, "a claim spelled the way the response serves the date must not conflict")

	stored := s.storedChanges(resp.ID)
	s.Equal(*served.ReleaseDate, stored["release_date"].OldValue)
	s.EqualValues(2011, toFloat(s.T(), stored["release_year"].OldValue))
}
