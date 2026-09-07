package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tagLinkPtr(s string) *string { return &s }

func TestTagLinksColumns(t *testing.T) {
	t.Run("an unsupplied field is absent, so an update leaves the column alone", func(t *testing.T) {
		cols := TagLinks{Website: tagLinkPtr("https://example.test")}.Columns()
		require.Len(t, cols, 1)
		assert.Equal(t, "https://example.test", cols["website"])
		_, hasInstagram := cols["instagram"]
		assert.False(t, hasInstagram)
		_, hasBandcamp := cols["bandcamp"]
		assert.False(t, hasBandcamp)
	})

	t.Run("an empty or whitespace-only value clears the column to NULL", func(t *testing.T) {
		cols := TagLinks{
			Website:   tagLinkPtr(""),
			Instagram: tagLinkPtr("   "),
			Bandcamp:  tagLinkPtr("\t\n"),
		}.Columns()
		require.Len(t, cols, 3)
		for _, column := range []string{"website", "instagram", "bandcamp"} {
			value, present := cols[column]
			require.True(t, present, column)
			assert.Nil(t, value, column)
		}
	})

	t.Run("a stored value is trimmed, so it is the string the host anchor read", func(t *testing.T) {
		cols := TagLinks{Instagram: tagLinkPtr("  https://instagram.com/psychichomily  ")}.Columns()
		assert.Equal(t, "https://instagram.com/psychichomily", cols["instagram"])
	})

	t.Run("each field lands on its own column", func(t *testing.T) {
		cols := TagLinks{
			Website:   tagLinkPtr("https://example.test"),
			Instagram: tagLinkPtr("https://instagram.com/a"),
			Bandcamp:  tagLinkPtr("https://a.bandcamp.com"),
		}.Columns()
		assert.Equal(t, "https://example.test", cols["website"])
		assert.Equal(t, "https://instagram.com/a", cols["instagram"])
		assert.Equal(t, "https://a.bandcamp.com", cols["bandcamp"])
	})
}

func TestTagLinksApply(t *testing.T) {
	t.Run("an unsupplied field leaves the stored value in place", func(t *testing.T) {
		tag := &Tag{
			Website:   tagLinkPtr("https://kept.test"),
			Instagram: tagLinkPtr("https://instagram.com/kept"),
			Bandcamp:  tagLinkPtr("https://kept.bandcamp.com"),
		}
		TagLinks{}.Apply(tag)
		assert.Equal(t, "https://kept.test", *tag.Website)
		assert.Equal(t, "https://instagram.com/kept", *tag.Instagram)
		assert.Equal(t, "https://kept.bandcamp.com", *tag.Bandcamp)
	})

	t.Run("an empty value clears the field", func(t *testing.T) {
		tag := &Tag{Website: tagLinkPtr("https://gone.test")}
		TagLinks{Website: tagLinkPtr("  ")}.Apply(tag)
		assert.Nil(t, tag.Website)
	})

	t.Run("a supplied value is stored trimmed", func(t *testing.T) {
		tag := &Tag{}
		TagLinks{Bandcamp: tagLinkPtr(" https://crew.bandcamp.com ")}.Apply(tag)
		require.NotNil(t, tag.Bandcamp)
		assert.Equal(t, "https://crew.bandcamp.com", *tag.Bandcamp)
	})
}
