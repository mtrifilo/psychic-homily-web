package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// TableName Tests
// =============================================================================

func TestTagTableName(t *testing.T) {
	assert.Equal(t, "tags", Tag{}.TableName())
}

func TestEntityTagTableName(t *testing.T) {
	assert.Equal(t, "entity_tags", EntityTag{}.TableName())
}

func TestTagVoteTableName(t *testing.T) {
	assert.Equal(t, "tag_votes", TagVote{}.TableName())
}

func TestTagAliasTableName(t *testing.T) {
	assert.Equal(t, "tag_aliases", TagAlias{}.TableName())
}

// =============================================================================
// IsValidTagCategory Tests
// =============================================================================

func TestIsValidTagCategory_Valid(t *testing.T) {
	for _, c := range TagCategories {
		assert.True(t, IsValidTagCategory(c), "expected %q to be valid", c)
	}
}

func TestIsValidTagCategory_Invalid(t *testing.T) {
	assert.False(t, IsValidTagCategory(""))
	assert.False(t, IsValidTagCategory("invalid"))
	assert.False(t, IsValidTagCategory("Genre")) // case-sensitive
	assert.False(t, IsValidTagCategory("GENRE"))
}

// =============================================================================
// IsValidTagEntityType Tests
// =============================================================================

func TestIsValidTagEntityType_Valid(t *testing.T) {
	for _, et := range TagEntityTypes {
		assert.True(t, IsValidTagEntityType(et), "expected %q to be valid", et)
	}
}

func TestIsValidTagEntityType_Invalid(t *testing.T) {
	assert.False(t, IsValidTagEntityType(""))
	assert.False(t, IsValidTagEntityType("invalid"))
	assert.False(t, IsValidTagEntityType("Artist")) // case-sensitive
	assert.False(t, IsValidTagEntityType("user"))
}

// =============================================================================
// Constants Tests
// =============================================================================

func TestTagCategoryConstants(t *testing.T) {
	assert.Equal(t, "genre", TagCategoryGenre)
	assert.Equal(t, "locale", TagCategoryLocale)
	assert.Equal(t, "other", TagCategoryOther)
	assert.Len(t, TagCategories, 3)
}

func TestTagEntityTypeConstants(t *testing.T) {
	assert.Equal(t, "artist", TagEntityArtist)
	assert.Equal(t, "release", TagEntityRelease)
	assert.Equal(t, "label", TagEntityLabel)
	assert.Equal(t, "show", TagEntityShow)
	assert.Equal(t, "venue", TagEntityVenue)
	assert.Equal(t, "festival", TagEntityFestival)
	// PSY-354: collections share the polymorphic entity_tags table.
	assert.Equal(t, "collection", TagEntityCollection)
	assert.Len(t, TagEntityTypes, 7)
}

// PSY-354: collection entity tag round-trips through IsValidTagEntityType.
func TestIsValidTagEntityType_Collection(t *testing.T) {
	assert.True(t, IsValidTagEntityType(TagEntityCollection))
	assert.True(t, IsValidTagEntityType("collection"))
}
