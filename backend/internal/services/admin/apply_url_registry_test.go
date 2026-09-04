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
// Without this, adding one URL field to any *AllowedEditFields silently reopens
// the hole on both paths at once: a contributor-supplied value reaching the
// column with none of its forward rules, and nothing fails.
//
// A field counts as a URL field when the canonical handler registry says so, so
// this cannot drift from what "is a URL field" means everywhere else.
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
