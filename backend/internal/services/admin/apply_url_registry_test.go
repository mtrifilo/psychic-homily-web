package admin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	handlershared "psychic-homily-backend/internal/api/handlers/shared"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

// TestRollbackURLFieldsCoverEveryEditableURLField is the tripwire rollbackURLFields
// was missing (PSY-1966 round 3).
//
// The other two apply-side registries each have one
// (TestFetchedURLFieldsMatchHandlerRegistry,
// TestShapedURLFieldsMatchHandlerRegistry), but those compare against a handler
// list. This one has to compare against something else, because rollbackURLFields
// answers a different question: "every URL field an APPLY can write", and what
// the two apply paths can write is the union of the per-entity edit allowlists.
//
// Adding one URL field to any *AllowedEditFields without adding it here reopens
// the hole on both paths at once: a contributor-supplied value reaching the
// column with none of its forward rules.
//
// WHAT IT DOES NOT CATCH, because a field counts as a URL field only when the
// canonical handler registry says so: a `*_url` column added to an allowlist and
// to NEITHER registry is skipped here silently. flyer_url is that shape today.
// Widening the definition of "URL field" is a change to urlFieldSpecs, which is
// where the definition belongs.
func TestRollbackURLFieldsCoverEveryEditableURLField(t *testing.T) {
	allowlists := map[string]map[string]bool{
		"artist":   catalogm.ArtistAllowedEditFields,
		"venue":    catalogm.VenueAllowedEditFields,
		"label":    catalogm.LabelAllowedEditFields,
		"festival": catalogm.FestivalAllowedEditFields,
		"release":  catalogm.ReleaseAllowedEditFields,
	}

	known := make(map[string]bool)
	for _, field := range handlershared.URLFieldNames() {
		known[field] = true
	}

	for entity, fields := range allowlists {
		for field := range fields {
			if !known[field] {
				continue // not a URL field; nothing to enforce
			}
			// bandcamp_embed_url is covered by shapedURLFields, which carries its
			// own tripwire against the handler registry.
			if field == utils.BandcampEmbedURLField {
				assert.Contains(t, shapedURLFields, field,
					"%s.%s is a URL field with a shape rule and must be re-validated on both apply paths",
					entity, field)
				continue
			}
			assert.Contains(t, rollbackURLFields, field,
				"%s.%s is an editable URL field, so an apply can write it: add it to "+
					"rollbackURLFields, or a contributor-supplied value reaches the column "+
					"with none of its forward rules", entity, field)
		}
	}
}

// TestSocialLabelsAgreeAcrossLayers turns a comment into a fact.
//
// The eight social columns are named for an operator in three packages: the HTTP
// boundary's urlFieldSpecs, this package's rollbackURLFields, and
// utils.SocialFieldLabels for the writers that sit outside both. utils is the
// only one all three can import, so it holds the source and this asserts the
// other two agree with it. Without this, a wording changed in one place leaves
// one operator seeing two names for one column and nothing fails.
func TestSocialLabelsAgreeAcrossLayers(t *testing.T) {
	assert.Len(t, utils.SocialFieldLabels, 8, "the eight social columns")

	for field, want := range utils.SocialFieldLabels {
		assert.Equal(t, want, rollbackURLFields[field],
			"the apply gate names %q differently from utils.SocialFieldLabels", field)

		got, known := handlershared.URLFieldDisplayName(field)
		if assert.True(t, known, "the handler registry does not know %q", field) {
			assert.Equal(t, want, got,
				"the HTTP boundary names %q differently from utils.SocialFieldLabels", field)
		}
	}
}

// TestApproveGateCoversEveryRollbackURLField pins that the two apply paths judge
// the SAME fields, so a value cannot be refused going backwards and accepted
// going forwards.
//
// An earlier revision of this branch ran approve on a SUBSET, excluding
// flyer_url on the reasoning that the submit handler had already checked it. It
// had not: TestFlyerURLHasNoSubmitSideRule below is the fact that reasoning got
// wrong, which is what made the exclusion a hole rather than a shortcut.
func TestApproveGateCoversEveryRollbackURLField(t *testing.T) {
	for field := range rollbackURLFields {
		assert.Error(t,
			validateApproveURLs(map[string]interface{}{field: "javascript:alert(1)"}),
			"approve must gate %q, which rollback gates", field)
	}
}

// TestFlyerURLHasNoSubmitSideRule is the fact the approve gate's field set rests
// on, asserted rather than described in a comment.
//
// shared.ValidateFieldChangeValue returns nil for any field absent from all
// three of its registries, and flyer_url is absent from all three, so a
// contributor can file any value at all. The column renders as an image source
// on the public festival page, which is why the approve gate must not skip it.
//
// If flyer_url is ever registered at submit this test fails, and the right
// response is to rewrite the comment it guards rather than delete the test:
// the approve gate would then be a second run of a rule instead of the only one.
func TestFlyerURLHasNoSubmitSideRule(t *testing.T) {
	assert.Contains(t, catalogm.FestivalAllowedEditFields, "flyer_url",
		"a contributor can edit it, which is what makes the gap reachable")
	assert.NotContains(t, handlershared.URLFieldNames(), "flyer_url",
		"registering it here would gate submit and make this test's premise stale")

	for _, value := range []string{"javascript:alert(1)", "/uploads/flyer.jpg", "not a url"} {
		assert.NoError(t,
			handlershared.ValidateFieldChangeValue(context.Background(), "flyer_url", value),
			"submit still accepts %q, so approve is the only gate on it", value)
	}
}
