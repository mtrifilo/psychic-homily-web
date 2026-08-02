package contracts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The vocabulary is the API contract for show_artists.set_type. Pinning it
// here means a value cannot be added, removed, or reordered without a
// deliberate edit to this list -- the OpenAPI enum tag on the show handler and
// the frontend's SetType union both have to move with it.
func TestSetTypeVocabulary_IsPinned(t *testing.T) {
	assert.Equal(t, []string{
		"headliner",
		"direct_support",
		"opener",
		"special_guest",
		"dj",
		"performer",
	}, SetTypeVocabulary())

	assert.Equal(t, "headliner,direct_support,opener,special_guest,dj,performer", SetTypeVocabularyCSV())
}

func TestSetTypeVocabulary_ReturnsACopy(t *testing.T) {
	got := SetTypeVocabulary()
	got[0] = "mutated"

	assert.Equal(t, SetTypeHeadliner, SetTypeVocabulary()[0], "callers must not be able to mutate the vocabulary")
}

// The default has to stay the semantically empty value. If it ever becomes a
// specific role, every uncurated row on the site starts asserting that role.
func TestSetTypeDefault_IsTheNeutralValue(t *testing.T) {
	assert.Equal(t, SetTypePerformer, SetTypeDefault)
	assert.True(t, IsValidSetType(SetTypeDefault))
}

func TestIsValidSetType_IsStrict(t *testing.T) {
	for _, value := range SetTypeVocabulary() {
		assert.True(t, IsValidSetType(value), "vocabulary value %q must validate", value)
	}

	// Rejected on purpose: coercing these would let a client's guess through
	// as a curated fact. Ingest gets leniency via NormalizeSetType; the API
	// does not.
	for _, value := range []string{
		"",
		"Headliner",
		"HEADLINER",
		" headliner",
		"support",
		"host",
		"co-headliner",
		"opener ",
	} {
		assert.False(t, IsValidSetType(value), "%q must not validate", value)
	}
}

func TestNormalizeSetType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"identity", "headliner", SetTypeHeadliner},
		{"case insensitive", "HEADLINER", SetTypeHeadliner},
		{"whitespace trimmed", "  headliner  ", SetTypeHeadliner},

		// The extraction prompt emits "support" only for an act the source
		// billed under "w/" or "with" -- that IS the direct support slot, so
		// the mapping preserves a stated fact rather than inventing one.
		{"support is a stated role", "support", SetTypeDirectSupport},
		{"support case insensitive", "Support", SetTypeDirectSupport},
		{"direct_support identity", "direct_support", SetTypeDirectSupport},

		// A source that says "opener" has curated the opening slot. That is
		// still a real value; what changed is that nothing DEFAULTS to it.
		{"opener stays opener when stated", "opener", SetTypeOpener},

		{"special guest", "special_guest", SetTypeSpecialGuest},
		{"dj is first class", "dj", SetTypeDJ},
		{"dj case insensitive", "DJ", SetTypeDJ},
		{"performer identity", "performer", SetTypePerformer},

		// The vocabulary models no host/MC slot. Recording a host as a
		// performer would be a lossy guess, so it stays unmapped and the
		// caller supplies the default.
		{"host is unmapped", "host", ""},
		{"mc is unmapped", "mc", ""},

		{"empty", "", ""},
		{"unknown", "co-headliner", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeSetType(tt.input))
		})
	}
}

// Everything NormalizeSetType maps must be something the API would accept,
// otherwise ingest can write a value the contract forbids.
func TestNormalizeSetType_OnlyEverProducesVocabularyValues(t *testing.T) {
	for _, input := range []string{
		"headliner", "support", "direct_support", "opener", "special_guest",
		"dj", "performer", "host", "", "nonsense",
	} {
		out := NormalizeSetType(input)
		if out == "" {
			continue
		}
		assert.True(t, IsValidSetType(out), "NormalizeSetType(%q) produced %q, which is not in the vocabulary", input, out)
	}
}
