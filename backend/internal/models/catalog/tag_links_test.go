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

func TestResolveTagLinkMerge(t *testing.T) {
	t.Run("a column the target has none of carries", func(t *testing.T) {
		source := &Tag{
			Website:   tagLinkPtr("https://source.test"),
			Instagram: tagLinkPtr("https://instagram.com/source"),
		}
		carry, discarded := ResolveTagLinkMerge(source, &Tag{})
		assert.Empty(t, discarded)
		assert.Equal(t, map[string]any{
			"website":   "https://source.test",
			"instagram": "https://instagram.com/source",
		}, carry)
	})

	t.Run("a column both hold keeps the target's value and reports the loss", func(t *testing.T) {
		source := &Tag{Instagram: tagLinkPtr("https://instagram.com/source")}
		target := &Tag{Instagram: tagLinkPtr("https://instagram.com/target")}
		carry, discarded := ResolveTagLinkMerge(source, target)
		assert.Empty(t, carry)
		require.Len(t, discarded, 1)
		assert.Equal(t, TagLinkDiscard{
			Field:       "instagram",
			SourceValue: "https://instagram.com/source",
			TargetValue: "https://instagram.com/target",
		}, discarded[0])
	})

	t.Run("the same value on both sides costs nothing", func(t *testing.T) {
		same := "https://instagram.com/both"
		carry, discarded := ResolveTagLinkMerge(&Tag{Instagram: &same}, &Tag{Instagram: &same})
		assert.Empty(t, carry)
		assert.Empty(t, discarded)
	})

	t.Run("a blank column is no link on either side", func(t *testing.T) {
		blank := "   "
		value := "https://source.test"
		carry, discarded := ResolveTagLinkMerge(&Tag{Website: &blank}, &Tag{})
		assert.Empty(t, carry, "a blank source value is nothing to carry")
		assert.Empty(t, discarded)

		carry, discarded = ResolveTagLinkMerge(&Tag{Website: &value}, &Tag{Website: &blank})
		assert.Equal(t, map[string]any{"website": value}, carry,
			"a blank target column is empty, so the source's value carries into it")
		assert.Empty(t, discarded)
	})

	t.Run("a source value the write boundary refuses is carried nowhere", func(t *testing.T) {
		// Stored before the rule that refuses it, and unrenderable on either
		// row, so the merge does not move it and does not report it as a link
		// the target cost anyone.
		userinfo := "https://evil.test@instagram.com/x"
		offPlatform := "https://instagram.com.evil.test/x"
		for _, value := range []string{userinfo, offPlatform} {
			carry, discarded := ResolveTagLinkMerge(&Tag{Instagram: &value}, &Tag{})
			assert.Empty(t, carry, value)
			assert.Empty(t, discarded, value)
		}

		// The unanchored column is judged by its own rules, so any host carries.
		anyHost := "https://crew.example.test/"
		carry, _ := ResolveTagLinkMerge(&Tag{Website: &anyHost}, &Tag{})
		assert.Equal(t, map[string]any{"website": anyHost}, carry)
	})

	t.Run("every link column is resolved, in one order", func(t *testing.T) {
		source := &Tag{
			Website:   tagLinkPtr("https://source.test"),
			Instagram: tagLinkPtr("https://instagram.com/source"),
			Bandcamp:  tagLinkPtr("https://source.bandcamp.com"),
		}
		target := &Tag{
			Website:   tagLinkPtr("https://target.test"),
			Instagram: tagLinkPtr("https://instagram.com/target"),
			Bandcamp:  tagLinkPtr("https://target.bandcamp.com"),
		}
		_, discarded := ResolveTagLinkMerge(source, target)
		fields := make([]string, 0, len(discarded))
		for _, d := range discarded {
			fields = append(fields, d.Field)
		}
		assert.Equal(t, []string{"website", "instagram", "bandcamp"}, fields)
	})
}
