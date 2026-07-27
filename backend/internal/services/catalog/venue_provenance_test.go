package catalog

import (
	"encoding/json"
	"testing"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// Unit: source derivation
// =============================================================================

// TestBuildVenueProvenance_Sources pins the rule that the source list is
// DERIVED FROM STORED FACTS ONLY. The tempting inference — "no data_source and
// no submitter means it came from ingest" — is exactly the kind of plausible
// guess that makes a provenance stamp untrustworthy, so an unpopulated
// data_source must yield no ingest source at all.
func TestBuildVenueProvenance_Sources(t *testing.T) {
	updated := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	ingest := "venue_ingest"
	empty := ""

	cases := []struct {
		name       string
		dataSource *string
		edits      venueEditAggregate
		confirms   venueConfirmationAggregate
		want       []string
	}{
		{"untouched venue has no sources", nil, venueEditAggregate{}, venueConfirmationAggregate{}, []string{}},
		{"empty data_source is not a source", &empty, venueEditAggregate{}, venueConfirmationAggregate{}, []string{}},
		{"data_source alone is ingest", &ingest, venueEditAggregate{}, venueConfirmationAggregate{}, []string{"ingest"}},
		{"an approved edit is community", nil, venueEditAggregate{EditCount: 1, ContributorCount: 1}, venueConfirmationAggregate{}, []string{"community"}},
		{"a confirmation alone is community", nil, venueEditAggregate{}, venueConfirmationAggregate{Count: 1}, []string{"community"}},
		{"both, in stable order", &ingest, venueEditAggregate{EditCount: 2, ContributorCount: 1}, venueConfirmationAggregate{Count: 3}, []string{"ingest", "community"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildVenueProvenance(updated, tc.dataSource, tc.edits, tc.confirms)
			if got.UpdatedAt != updated {
				t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, updated)
			}
			if len(got.Sources) != len(tc.want) {
				t.Fatalf("Sources = %v, want %v", got.Sources, tc.want)
			}
			for i := range tc.want {
				if got.Sources[i] != tc.want[i] {
					t.Fatalf("Sources = %v, want %v", got.Sources, tc.want)
				}
			}
		})
	}
}

// TestBuildVenueProvenance_SourcesSerializeAsArray guards the wire shape: an
// empty source list must marshal as [] rather than null, so the client can
// render it without a nil branch.
func TestBuildVenueProvenance_SourcesSerializeAsArray(t *testing.T) {
	p := buildVenueProvenance(time.Now().UTC(), nil, venueEditAggregate{}, venueConfirmationAggregate{})
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(decoded["sources"]) != "[]" {
		t.Errorf("sources serialized as %s, want []", decoded["sources"])
	}
	if _, ok := decoded["last_confirmed_at"]; ok {
		t.Error("last_confirmed_at must be omitted when there are no confirmations")
	}
}

// =============================================================================
// Integration: confirm + provenance
// =============================================================================

// confirmedVenueRows counts the physical rows for a venue, so the idempotence
// assertion checks the TABLE and not just the returned aggregate.
func (suite *VenueServiceIntegrationTestSuite) confirmedVenueRows(venueID uint) int64 {
	var n int64
	suite.Require().NoError(
		suite.db.Model(&catalogm.VenueConfirmation{}).Where("venue_id = ?", venueID).Count(&n).Error)
	return n
}

func (suite *VenueServiceIntegrationTestSuite) createApprovedVenueEdit(venueID, userID uint) {
	changes := json.RawMessage(`{"capacity":{"old":null,"new":250}}`)
	edit := &adminm.PendingEntityEdit{
		EntityType:   adminm.PendingEditEntityVenue,
		EntityID:     venueID,
		SubmittedBy:  userID,
		FieldChanges: &changes,
		Summary:      "Set capacity",
		Status:       adminm.PendingEditStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(edit).Error)
}

