package catalog

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoinRadioTrackKeys(t *testing.T) {
	k := func(artist, title string) radioTrackKey { return radioTrackKey{Artist: artist, Title: title} }
	pairs := []radioTrackMBPair{
		// p and q share m1; q and r share m2: one group of three.
		{"p", "song", "m1"},
		{"q", "song", "m1"},
		{"q", "song", "m2"},
		{"r", "song", "m2"},
		// s carries two ids nobody else carries: no group.
		{"s", "song", "m3"},
		{"s", "song", "m4"},
		// u and t share m5: a second group.
		{"u", "other", "m5"},
		{"t", "other", "m5"},
	}
	want := [][]radioTrackKey{
		{k("p", "song"), k("q", "song"), k("r", "song")},
		{k("t", "other"), k("u", "other")},
	}
	assert.Equal(t, want, joinRadioTrackKeys(pairs))

	rng := rand.New(rand.NewPCG(1, 2))
	for range 20 {
		shuffled := append([]radioTrackMBPair(nil), pairs...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		assert.Equal(t, want, joinRadioTrackKeys(shuffled), "the groups depend only on the set of pairs")
	}

	assert.Empty(t, joinRadioTrackKeys(nil))
}

func TestCanonicalRadioTrackKey(t *testing.T) {
	a := radioTrackKey{Artist: "a", Title: "x"}
	b := radioTrackKey{Artist: "b", Title: "x"}
	c := radioTrackKey{Artist: "c", Title: "x"}
	group := []radioTrackKey{a, b, c}

	assert.Equal(t, a, canonicalRadioTrackKey(group, nil), "no existing row: the smallest key")
	assert.Equal(t, c, canonicalRadioTrackKey(group, map[radioTrackKey]uint{c: 7}), "the member with a row keeps it")
	assert.Equal(t, b, canonicalRadioTrackKey(group, map[radioTrackKey]uint{c: 9, b: 4}), "the oldest row wins")
}
