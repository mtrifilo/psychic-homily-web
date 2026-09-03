package catalog

import (
	"fmt"
	"strings"
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

// The update path's bill-level rule (PSY-1860): position inference is a
// fallback for an act nobody placed, and it has to stop once some OTHER act has
// named itself the headliner -- otherwise the act the caller never designated is
// written as a second headliner and outranks the curated one on every reader's
// `set_type='headliner' ORDER BY position ASC LIMIT 1`.
func TestSuppressPositionInferenceWhenHeadlinerNamed(t *testing.T) {
	t.Run("a named headliner silences inference on the silent acts", func(t *testing.T) {
		out := suppressPositionInferenceWhenHeadlinerNamed([]contracts.CreateShowArtist{
			{Name: "Earth"},
			{Name: "Boris", SetType: strPtr(contracts.SetTypeHeadliner)},
		})
		require.Len(t, out, 2)
		require.NotNil(t, out[0].IsHeadliner)
		assert.False(t, *out[0].IsHeadliner)
		// The act that stated a role is never rewritten.
		assert.Nil(t, out[1].IsHeadliner)
		assert.Equal(t, contracts.SetTypeHeadliner, *out[1].SetType)
	})

	t.Run("the legacy is_headliner flag also names a headliner", func(t *testing.T) {
		out := suppressPositionInferenceWhenHeadlinerNamed([]contracts.CreateShowArtist{
			{Name: "Earth"},
			{Name: "Boris", IsHeadliner: boolPtr(true)},
		})
		require.NotNil(t, out[0].IsHeadliner)
		assert.False(t, *out[0].IsHeadliner)
		assert.True(t, *out[1].IsHeadliner)
	})

	t.Run("an undescribed bill is left alone so position 0 still infers", func(t *testing.T) {
		in := []contracts.CreateShowArtist{{Name: "Earth"}, {Name: "Boris"}}
		out := suppressPositionInferenceWhenHeadlinerNamed(in)
		for i := range out {
			assert.Nil(t, out[i].IsHeadliner, "artist %d", i)
		}
	})

	// The deliberate NON-change, pinned so a future edit cannot widen the
	// trigger silently. Nobody claims the top of this bill, so suppressing would
	// write the silent top act as 'performer' -- and headlineSlotSQL would then
	// read the bill as curated with no headline slot and classify the genuine
	// top act as a SUPPORT slot (eligible for Openers to Watch). That is the
	// shape PSY-1704 calls a write-path defect; PSY-1705's wider rule disagrees,
	// and PSY-1860 declines to settle it here.
	t.Run("a described bill with no named headliner keeps position inference", func(t *testing.T) {
		in := []contracts.CreateShowArtist{
			{Name: "Earth"},
			{Name: "Boris", SetType: strPtr(contracts.SetTypeOpener)},
		}
		out := suppressPositionInferenceWhenHeadlinerNamed(in)
		assert.Nil(t, out[0].IsHeadliner, "the silent top act must keep its 'stated nothing' signal")
	})

	// 'performer' is one of the two spellings of "slot unknown"
	// (headlineSlotUnknownValues), so it names no headliner and must not arm
	// the suppression.
	t.Run("set_type performer names no headliner", func(t *testing.T) {
		in := []contracts.CreateShowArtist{
			{Name: "Earth"},
			{Name: "Boris", SetType: strPtr(contracts.SetTypePerformer)},
		}
		out := suppressPositionInferenceWhenHeadlinerNamed(in)
		assert.Nil(t, out[0].IsHeadliner)
	})

	// An explicit is_headliner:false is a statement about that act only, not a
	// nomination, so it must not cost the silent top act its inference.
	t.Run("is_headliner false names no headliner", func(t *testing.T) {
		in := []contracts.CreateShowArtist{
			{Name: "Earth"},
			{Name: "Boris", IsHeadliner: boolPtr(false)},
		}
		out := suppressPositionInferenceWhenHeadlinerNamed(in)
		assert.Nil(t, out[0].IsHeadliner)
	})

	// Rule 1 outranks rule 2 in resolveArtistRole, so a curated non-headliner
	// role beats a contradicting flag here too: this act does NOT claim the slot.
	t.Run("curated opener outranks a contradicting is_headliner flag", func(t *testing.T) {
		in := []contracts.CreateShowArtist{
			{Name: "Earth"},
			{Name: "Boris", SetType: strPtr(contracts.SetTypeOpener), IsHeadliner: boolPtr(true)},
		}
		out := suppressPositionInferenceWhenHeadlinerNamed(in)
		assert.Nil(t, out[0].IsHeadliner)
	})

	t.Run("whitespace-only set_type is silence, matching resolveArtistRole", func(t *testing.T) {
		out := suppressPositionInferenceWhenHeadlinerNamed([]contracts.CreateShowArtist{
			{Name: "Earth", SetType: strPtr("   ")},
			{Name: "Boris", SetType: strPtr(contracts.SetTypeHeadliner)},
		})
		require.NotNil(t, out[0].IsHeadliner)
		assert.False(t, *out[0].IsHeadliner)
	})

	// validateShowArtistSetTypes rejects this bill before the helper runs on the
	// update path, so this pins the predicate's standalone correctness: reading
	// an out-of-vocabulary value as "stated" would skip the act, let rule 3
	// promote it, and reintroduce the second headliner.
	t.Run("out-of-vocabulary set_type is silence, matching resolveArtistRole", func(t *testing.T) {
		out := suppressPositionInferenceWhenHeadlinerNamed([]contracts.CreateShowArtist{
			{Name: "Earth", SetType: strPtr("co-headliner")},
			{Name: "Boris", SetType: strPtr(contracts.SetTypeHeadliner)},
		})
		require.NotNil(t, out[0].IsHeadliner)
		assert.False(t, *out[0].IsHeadliner)
	})

	t.Run("does not mutate the caller's slice", func(t *testing.T) {
		in := []contracts.CreateShowArtist{
			{Name: "Earth"},
			{Name: "Boris", SetType: strPtr(contracts.SetTypeHeadliner)},
		}
		_ = suppressPositionInferenceWhenHeadlinerNamed(in)
		assert.Nil(t, in[0].IsHeadliner, "the input bill must be left untouched")
	})

	t.Run("a nil bill stays nil", func(t *testing.T) {
		// replaceShowArtists reads nil as "leave the associations untouched";
		// an allocated empty slice would tear the whole bill down instead.
		assert.Nil(t, suppressPositionInferenceWhenHeadlinerNamed(nil))
	})

	t.Run("a fully stated bill is returned unchanged", func(t *testing.T) {
		in := []contracts.CreateShowArtist{
			{Name: "Earth", SetType: strPtr(contracts.SetTypeHeadliner)},
			{Name: "Boris", IsHeadliner: boolPtr(false)},
		}
		out := suppressPositionInferenceWhenHeadlinerNamed(in)
		assert.Equal(t, in, out)
	})

	// The documented NON-guarantee, pinned so it cannot be inverted in silence.
	// Two acts that each state 'headliner' are the caller's own bill, not an
	// inference, and nothing on this path validates headliner COUNT. A future
	// count check or a trigger widened past the anySilent guard would change
	// this, and should have to change this test to do it.
	t.Run("two stated headliners are left alone -- this is not a count guard", func(t *testing.T) {
		in := []contracts.CreateShowArtist{
			{Name: "Earth", SetType: strPtr(contracts.SetTypeHeadliner)},
			{Name: "Boris", SetType: strPtr(contracts.SetTypeHeadliner)},
		}
		out := suppressPositionInferenceWhenHeadlinerNamed(in)
		assert.Equal(t, in, out)
		for i, a := range out {
			setType, isHeadliner := resolveArtistRole(a, i)
			assert.Equal(t, contracts.SetTypeHeadliner, setType, "artist %d", i)
			assert.True(t, isHeadliner, "artist %d", i)
		}
	})
}

// statesBillRole is the OTHER half of the trigger and is hand-maintained, so it
// gets the guard claimsHeadlineSlot no longer needs (that one is now derived).
//
// Its contract is exactly "rule 1 or rule 2 applies", i.e. this act is out of
// reach of the rule 3 position fallback -- which is observable as
// resolveArtistRole returning the SAME verdict at position 0 as at any later
// position. If it ever drifts to true for an act that still falls to rule 3, the
// helper would skip that act, rule 3 would promote it, and the PSY-1860 defect
// would be back.
func TestStatesBillRoleMeansPositionIndependent(t *testing.T) {
	cases := []struct {
		artist contracts.CreateShowArtist
		stated bool
	}{
		{contracts.CreateShowArtist{Name: "curated headliner", SetType: strPtr(contracts.SetTypeHeadliner)}, true},
		{contracts.CreateShowArtist{Name: "curated opener", SetType: strPtr(contracts.SetTypeOpener)}, true},
		{contracts.CreateShowArtist{Name: "curated performer", SetType: strPtr(contracts.SetTypePerformer)}, true},
		{contracts.CreateShowArtist{Name: "flag true", IsHeadliner: boolPtr(true)}, true},
		{contracts.CreateShowArtist{Name: "flag false", IsHeadliner: boolPtr(false)}, true},
		{contracts.CreateShowArtist{Name: "silent"}, false},
		{contracts.CreateShowArtist{Name: "blank set_type"}, false},
		{contracts.CreateShowArtist{Name: "whitespace set_type", SetType: strPtr("   ")}, false},
		{contracts.CreateShowArtist{Name: "garbage set_type", SetType: strPtr("co-headliner")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.artist.Name, func(t *testing.T) {
			assert.Equal(t, tc.stated, statesBillRole(tc.artist))

			firstType, firstHeadliner := resolveArtistRole(tc.artist, 0)
			laterType, laterHeadliner := resolveArtistRole(tc.artist, anyPositionPastFirst)
			positionIndependent := firstType == laterType && firstHeadliner == laterHeadliner
			assert.Equal(t, tc.stated, positionIndependent,
				"statesBillRole must mean exactly 'rule 3 cannot reach this act'")
		})
	}
}

// claimsHeadlineSlot is derived from resolveArtistRole, so this is a
// characterization test: it states, in one place, which shapes the trigger reads
// as naming a headliner.
func TestClaimsHeadlineSlot(t *testing.T) {
	cases := []struct {
		artist contracts.CreateShowArtist
		claims bool
	}{
		{contracts.CreateShowArtist{Name: "a", SetType: strPtr(contracts.SetTypeHeadliner)}, true},
		{contracts.CreateShowArtist{Name: "b", SetType: strPtr(contracts.SetTypeOpener)}, false},
		{contracts.CreateShowArtist{Name: "c", SetType: strPtr(contracts.SetTypePerformer)}, false},
		// Rule 1 outranks rule 2: a curated non-headliner beats the flag.
		{contracts.CreateShowArtist{Name: "d", SetType: strPtr(contracts.SetTypeDJ), IsHeadliner: boolPtr(true)}, false},
		{contracts.CreateShowArtist{Name: "e", IsHeadliner: boolPtr(true)}, true},
		{contracts.CreateShowArtist{Name: "f", IsHeadliner: boolPtr(false)}, false},
		{contracts.CreateShowArtist{Name: "g", SetType: strPtr(contracts.SetTypeHeadliner), IsHeadliner: boolPtr(false)}, true},
		// States nothing, so it claims nothing -- rule 3 is not consulted here.
		{contracts.CreateShowArtist{Name: "h"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.artist.Name, func(t *testing.T) {
			assert.Equal(t, tc.claims, claimsHeadlineSlot(tc.artist))
		})
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

// The decisive PSY-1860 case. On main this stores TWO headliner rows: the
// caller designated Boris and said nothing about Earth, and position inference
// promoted Earth anyway -- so every reader that resolves the one headliner
// (`set_type='headliner' ORDER BY position ASC LIMIT 1`) returns Earth, the act
// nobody chose, for the slug, search, explore cards, tags and dedup display.
func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_StatedBillWritesExactlyOneHeadliner() {
	show := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Bill To Restate"
		req.EventDate = suite.uniqueEventDate()
		req.Artists = []contracts.CreateShowArtist{{Name: "Original Act", IsHeadliner: boolPtr(true)}}
	})

	updated, _, err := suite.showService.UpdateShowWithRelations(
		show.ID,
		&contracts.UpdateShowRequest{},
		nil,
		[]contracts.CreateShowArtist{
			// States nothing: first-in-list is not a second opinion.
			{Name: "Earth"},
			{Name: "Boris", SetType: strPtr(contracts.SetTypeHeadliner)},
		},
		true,
	)
	suite.Require().NoError(err)

	suite.Equal([]string{contracts.SetTypePerformer, contracts.SetTypeHeadliner}, suite.storedSetTypes(show.ID))
	suite.Equal(1, suite.storedHeadlinerCount(show.ID), "an update must never write a second, unstated headliner")

	// The response mirrors the rows, so the caller sees the bill they stated.
	suite.Require().Len(updated.Artists, 2)
	suite.False(*updated.Artists[0].IsHeadliner)
	suite.True(*updated.Artists[1].IsHeadliner)
}

// Same corruption through the legacy flag rather than set_type: an update that
// marks the SECOND act is_headliner:true and leaves the first alone.
func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_LegacyFlagAlsoSilencesPositionInference() {
	show := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Bill To Restate By Flag"
		req.EventDate = suite.uniqueEventDate()
		req.Artists = []contracts.CreateShowArtist{{Name: "Flag Original Act", IsHeadliner: boolPtr(true)}}
	})

	_, _, err := suite.showService.UpdateShowWithRelations(
		show.ID,
		&contracts.UpdateShowRequest{},
		nil,
		[]contracts.CreateShowArtist{
			{Name: "Flag Silent Act"},
			{Name: "Flag Named Headliner", IsHeadliner: boolPtr(true)},
		},
		true,
	)
	suite.Require().NoError(err)

	suite.Equal([]string{contracts.SetTypePerformer, contracts.SetTypeHeadliner}, suite.storedSetTypes(show.ID))
	suite.Equal(1, suite.storedHeadlinerCount(show.ID))
}

// The other half of the rule: an update whose bill NOBODY describes keeps
// resolveArtistRole's position-0 fallback. Suppressing unconditionally would
// leave an undescribed bill with no headline slot at all.
func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_UncuratedBillKeepsPositionInference() {
	show := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Bill To Leave Undescribed"
		req.EventDate = suite.uniqueEventDate()
		req.Artists = []contracts.CreateShowArtist{{Name: "Undescribed Original", IsHeadliner: boolPtr(true)}}
	})

	updated, _, err := suite.showService.UpdateShowWithRelations(
		show.ID,
		&contracts.UpdateShowRequest{},
		nil,
		[]contracts.CreateShowArtist{
			{Name: "Undescribed First"},
			{Name: "Undescribed Second"},
			{Name: "Undescribed Third"},
		},
		true,
	)
	suite.Require().NoError(err)

	suite.Equal([]string{
		contracts.SetTypeHeadliner,
		contracts.SetTypePerformer,
		contracts.SetTypePerformer,
	}, suite.storedSetTypes(show.ID))
	suite.Require().Len(updated.Artists, 3)
	suite.True(*updated.Artists[0].IsHeadliner)
}

