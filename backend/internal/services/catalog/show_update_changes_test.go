package catalog

import (
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

func TestShowColumnHolds(t *testing.T) {
	at := time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC)
	price := 12.5
	image := "https://example.com/flyer.jpg"
	show := &catalogm.Show{
		Title:     "Stored",
		EventDate: at,
		DoorsAt:   &at,
		City:      stringPtr("Phoenix"),
		Price:     &price,
		ImageURL:  &image,
	}
	otherPrice := 13.0
	later := at.Add(time.Hour)
	otherImage := "https://example.com/other.jpg"

	cases := []struct {
		name   string
		column string
		value  interface{}
		holds  bool
	}{
		{"same title", "title", "Stored", true},
		{"new title", "title", "Retitled", false},
		{"same instant in another zone", "event_date", at.In(time.FixedZone("MST", -7*3600)), true},
		{"sub-microsecond difference is the same stored instant", "event_date", at.Add(400 * time.Nanosecond), true},
		{"a microsecond apart is a change", "event_date", at.Add(time.Microsecond), false},
		{"same doors time", "doors_at", &at, true},
		{"moved doors time", "doors_at", &later, false},
		{"clearing a set doors time", "doors_at", (*time.Time)(nil), false},
		{"clearing an empty music time", "music_at", (*time.Time)(nil), true},
		{"filling an empty music time", "music_at", &at, false},
		{"same city", "city", "Phoenix", true},
		{"empty string over NULL is a change", "state", "", false},
		{"same price", "price", &price, true},
		{"new price", "price", &otherPrice, false},
		{"clearing an empty door price", "door_price", (*float64)(nil), true},
		{"same image", "image_url", &image, true},
		{"new image", "image_url", &otherImage, false},
		{"clearing a set image", "image_url", (*string)(nil), false},
		{"unknown column always writes", "status", "approved", false},
		{"unexpected value type writes", "title", 7, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := showColumnHolds(show, tc.column, tc.value); got != tc.holds {
				t.Fatalf("showColumnHolds(%q, %v) = %v, want %v", tc.column, tc.value, got, tc.holds)
			}
		})
	}
}

func TestChangedShowColumns_KeepsOnlyChanges(t *testing.T) {
	at := time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC)
	show := &catalogm.Show{Title: "Stored", EventDate: at, City: stringPtr("Phoenix")}
	changed := changedShowColumns(show, map[string]interface{}{
		"title":      "Stored",
		"event_date": at,
		"city":       "Tucson",
	})
	if len(changed) != 1 || changed["city"] != "Tucson" {
		t.Fatalf("changedShowColumns = %v, want only city", changed)
	}
}

// pinShowUpdatedAt moves a show's updated_at to a fixed past instant so a
// test can tell "not written" from "written within the same second".
func (suite *ShowServiceIntegrationTestSuite) pinShowUpdatedAt(showID uint) time.Time {
	pinned := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	suite.Require().NoError(suite.db.Exec("UPDATE shows SET updated_at = ? WHERE id = ?", pinned, showID).Error)
	return pinned
}

func (suite *ShowServiceIntegrationTestSuite) storedShowUpdatedAt(showID uint) time.Time {
	var show catalogm.Show
	suite.Require().NoError(suite.db.First(&show, showID).Error)
	return show.UpdatedAt
}

// A save that restates every field the row already holds is the shape a
// re-import sends for a show it matched. It must leave updated_at alone,
// because updated_at is the show's sitemap lastmod.
func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_UnchangedSaveLeavesUpdatedAt() {
	doors := time.Date(2026, 6, 15, 19, 0, 0, 0, time.UTC)
	music := doors.Add(time.Hour)
	created := suite.createTestShow()
	_, err := suite.showService.UpdateShow(created.ID, &contracts.UpdateShowRequest{
		DoorsAt:        contracts.NullableSet(doors),
		MusicAt:        contracts.NullableSet(music),
		Price:          contracts.NullableSet(15.0),
		AgeRequirement: stringPtr("21+"),
		Description:    stringPtr("Night one"),
		TicketURL:      stringPtr("https://tickets.example.com/1"),
		ImageURL:       stringPtr("https://example.com/flyer.jpg"),
	})
	suite.Require().NoError(err)
	pinned := suite.pinShowUpdatedAt(created.ID)

	before, err := suite.showService.GetShow(created.ID)
	suite.Require().NoError(err)
	restated := &contracts.UpdateShowRequest{
		Title:          &before.Title,
		EventDate:      &before.EventDate,
		DoorsAt:        contracts.NullableSet(doors),
		MusicAt:        contracts.NullableSet(music),
		City:           before.City,
		State:          before.State,
		Price:          contracts.NullableSet(15.0),
		DoorPrice:      contracts.NullableClear[float64](),
		AgeRequirement: stringPtr("21+"),
		Description:    stringPtr("Night one"),
		TicketURL:      stringPtr("https://tickets.example.com/1"),
		ImageURL:       stringPtr("https://example.com/flyer.jpg"),
	}
	resp, _, err := suite.showService.UpdateShowWithRelations(created.ID, restated, nil, nil, true)
	suite.Require().NoError(err)
	suite.Equal(before.Title, resp.Title)
	suite.True(pinned.Equal(suite.storedShowUpdatedAt(created.ID)), "a save that changes nothing must not move updated_at")

	// The service-level UpdateShow takes the same path.
	_, err = suite.showService.UpdateShow(created.ID, restated)
	suite.Require().NoError(err)
	suite.True(pinned.Equal(suite.storedShowUpdatedAt(created.ID)), "UpdateShow with nothing changed must not move updated_at")
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_ChangedSaveMovesUpdatedAt() {
	created := suite.createTestShow()
	pinned := suite.pinShowUpdatedAt(created.ID)

	before, err := suite.showService.GetShow(created.ID)
	suite.Require().NoError(err)
	music := time.Date(2026, 6, 15, 21, 0, 0, 0, time.UTC)
	resp, _, err := suite.showService.UpdateShowWithRelations(created.ID, &contracts.UpdateShowRequest{
		Title:   &before.Title, // restated, unchanged
		MusicAt: contracts.NullableSet(music),
	}, nil, nil, true)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp.MusicAt)
	suite.Equal(music.Unix(), resp.MusicAt.Unix())
	suite.True(suite.storedShowUpdatedAt(created.ID).After(pinned), "a save that changes a column must move updated_at")
}
