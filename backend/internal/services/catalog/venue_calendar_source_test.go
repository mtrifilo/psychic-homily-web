package catalog

import (
	"encoding/json"
	"fmt"
	"testing"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// createSluggedVenue goes through CreateVenue because it is what assigns the
// slug the venue page addresses rooms by.
func (suite *VenueServiceIntegrationTestSuite) createSluggedVenue(name string) *contracts.VenueDetailResponse {
	created, err := suite.venueService.CreateVenue(&contracts.CreateVenueRequest{
		Name: name, City: "Austin", State: "TX",
	}, true)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(created.Slug)
	return created
}

func (suite *VenueServiceIntegrationTestSuite) registerSource(entityType string, entityID uint) {
	suite.Require().NoError(suite.db.Create(&adminm.SourceConfig{
		EntityType: entityType,
		EntityID:   entityID,
	}).Error)
}

// detailByIDAndSlug reads the venue detail through both of its entry points.
func (suite *VenueServiceIntegrationTestSuite) detailByIDAndSlug(v *contracts.VenueDetailResponse) map[string]*contracts.VenueDetailResponse {
	byID, err := suite.venueService.GetVenueDetail(fmt.Sprintf("%d", v.ID))
	suite.Require().NoError(err)
	bySlug, err := suite.venueService.GetVenueDetail(v.Slug)
	suite.Require().NoError(err)
	return map[string]*contracts.VenueDetailResponse{"by id": byID, "by slug": bySlug}
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueDetail_HasCalendarSource_FalseWithoutRegistryRow() {
	venue := suite.createSluggedVenue("Unfollowed Room")

	for entry, got := range suite.detailByIDAndSlug(venue) {
		suite.Require().NotNil(got.HasCalendarSource, entry)
		suite.False(*got.HasCalendarSource, entry)
	}
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueDetail_HasCalendarSource_TrueWithRegistryRow() {
	venue := suite.createSluggedVenue("Followed Room")
	// No URL and never refreshed: the row alone is what registers the source.
	suite.registerSource(adminm.SourceEntityVenue, venue.ID)

	for entry, got := range suite.detailByIDAndSlug(venue) {
		suite.Require().NotNil(got.HasCalendarSource, entry)
		suite.True(*got.HasCalendarSource, entry)
	}
}

// A label row whose entity_id happens to equal the venue's id is a different
// entity and registers nothing for the venue.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenueDetail_HasCalendarSource_IgnoresOtherEntityTypes() {
	venue := suite.createSluggedVenue("Label Id Twin")
	suite.registerSource(adminm.SourceEntityLabel, venue.ID)

	for entry, got := range suite.detailByIDAndSlug(venue) {
		suite.Require().NotNil(got.HasCalendarSource, entry)
		suite.False(*got.HasCalendarSource, entry)
	}
}

// The identity lookups and the list payload never compute the fact, so they
// must leave it nil (absent on the wire) rather than claim false.
func (suite *VenueServiceIntegrationTestSuite) TestHasCalendarSource_OnlyOnTheDetailRead() {
	venue := suite.createSluggedVenue("Not On Lists")
	suite.registerSource(adminm.SourceEntityVenue, venue.ID)

	byID, err := suite.venueService.GetVenue(venue.ID)
	suite.Require().NoError(err)
	suite.Nil(byID.HasCalendarSource, "GetVenue is an identity lookup")

	bySlug, err := suite.venueService.GetVenueBySlug(venue.Slug)
	suite.Require().NoError(err)
	suite.Nil(bySlug.HasCalendarSource, "GetVenueBySlug is an identity lookup")

	// The browse list shows verified rooms only.
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).
		Where("id = ?", venue.ID).Update("verified", true).Error)
	// With and without the rail fields, the list variant that pays for extras.
	for _, rail := range []bool{false, true} {
		list, _, err := suite.venueService.GetVenuesWithShowCounts(
			contracts.VenueListFilters{IncludeRailFields: rail}, 50, 0)
		suite.Require().NoError(err)
		suite.Require().Len(list, 1)
		suite.Equal(venue.ID, list[0].ID)
		suite.Nil(list[0].HasCalendarSource, "list payloads do not carry the field (rail=%v)", rail)
	}
}

// TestHasCalendarSource_WireShape pins the three wire states: an undetermined
// field is absent, and a determined false is sent, not dropped.
func TestHasCalendarSource_WireShape(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name    string
		value   *bool
		want    string
		present bool
	}{
		{"undetermined is absent", nil, "", false},
		{"registered is true", &yes, "true", true},
		{"unregistered is false", &no, "false", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(contracts.VenueDetailResponse{HasCalendarSource: tc.value})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got, ok := decoded["has_calendar_source"]
			if ok != tc.present {
				t.Fatalf("has_calendar_source present = %v, want %v (%s)", ok, tc.present, raw)
			}
			if ok && string(got) != tc.want {
				t.Errorf("has_calendar_source = %s, want %s", got, tc.want)
			}
		})
	}
}
