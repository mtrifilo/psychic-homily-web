package catalog

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/services/contracts"
)

// Go struct tags must be constant literals, so the OpenAPI enum on
// Artist.set_type cannot be built from contracts.SetTypeVocabulary(). This
// test is the join: adding a value to the vocabulary without widening the tag
// would leave the API rejecting a value the service accepts (and the
// regenerated frontend types would silently disagree with the server).
func TestArtistSetTypeEnumTagMatchesVocabulary(t *testing.T) {
	field, ok := reflect.TypeOf(Artist{}).FieldByName("SetType")
	require.True(t, ok, "Artist.SetType must exist")

	assert.Equal(t, contracts.SetTypeVocabularyCSV(), field.Tag.Get("enum"),
		"the enum tag on Artist.set_type must list exactly the contracts vocabulary, in order")
}