// TestConfirmVenue_RepeatIsIdempotentNoOp is the headline behaviour: a second
// tap must not error and must not inflate the count. Freshness evidence is
// only worth anything if one person counts once.
func (suite *VenueServiceIntegrationTestSuite) TestConfirmVenue_RepeatIsIdempotentNoOp() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Confirm Twice", "Austin", "TX", true)

	first, err := suite.venueService.ConfirmVenue(venue.ID, user.ID)
	suite.Require().NoError(err)
	suite.Equal(1, first.ConfirmationCount)
	suite.True(first.ViewerHasConfirmed)
	suite.Require().NotNil(first.LastConfirmedAt)

	second, err := suite.venueService.ConfirmVenue(venue.ID, user.ID)
	suite.Require().NoError(err, "a repeat confirm must be a no-op, not an error")
	suite.Equal(1, second.ConfirmationCount, "a repeat confirm must not inflate the count")
	suite.True(second.ViewerHasConfirmed)
	suite.Equal(int64(1), suite.confirmedVenueRows(venue.ID), "no duplicate row may be written")
	suite.Equal(first.LastConfirmedAt.UTC(), second.LastConfirmedAt.UTC(),
		"a repeat confirm must not move the last-confirmed timestamp")
}

// TestConfirmVenue_DistinctUsersEachCount is the other half of the invariant:
// idempotence is per USER, not per venue.
func (suite *VenueServiceIntegrationTestSuite) TestConfirmVenue_DistinctUsersEachCount() {
	a := suite.createTestUser()
	b := suite.createTestUser()
	venue := suite.createTestVenue("Confirm Two Users", "Austin", "TX", true)

	_, err := suite.venueService.ConfirmVenue(venue.ID, a.ID)
	suite.Require().NoError(err)
	resp, err := suite.venueService.ConfirmVenue(venue.ID, b.ID)
	suite.Require().NoError(err)

	suite.Equal(2, resp.ConfirmationCount)
	suite.Equal(int64(2), suite.confirmedVenueRows(venue.ID))
}

// TestConfirmVenue_UnknownVenueIs404 keeps a bad id a clean not-found rather
// than a foreign-key violation surfaced as a 500.
func (suite *VenueServiceIntegrationTestSuite) TestConfirmVenue_UnknownVenueIs404() {
	user := suite.createTestUser()
	_, err := suite.venueService.ConfirmVenue(99999999, user.ID)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "not found")
	suite.Equal(int64(0), suite.confirmedVenueRows(99999999))
}

// TestConfirmVenue_RequiresUser refuses an unauthenticated write at the service
// boundary too, so a caller that forgets the auth check cannot write an
// orphan row.
func (suite *VenueServiceIntegrationTestSuite) TestConfirmVenue_RequiresUser() {
	venue := suite.createTestVenue("Confirm Anon", "Austin", "TX", true)
	_, err := suite.venueService.ConfirmVenue(venue.ID, 0)
	suite.Require().Error(err)
	suite.Equal(int64(0), suite.confirmedVenueRows(venue.ID))
}

// TestGetVenue_ProvenanceStamp covers the venue-detail read: counts come from
// approved edits and confirmations, and contributors are DISTINCT people.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenue_ProvenanceStamp() {
	editor := suite.createTestUser()
	other := suite.createTestUser()
	venue := suite.createTestVenue("Provenance Detail", "Austin", "TX", true)

	suite.createApprovedVenueEdit(venue.ID, editor.ID)
	suite.createApprovedVenueEdit(venue.ID, editor.ID) // same person twice
	suite.createApprovedVenueEdit(venue.ID, other.ID)
	_, err := suite.venueService.ConfirmVenue(venue.ID, other.ID)
	suite.Require().NoError(err)

	got, err := suite.venueService.GetVenue(venue.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got.Provenance)
	suite.Equal(3, got.Provenance.EditCount)
	suite.Equal(2, got.Provenance.ContributorCount, "two edits by one person is one contributor")
	suite.Equal(1, got.Provenance.ConfirmationCount)
	suite.Require().NotNil(got.Provenance.LastConfirmedAt)
	suite.Equal([]string{contracts.VenueProvenanceSourceCommunity}, got.Provenance.Sources)
	suite.False(got.Provenance.UpdatedAt.IsZero())
}

