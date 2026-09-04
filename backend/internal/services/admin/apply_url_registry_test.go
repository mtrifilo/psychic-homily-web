package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	handlershared "psychic-homily-backend/internal/api/handlers/shared"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

// TestApplyURLFieldsCoverEveryEditableURLField is the tripwire applyURLFields
// was missing (PSY-1966 round 3).
//
// The other two apply-side registries each have one
// (TestFetchedURLFieldsMatchHandlerRegistry,
// TestShapedURLFieldsMatchHandlerRegistry), but those compare against a handler
// list. This one has to compare against something else, because applyURLFields
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
func TestApplyURLFieldsCoverEveryEditableURLField(t *testing.T) {
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
			assert.Contains(t, applyURLFields, field,
				"%s.%s is an editable URL field, so an apply can write it: add it to "+
					"applyURLFields, or a contributor-supplied value reaches the column "+
					"with none of its forward rules", entity, field)
		}
	}
}

// TestSocialLabelsAgreeAcrossLayers turns a comment into a fact.
//
// The eight social columns are named for an operator in three packages: the HTTP
// boundary's urlFieldSpecs, this package's applyURLFields, and
// utils.SocialFieldLabels for the writers that sit outside both. utils is the
// only one all three can import, so it holds the source and this asserts the
// other two agree with it. Without this, a wording changed in one place leaves
// one operator seeing two names for one column and nothing fails.
func TestSocialLabelsAgreeAcrossLayers(t *testing.T) {
	assert.Len(t, utils.SocialFieldLabels, 8, "the eight social columns")

	for field, want := range utils.SocialFieldLabels {
		assert.Equal(t, want, applyURLFields[field],
			"the apply gate names %q differently from utils.SocialFieldLabels", field)

		got, known := handlershared.URLFieldDisplayName(field)
		if assert.True(t, known, "the handler registry does not know %q", field) {
			assert.Equal(t, want, got,
				"the HTTP boundary names %q differently from utils.SocialFieldLabels", field)
		}
	}
}

// TestApproveURLFieldsMatchHandlerRegistry derives the approve-side set rather
// than trusting the literal, so the difference between the two apply paths stays
// a RULE and not a hand-maintained exception.
//
// The rule: approve re-checks a field iff the submit handler already checked it,
// which is exactly applyURLFields intersected with the handler's URL registry.
// A field registered there later comes under the approve gate with no edit here;
// one dropped from applyURLFields leaves both.
func TestApproveURLFieldsMatchHandlerRegistry(t *testing.T) {
	known := make(map[string]bool)
	for _, field := range handlershared.URLFieldNames() {
		known[field] = true
	}

	want := map[string]string{}
	for field, displayName := range applyURLFields {
		if known[field] {
			want[field] = displayName
		}
	}
	assert.Equal(t, want, approveURLFields)

	// The one field the two sets differ by today, named so a change to it is a
	// deliberate edit here rather than a silent pass.
	assert.NotContains(t, approveURLFields, "flyer_url",
		"flyer_url is length-only at submit, so refusing it at approve strands an edit a contributor was allowed to file")
	assert.Contains(t, applyURLFields, "flyer_url",
		"rollback writes an OldValue nothing ever validated, so it keeps the wider set")
}
