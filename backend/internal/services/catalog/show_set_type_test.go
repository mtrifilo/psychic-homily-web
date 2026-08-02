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
			// Padding is NOT trimmed away into a valid value: the boundary
			// validator rejects it, and resolution treats it as absent rather
			// than accepting what the OpenAPI enum would have refused.
			name:            "padded set_type is not silently accepted",
			artist:          contracts.CreateShowArtist{Name: "Act", SetType: strPtr(" headliner ")},
			position:        1,
			wantSetType:     contracts.SetTypePerformer,
			wantIsHeadliner: false,
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
		// The message lists what IS allowed. Note this text is for LOGS and
		// in-process callers only: both HTTP handlers replace it with a
		// generic "failed to create/update show", so the vocabulary is never
		// echoed to a client. Do not "fix" that by surfacing the raw error.
		assert.Contains(t, err.Error(), contracts.SetTypeVocabularyCSV())
	})

	t.Run("rejects wrong casing rather than coercing it", func(t *testing.T) {
		assert.Error(t, validateShowArtistSetTypes([]contracts.CreateShowArtist{
			{Name: "Shouty", SetType: strPtr("HEADLINER")},
		}))
	})

	t.Run("rejects a padded value, matching the OpenAPI enum exactly", func(t *testing.T) {
		// The service must not be quietly laxer than the published contract:
		// the enum 422s " headliner ", so an in-process caller gets the same
		// verdict rather than a silently trimmed pass.
		assert.Error(t, validateShowArtistSetTypes([]contracts.CreateShowArtist{
			{Name: "Padded", SetType: strPtr(" headliner ")},
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

// The duplicate-headliner pre-check must see EVERY row that will be written as
// a headliner, including one the caller never flagged and never named a slot
// for -- associateArtists infers that row from position 0, so a pre-check that
// did not would lock and probe a different set than it writes.
func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_PositionZeroWithNoSignalIsDuplicateChecked() {
	user := suite.createTestUser()
	eventDate := suite.uniqueEventDate()
	venue := contracts.CreateShowVenue{Name: "Inferred Headliner Room", City: "Phoenix", State: "AZ"}

	build := func(title string) *contracts.CreateShowRequest {
		return &contracts.CreateShowRequest{
			Title:     title,
			EventDate: eventDate,
			City:      "Phoenix",
			State:     "AZ",
			Venues:    []contracts.CreateShowVenue{venue},
			Artists: []contracts.CreateShowArtist{
				// No set_type and no is_headliner: resolved to headliner from
				// position 0 alone.
				{Name: "Inferred Headliner"},
				// An explicit headliner further down the bill. Before the
				// pre-check resolved with real positions, this entry alone
				// filled headlinerNames, which suppressed the first-billed
				// fallback and left the inferred headliner unchecked.
				{Name: "Named Headliner", SetType: strPtr(contracts.SetTypeHeadliner)},
			},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
	}

	first, err := suite.showService.CreateShow(build("First Inferred Booking"))
	suite.Require().NoError(err)
	suite.Equal([]string{contracts.SetTypeHeadliner, contracts.SetTypeHeadliner}, suite.storedSetTypes(first.ID))

	_, err = suite.showService.CreateShow(build("Second Inferred Booking"))
	suite.Require().Error(err, "the position-inferred headliner must be duplicate-checked too")
}

// ConfirmShowImport carries curated roles through from the export frontmatter
// instead of collapsing them to "was this the headliner".
func (suite *ShowServiceIntegrationTestSuite) TestConfirmShowImport_PersistsAndNormalizesFrontmatterSetTypes() {
	eventDate := suite.uniqueEventDate().Format(time.RFC3339)

	markdown := fmt.Sprintf(`---
show:
  title: Imported Curated Bill
  event_date: "%s"
  city: Phoenix
  state: AZ
venues:
  - name: Import Room
    city: Phoenix
    state: AZ
artists:
  - name: Import Headliner
    position: 0
    set_type: headliner
  - name: Import Support
    position: 1
    set_type: support
  - name: Import Spinner
    position: 2
    set_type: dj
  - name: Import Host
    position: 3
    set_type: host
---

Imported by the PSY-1673 set_type test.
`, eventDate)

	resp, err := suite.showService.ConfirmShowImport([]byte(markdown), true)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.Equal([]string{
		contracts.SetTypeHeadliner,
		// "support" is a stated role, mapped rather than flattened.
		contracts.SetTypeDirectSupport,
		contracts.SetTypeDJ,
		// The vocabulary models no host slot, so it defaults rather than
		// guessing -- and is NOT promoted to headliner by position.
		contracts.SetTypeDefault,
	}, suite.storedSetTypes(resp.ID))
}
