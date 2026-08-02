package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// UNIT TESTS -- role resolution (no database)
// =============================================================================

func TestResolveArtistRole(t *testing.T) {
	tests := []struct {
		name            string
		artist          contracts.CreateShowArtist
		position        int
		wantSetType     string
		wantIsHeadliner bool
	}{
		{
			name:            "no signal at position 0 infers the headliner",
			artist:          contracts.CreateShowArtist{Name: "Top Act"},
			position:        0,
			wantSetType:     contracts.SetTypeHeadliner,
			wantIsHeadliner: true,
		},
		{
			// The regression this ticket exists for: every non-headliner used
			// to be stamped "opener" purely because it was not first.
			name:            "no signal past position 0 gets the neutral default, NOT opener",
			artist:          contracts.CreateShowArtist{Name: "Second Act"},
			position:        1,
			wantSetType:     contracts.SetTypePerformer,
			wantIsHeadliner: false,
		},
		{
			name:            "legacy is_headliner true",
			artist:          contracts.CreateShowArtist{Name: "Top Act", IsHeadliner: boolPtr(true)},
			position:        3,
			wantSetType:     contracts.SetTypeHeadliner,
			wantIsHeadliner: true,
		},
		{
			name:            "legacy is_headliner false no longer means opener",
			artist:          contracts.CreateShowArtist{Name: "Support", IsHeadliner: boolPtr(false)},
			position:        1,
			wantSetType:     contracts.SetTypePerformer,
			wantIsHeadliner: false,
		},
		{
			// An explicit false at position 0 must stay false: the position
			// inference is only a fallback for "no signal at all".
			name:            "explicit is_headliner false at position 0 is respected",
			artist:          contracts.CreateShowArtist{Name: "First Billed", IsHeadliner: boolPtr(false)},
			position:        0,
			wantSetType:     contracts.SetTypePerformer,
			wantIsHeadliner: false,
		},
		{
			name:            "curated set_type wins over position",
			artist:          contracts.CreateShowArtist{Name: "Guest", SetType: strPtr(contracts.SetTypeSpecialGuest)},
			position:        0,
			wantSetType:     contracts.SetTypeSpecialGuest,
			wantIsHeadliner: false,
		},
		{
			// is_headliner is DERIVED from set_type, so a client may send
			// set_type alone and still get the headliner slot.
			name:            "curated headliner derives is_headliner without the flag",
			artist:          contracts.CreateShowArtist{Name: "Top Act", SetType: strPtr(contracts.SetTypeHeadliner)},
			position:        4,
			wantSetType:     contracts.SetTypeHeadliner,
			wantIsHeadliner: true,
		},
		{
			name:            "curated set_type wins over a contradicting is_headliner",
			artist:          contracts.CreateShowArtist{Name: "Opener", IsHeadliner: boolPtr(true), SetType: strPtr(contracts.SetTypeOpener)},
			position:        0,
			wantSetType:     contracts.SetTypeOpener,
			wantIsHeadliner: false,
		},
		{
			name:            "curated opener persists -- opener is still a real value when stated",
			artist:          contracts.CreateShowArtist{Name: "Opener", SetType: strPtr(contracts.SetTypeOpener)},
			position:        2,
			wantSetType:     contracts.SetTypeOpener,
			wantIsHeadliner: false,
		},
		{
			name:            "curated direct_support persists",
			artist:          contracts.CreateShowArtist{Name: "Support", SetType: strPtr(contracts.SetTypeDirectSupport)},
			position:        1,
			wantSetType:     contracts.SetTypeDirectSupport,
			wantIsHeadliner: false,
		},
		{
			name:            "curated dj persists",
			artist:          contracts.CreateShowArtist{Name: "DJ Spinz", SetType: strPtr(contracts.SetTypeDJ)},
			position:        3,
			wantSetType:     contracts.SetTypeDJ,
			wantIsHeadliner: false,
		},
		{
			name:            "whitespace-only set_type reads as absent",
			artist:          contracts.CreateShowArtist{Name: "Act", SetType: strPtr("   "), IsHeadliner: boolPtr(true)},
			position:        1,
			wantSetType:     contracts.SetTypeHeadliner,
			wantIsHeadliner: true,
		},
		{
			// The boundary validator rejects this first; resolution treats it
			// as absent rather than writing an unknown value to the column.
			name:            "out-of-vocabulary set_type falls back rather than writing through",
			artist:          contracts.CreateShowArtist{Name: "Act", SetType: strPtr("co-headliner")},
			position:        1,
			wantSetType:     contracts.SetTypePerformer,
			wantIsHeadliner: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setType, isHeadliner := resolveArtistRole(tt.artist, tt.position)
			assert.Equal(t, tt.wantSetType, setType)
			assert.Equal(t, tt.wantIsHeadliner, isHeadliner)
		})
	}
}