// The trigger's boundary, pinned end to end: an update that describes the bill
// but names NO headliner keeps position inference, so the genuine top act is
// still written into the headline slot. Suppressing here would leave the show
// with zero headliner rows, and headlineSlotSQL would then count that top act as
// a SUPPORT slot. Its create-path twin is
// TestCreateShow_PartiallyCuratedBillKeepsAHeadlineSlot.
func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_DescribedBillWithNoNamedHeadlinerKeepsInference() {
	show := suite.createTestShow(func(req *contracts.CreateShowRequest) {
		req.Title = "Bill With No Named Headliner"
		req.EventDate = suite.uniqueEventDate()
		req.Artists = []contracts.CreateShowArtist{{Name: "No Named Original", IsHeadliner: boolPtr(true)}}
	})

	_, _, err := suite.showService.UpdateShowWithRelations(
		show.ID,
		&contracts.UpdateShowRequest{},
		nil,
		[]contracts.CreateShowArtist{
			{Name: "No Named Top Act"},
			{Name: "No Named Opener", SetType: strPtr(contracts.SetTypeOpener)},
		},
		true,
	)
	suite.Require().NoError(err)

	suite.Equal([]string{contracts.SetTypeHeadliner, contracts.SetTypeOpener}, suite.storedSetTypes(show.ID))
	suite.Equal(1, suite.storedHeadlinerCount(show.ID))
}

