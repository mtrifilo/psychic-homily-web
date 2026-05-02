package community_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/models/community"
)

// PSY-421: this consistency check used to live in catalog/tag_test.go but had
// to be moved here when the flat models package was split into sub-packages
// (catalog, community, ...). community imports catalog already, so the check
// belongs on this side of the boundary.
//
// Verify tag entity types match collection entity types (same values used
// project-wide).
func TestTagEntityTypesMatchCollectionEntityTypes(t *testing.T) {
	assert.Equal(t, community.CollectionEntityArtist, catalog.TagEntityArtist)
	assert.Equal(t, community.CollectionEntityRelease, catalog.TagEntityRelease)
	assert.Equal(t, community.CollectionEntityLabel, catalog.TagEntityLabel)
	assert.Equal(t, community.CollectionEntityShow, catalog.TagEntityShow)
	assert.Equal(t, community.CollectionEntityVenue, catalog.TagEntityVenue)
	assert.Equal(t, community.CollectionEntityFestival, catalog.TagEntityFestival)
}