// Whatever resolution writes has to be a value the contract accepts, on every
// input shape -- including the malformed ones.
func TestResolveArtistRole_AlwaysWritesAVocabularyValue(t *testing.T) {
	inputs := []contracts.CreateShowArtist{
		{Name: "a"},
		{Name: "b", IsHeadliner: boolPtr(true)},
		{Name: "c", IsHeadliner: boolPtr(false)},
		{Name: "d", SetType: strPtr("")},
		{Name: "e", SetType: strPtr("garbage")},
		{Name: "f", SetType: strPtr(contracts.SetTypeDJ)},
	}
	for _, in := range inputs {
		for _, position := range []int{0, 1, 7} {
			setType, _ := resolveArtistRole(in, position)
			assert.True(t, contracts.IsValidSetType(setType), "resolveArtistRole wrote %q", setType)
		}
	}
}

func TestValidateShowArtistSetTypes(t *testing.T) {
	t.Run("accepts every vocabulary value", func(t *testing.T) {
		var artists []contracts.CreateShowArtist
		for _, value := range contracts.SetTypeVocabulary() {
			artists = append(artists, contracts.CreateShowArtist{Name: value, SetType: strPtr(value)})
		}
		assert.NoError(t, validateShowArtistSetTypes(artists))
	})

	t.Run("accepts an absent set_type", func(t *testing.T) {
		assert.NoError(t, validateShowArtistSetTypes([]contracts.CreateShowArtist{
			{Name: "No Curation"},
			{Name: "Blank", SetType: strPtr("")},
		}))
	})

	t.Run("accepts a nil slice", func(t *testing.T) {
		assert.NoError(t, validateShowArtistSetTypes(nil))
	})

	t.Run("rejects an unknown value and names the index", func(t *testing.T) {
		err := validateShowArtistSetTypes([]contracts.CreateShowArtist{
			{Name: "Fine", SetType: strPtr(contracts.SetTypeHeadliner)},
			{Name: "Bad", SetType: strPtr("co-headliner")},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "artists[1].set_type")
		assert.Contains(t, err.Error(), "co-headliner")
		// The message lists what IS allowed, so a client can fix it without
		// reading the source.
		assert.Contains(t, err.Error(), contracts.SetTypeVocabularyCSV())
	})

	t.Run("rejects wrong casing rather than coercing it", func(t *testing.T) {
		assert.Error(t, validateShowArtistSetTypes([]contracts.CreateShowArtist{
			{Name: "Shouty", SetType: strPtr("HEADLINER")},
		}))
	})

	t.Run("rejects an ingest-only alias", func(t *testing.T) {
		// "support" is meaningful to NormalizeSetType but is NOT part of the
		// API contract -- clients must send direct_support.
		assert.Error(t, validateShowArtistSetTypes([]contracts.CreateShowArtist{
			{Name: "Support", SetType: strPtr("support")},
		}))
	})
}

// The duplicate-headliner pre-check reads the request, not the resolved rows,
// so it has to see a headliner declared by set_type alone.
func TestRequestsHeadlinerSlot(t *testing.T) {
	tests := []struct {
		name   string
		artist contracts.CreateShowArtist
		want   bool
	}{
		{"set_type headliner alone", contracts.CreateShowArtist{SetType: strPtr(contracts.SetTypeHeadliner)}, true},
		{"legacy flag alone", contracts.CreateShowArtist{IsHeadliner: boolPtr(true)}, true},
		{"set_type overrides a true flag", contracts.CreateShowArtist{IsHeadliner: boolPtr(true), SetType: strPtr(contracts.SetTypeOpener)}, false},
		{"set_type overrides a false flag", contracts.CreateShowArtist{IsHeadliner: boolPtr(false), SetType: strPtr(contracts.SetTypeHeadliner)}, true},
		{"no signal", contracts.CreateShowArtist{}, false},
		{"non-headliner set_type", contracts.CreateShowArtist{SetType: strPtr(contracts.SetTypeDJ)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, requestsHeadlinerSlot(tt.artist))
		})
	}
}