// The create-path twin of TestUpdateShowWithRelations_StatedBillWritesExactlyOneHeadliner.
// CreateShow applies the same suppression, so a caller that names one headliner
// and leaves another act silent gets exactly the bill it stated.
func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_StatedBillWritesExactlyOneHeadliner() {
	user := suite.createTestUser()
	created, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title:     "Created Stated Bill",
		EventDate: suite.uniqueEventDate(),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Created Stated Room", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			// States nothing: first-in-list is not a second opinion.
			{Name: "Created Earth"},
			{Name: "Created Boris", SetType: strPtr(contracts.SetTypeHeadliner)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	})
	suite.Require().NoError(err)

	suite.Equal([]string{contracts.SetTypePerformer, contracts.SetTypeHeadliner}, suite.storedSetTypes(created.ID))
	suite.Equal(1, suite.storedHeadlinerCount(created.ID), "a create must never write a second, unstated headliner")
}

// The create-path twin of
// TestUpdateShowWithRelations_DescribedBillWithNoNamedHeadlinerKeepsInference,
// and the case PSY-1943 is about: a bill that curates an opener and says nothing
// about the top act still gets a headline slot, so headlineSlotSQL does not read
// the genuine headliner as a support slot and put it in Openers to Watch.
func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_PartiallyCuratedBillKeepsAHeadlineSlot() {
	user := suite.createTestUser()
	created, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title:     "Created Partially Curated Bill",
		EventDate: suite.uniqueEventDate(),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Created Partial Room", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "Created Silent Top Act"},
			{Name: "Created Curated Opener", SetType: strPtr(contracts.SetTypeOpener)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	})
	suite.Require().NoError(err)

	suite.Equal([]string{contracts.SetTypeHeadliner, contracts.SetTypeOpener}, suite.storedSetTypes(created.ID))
	suite.Equal(1, suite.storedHeadlinerCount(created.ID))
}

