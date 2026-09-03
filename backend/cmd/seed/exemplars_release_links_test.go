package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/utils"
)

// TestExemplarReleaseLinksPassTheGate is the tripwire for the one release-link
// writer that does not go through ReleaseService (PSY-1996).
//
// seedExemplarRelease inserts these rows with tx.Create, and a seed failure
// there degrades to a logged warning, so without this a literal that stopped
// clearing the gate would silently produce an exemplar whose Listen / Buy grid
// renders nothing, in a run nobody reads.
func TestExemplarReleaseLinksPassTheGate(t *testing.T) {
	require.NotEmpty(t, exemplarReleaseLinks)
	for _, l := range exemplarReleaseLinks {
		assert.NoError(t, utils.ValidateReleaseLink(l.Platform, l.URL),
			"exemplar link %s", l.Platform)
	}
}