// =============================================================================
// INTEGRATION TESTS -- what actually lands in show_artists
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NonHeadlinersDefaultToPerformerNotOpener() {
	user := suite.createTestUser()
	resp, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title:     "Uncurated Bill",
		EventDate: suite.uniqueEventDate(),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Set Type Room", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "Default Headliner", IsHeadliner: boolPtr(true)},
			{Name: "Default Second", IsHeadliner: boolPtr(false)},
			{Name: "Default Third", IsHeadliner: boolPtr(false)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	})
	suite.Require().NoError(err)

	stored := suite.storedSetTypes(resp.ID)
	suite.Equal([]string{
		contracts.SetTypeHeadliner,
		contracts.SetTypePerformer,
		contracts.SetTypePerformer,
	}, stored)

	// The response mirrors what was written, so a client never sees "opener"
	// for an act nobody described as one.
	suite.Equal(contracts.SetTypeHeadliner, resp.Artists[0].SetType)
	suite.Equal(contracts.SetTypePerformer, resp.Artists[1].SetType)
	suite.Equal(contracts.SetTypePerformer, resp.Artists[2].SetType)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_PersistsEveryCuratedSetType() {
	user := suite.createTestUser()
	vocabulary := contracts.SetTypeVocabulary()

	artists := make([]contracts.CreateShowArtist, 0, len(vocabulary))
	for i, value := range vocabulary {
		artists = append(artists, contracts.CreateShowArtist{
			Name:    fmt.Sprintf("Curated %s %d", value, i),
			SetType: strPtr(value),
		})
	}

	resp, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title:             "Fully Curated Bill",
		EventDate:         suite.uniqueEventDate(),
		City:              "Phoenix",
		State:             "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Curated Room", City: "Phoenix", State: "AZ"}},
		Artists:           artists,
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	})
	suite.Require().NoError(err)

	suite.Equal(vocabulary, suite.storedSetTypes(resp.ID))

	// Only the curated headliner carries is_headliner.
	for i, artist := range resp.Artists {
		suite.Equal(vocabulary[i] == contracts.SetTypeHeadliner, *artist.IsHeadliner, "artist %d (%s)", i, vocabulary[i])
	}
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_RejectsUnknownSetType() {
	user := suite.createTestUser()
	_, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title:     "Bad Bill",
		EventDate: suite.uniqueEventDate(),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Reject Room", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "Fine Act", IsHeadliner: boolPtr(true)},
			{Name: "Bad Act", SetType: strPtr("co-headliner")},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	})
	suite.Require().Error(err)
	suite.Contains(err.Error(), "artists[1].set_type")

	// Rejected before the transaction opens: no show, no artists.
	var showCount int64
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).Where("title = ?", "Bad Bill").Count(&showCount).Error)
	suite.Zero(showCount)
}