// TestGetVenue_ProvenanceExcludesUnappliedEdits pins the count's meaning: a
// proposal that was never applied did not change what the reader is looking
// at, so counting it would overstate how curated the listing is.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenue_ProvenanceExcludesUnappliedEdits() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Provenance Pending", "Austin", "TX", true)

	changes := json.RawMessage(`{"capacity":{"old":null,"new":100}}`)
	for _, status := range []adminm.PendingEditStatus{
		adminm.PendingEditStatusPending,
		adminm.PendingEditStatusRejected,
	} {
		suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
			EntityType:   adminm.PendingEditEntityVenue,
			EntityID:     venue.ID,
			SubmittedBy:  user.ID,
			FieldChanges: &changes,
			Summary:      "Proposed",
			Status:       status,
		}).Error)
	}

	got, err := suite.venueService.GetVenue(venue.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got.Provenance)
	suite.Equal(0, got.Provenance.EditCount, "pending and rejected edits must not count")
	suite.Equal(0, got.Provenance.ContributorCount)
	suite.Empty(got.Provenance.Sources)
}

// TestGetVenue_ProvenanceIgnoresOtherEntityTypes guards the polymorphic table:
// an artist edit whose entity_id happens to equal a venue id must not leak
// into that venue's stamp.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenue_ProvenanceIgnoresOtherEntityTypes() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Provenance Type Scoped", "Austin", "TX", true)

	changes := json.RawMessage(`{"name":{"old":"a","new":"b"}}`)
	suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
		EntityType:   adminm.PendingEditEntityArtist,
		EntityID:     venue.ID,
		SubmittedBy:  user.ID,
		FieldChanges: &changes,
		Summary:      "Artist edit with a colliding id",
		Status:       adminm.PendingEditStatusApproved,
	}).Error)

	got, err := suite.venueService.GetVenue(venue.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got.Provenance)
	suite.Equal(0, got.Provenance.EditCount)
}

// TestGetVenuesWithShowCounts_ProvenanceIsRailOptIn covers the city-scoped
// list: the stamp rides IncludeRailFields, and the venue browse page (which
// renders none of it) must not pay for the aggregations.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_ProvenanceIsRailOptIn() {
	user := suite.createTestUser()
	withStamp := suite.createTestVenue("Rail Provenance A", "Austin", "TX", true)
	suite.createTestVenue("Rail Provenance B", "Austin", "TX", true)

	suite.createApprovedVenueEdit(withStamp.ID, user.ID)
	_, err := suite.venueService.ConfirmVenue(withStamp.ID, user.ID)
	suite.Require().NoError(err)

	railed, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	stamped := suite.findVenueResponse(railed, "Rail Provenance A")
	suite.Require().NotNil(stamped.Provenance)
	suite.Equal(1, stamped.Provenance.EditCount)
	suite.Equal(1, stamped.Provenance.ContributorCount)
	suite.Equal(1, stamped.Provenance.ConfirmationCount)

	// Every row on the page gets a stamp, including one with nothing to show —
	// an absent stamp and a zero stamp mean different things to a reader.
	untouched := suite.findVenueResponse(railed, "Rail Provenance B")
	suite.Require().NotNil(untouched.Provenance)
	suite.Equal(0, untouched.Provenance.EditCount)
	suite.Equal(0, untouched.Provenance.ConfirmationCount)
	suite.Nil(untouched.Provenance.LastConfirmedAt)

	plain, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX"}, 50, 0)
	suite.Require().NoError(err)
	suite.Nil(suite.findVenueResponse(plain, "Rail Provenance A").Provenance,
		"the browse page must not pay for the provenance aggregations")
}
