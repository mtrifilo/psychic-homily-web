package catalog

import (
	"context"
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

// TestGetVenueDetail_HasCalendarSource reads the detail through both of its
// entry points, id and slug, for a room with and without a registry row. A
// bare row (no URL, never refreshed) registers the room; a label row sharing
// the venue's id does not.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenueDetail_HasCalendarSource() {
	cases := []struct {
		name       string
		registerAs string // entity type of the registry row; "" for none
		want       bool
	}{
		{"no registry row", "", false},
		{"venue registry row", adminm.SourceEntityVenue, true},
		{"label row with the venue's id", adminm.SourceEntityLabel, false},
	}
	for _, tc := range cases {
		venue := suite.createSluggedVenue("Room " + tc.name)
		if tc.registerAs != "" {
			suite.registerSource(tc.registerAs, venue.ID)
		}
		for _, idOrSlug := range []string{fmt.Sprintf("%d", venue.ID), venue.Slug} {
			got, err := suite.venueService.GetVenueDetail(idOrSlug)
			suite.Require().NoError(err)
			suite.Require().NotNil(got.HasCalendarSource, "%s via %s", tc.name, idOrSlug)
			suite.Equal(tc.want, *got.HasCalendarSource, "%s via %s", tc.name, idOrSlug)
		}
	}
}

// A failed lookup leaves the field nil (absent on the wire), never false: an
// absent field renders the default empty line, while false would claim the
// calendar is not followed.
func (suite *VenueServiceIntegrationTestSuite) TestVenueHasCalendarSource_NilWhenLookupFails() {
	venue := suite.createSluggedVenue("Lookup Fails")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failing := &VenueService{db: suite.db.WithContext(ctx)}

	suite.Nil(failing.venueHasCalendarSource(venue.ID))
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