// A show created with only set_type (no is_headliner) must still trip the
// duplicate-headliner guard, which reads the request rather than the rows.
func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_SetTypeOnlyHeadlinerTripsDuplicateGuard() {
	user := suite.createTestUser()
	eventDate := suite.uniqueEventDate()
	venue := contracts.CreateShowVenue{Name: "Dup Guard Room", City: "Phoenix", State: "AZ"}

	build := func(title string) *contracts.CreateShowRequest {
		return &contracts.CreateShowRequest{
			Title:     title,
			EventDate: eventDate,
			City:      "Phoenix",
			State:     "AZ",
			Venues:    []contracts.CreateShowVenue{venue},
			Artists: []contracts.CreateShowArtist{
				{Name: "Set Type Only Headliner", SetType: strPtr(contracts.SetTypeHeadliner)},
			},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
	}

	_, err := suite.showService.CreateShow(build("First Booking"))
	suite.Require().NoError(err)

	_, err = suite.showService.CreateShow(build("Second Booking"))
	suite.Require().Error(err, "a set_type-only headliner must still be seen by the duplicate guard")
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_PersistsCuratedSetTypes() {
	show := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Bill To Curate"
		req.EventDate = suite.uniqueEventDate()
		req.Artists = []contracts.CreateShowArtist{
			{Name: "Edit Headliner", IsHeadliner: boolPtr(true)},
			{Name: "Edit Support", IsHeadliner: boolPtr(false)},
		}
	})

	// Before the edit the support act holds the neutral default.
	suite.Equal([]string{contracts.SetTypeHeadliner, contracts.SetTypePerformer}, suite.storedSetTypes(show.ID))

	updated, _, err := suite.showService.UpdateShowWithRelations(
		show.ID,
		&contracts.UpdateShowRequest{},
		nil,
		[]contracts.CreateShowArtist{
			{Name: "Edit Headliner", SetType: strPtr(contracts.SetTypeHeadliner)},
			{Name: "Edit Support", SetType: strPtr(contracts.SetTypeDirectSupport)},
			{Name: "Edit Opener", SetType: strPtr(contracts.SetTypeOpener)},
		},
		true,
	)
	suite.Require().NoError(err)

	suite.Equal([]string{
		contracts.SetTypeHeadliner,
		contracts.SetTypeDirectSupport,
		contracts.SetTypeOpener,
	}, suite.storedSetTypes(show.ID))
	suite.Require().Len(updated.Artists, 3)
	suite.Equal(contracts.SetTypeDirectSupport, updated.Artists[1].SetType)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_RejectsUnknownSetTypeWithoutTouchingTheBill() {
	show := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Bill To Preserve"
		req.EventDate = suite.uniqueEventDate()
		req.Artists = []contracts.CreateShowArtist{
			{Name: "Preserved Headliner", IsHeadliner: boolPtr(true)},
			{Name: "Preserved Support", IsHeadliner: boolPtr(false)},
		}
	})
	before := suite.storedSetTypes(show.ID)

	_, _, err := suite.showService.UpdateShowWithRelations(
		show.ID,
		&contracts.UpdateShowRequest{},
		nil,
		[]contracts.CreateShowArtist{
			{Name: "Preserved Headliner", SetType: strPtr(contracts.SetTypeHeadliner)},
			{Name: "Preserved Support", SetType: strPtr("nonsense")},
		},
		true,
	)
	suite.Require().Error(err)

	// replaceShowArtists deletes before it rebuilds, so validating late would
	// have left the bill destroyed on a rolled-back transaction.
	suite.Equal(before, suite.storedSetTypes(show.ID))
	var count int64
	suite.Require().NoError(suite.db.Model(&catalogm.ShowArtist{}).Where("show_id = ?", show.ID).Count(&count).Error)
	suite.EqualValues(2, count)
}

// storedSetTypes reads set_type straight off show_artists in bill order, so
// these assertions describe the DATABASE rather than the response mapper.
func (suite *ShowServiceIntegrationTestSuite) storedSetTypes(showID uint) []string {
	var rows []catalogm.ShowArtist
	suite.Require().NoError(
		suite.db.Where("show_id = ?", showID).Order("position ASC").Find(&rows).Error,
	)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.SetType)
	}
	return out
}

// uniqueEventDate keeps each case's bill clear of the
// (artist_id, venue_id, event_date) dedup index used by its neighbours.
func (suite *ShowServiceIntegrationTestSuite) uniqueEventDate() time.Time {
	setTypeTestDateOffset++
	return time.Date(2027, 3, 1, 20, 0, 0, 0, time.UTC).AddDate(0, 0, setTypeTestDateOffset)
}

var setTypeTestDateOffset int