// The create-path twin of
// TestUpdateShowWithRelations_UncuratedBillKeepsPositionInference. A bill nobody
// described keeps the position-0 fallback rather than storing every act as
// 'performer', which is what pinning a silent act's flag false produces.
func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_UndescribedBillInfersPositionZero() {
	user := suite.createTestUser()
	created, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title:     "Created Undescribed Bill",
		EventDate: suite.uniqueEventDate(),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []contracts.CreateShowVenue{{Name: "Created Undescribed Room", City: "Phoenix", State: "AZ"}},
		Artists: []contracts.CreateShowArtist{
			{Name: "Created Undescribed First"},
			{Name: "Created Undescribed Second"},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	})
	suite.Require().NoError(err)

	suite.Equal([]string{contracts.SetTypeHeadliner, contracts.SetTypePerformer}, suite.storedSetTypes(created.ID))
	suite.Equal(1, suite.storedHeadlinerCount(created.ID))
}

// storedHeadlinerCount counts the rows a write actually put into the headliner
// slot. storedSetTypes alone would still pass a bill that promoted the wrong act
// to a sole headliner.
func (suite *ShowServiceIntegrationTestSuite) storedHeadlinerCount(showID uint) int {
	var count int64
	suite.Require().NoError(
		suite.db.Model(&catalogm.ShowArtist{}).
			Where("show_id = ? AND set_type = ?", showID, contracts.SetTypeHeadliner).
			Count(&count).Error,
	)
	return int(count)
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

// The duplicate pre-check must see the act at POSITION 0 whatever role that act
// ends up storing, because the stored-row half of the guard matches position 0.
//
// The bill names a headliner further down, so the top act is suppressed to
// 'performer' and its own signal claims nothing: the ONLY thing that puts it in
// the probe set is its bill index. Delete that arm and this case notices, while
// a bill nobody describes would not, because there the top act claims the slot
// through its own signal too. Only the top act repeats across the two bookings,
// so the refusal can come from the pre-check alone; the message assertions prove
// it did rather than the unique index catching the same collision as a driver
// error.
func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_PositionZeroWithNoSignalIsDuplicateChecked() {
	user := suite.createTestUser()
	eventDate := suite.uniqueEventDate()
	venue := contracts.CreateShowVenue{Name: "Inferred Headliner Room", City: "Phoenix", State: "AZ"}

	build := func(title, headlinerName string) *contracts.CreateShowRequest {
		return &contracts.CreateShowRequest{
			Title:     title,
			EventDate: eventDate,
			City:      "Phoenix",
			State:     "AZ",
			Venues:    []contracts.CreateShowVenue{venue},
			Artists: []contracts.CreateShowArtist{
				// No set_type and no is_headliner.
				{Name: "Inferred Headliner"},
				{Name: headlinerName, SetType: strPtr(contracts.SetTypeHeadliner)},
			},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
	}

	first, err := suite.showService.CreateShow(build("First Inferred Booking", "Named Headliner A"))
	suite.Require().NoError(err)
	suite.Equal([]string{contracts.SetTypePerformer, contracts.SetTypeHeadliner}, suite.storedSetTypes(first.ID),
		"the top act stores 'performer', which is what makes its probe index-driven")

	_, err = suite.showService.CreateShow(build("Second Inferred Booking", "Named Headliner B"))
	suite.Require().Error(err, "the position-0 act must be duplicate-checked too")
	suite.Contains(err.Error(), "'Inferred Headliner' is already performing",
		"the pre-check refused this, so the unique index never had to")
	suite.NotContains(strings.ToLower(err.Error()), "duplicated key")
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
