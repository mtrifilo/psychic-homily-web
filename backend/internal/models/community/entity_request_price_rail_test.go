// Package community_test is an EXTERNAL test package on purpose.
//
// services/contracts imports models/community, so an internal (`package
// community`) test file cannot import contracts without creating an import
// cycle. An external test package is compiled separately and may import both,
// which is what lets the duplicated price ceiling fail closed.
package community_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntityRequestPriceRailMatchesContracts holds the entity-request queue's
// price ceiling to the same rail the direct show-create and show-update paths
// enforce (PSY-1864).
//
// The queue keeps its OWN copy of the bound (maxRequestPrice) because models
// must not import services. Nothing downstream catches a drift: shows.price and
// shows.door_price are DECIMAL(10,2), so any value under 99,999,999.99 INSERTs
// cleanly and publishes on the show page. The failure is silent by
// construction, so the guard has to be a test.
//
// Asserted through the VALIDATOR rather than by comparing constants, so the
// unexported bound stays unexported and the test pins the behavior a caller
// actually gets.
func TestEntityRequestPriceRailMatchesContracts(t *testing.T) {
	payload := func(field string, value float64) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(
			`{"title":"Boris","event_date":"2026-07-04","%s":%g}`, field, value))
	}

	// Both halves of the split ride the same rail.
	for _, field := range []string{"price", "door_price"} {
		t.Run(field+" accepts the contract ceiling", func(t *testing.T) {
			require.NoError(t, community.ValidateEntityRequestPayload(
				community.EntityRequestShow, payload(field, contracts.MaxShowPrice)),
				"queue rejects a value the direct show API accepts; maxRequestPrice drifted below contracts.MaxShowPrice")
		})

		t.Run(field+" rejects just above the contract ceiling", func(t *testing.T) {
			err := community.ValidateEntityRequestPayload(
				community.EntityRequestShow, payload(field, contracts.MaxShowPrice+1))
			assert.Error(t, err,
				"queue accepts a value the direct show API refuses; maxRequestPrice drifted above contracts.MaxShowPrice. "+
					"DECIMAL(10,2) will not catch it -- the row inserts and publishes")
		})

		t.Run(field+" accepts the contract floor", func(t *testing.T) {
			require.NoError(t, community.ValidateEntityRequestPayload(
				community.EntityRequestShow, payload(field, contracts.MinShowPrice)),
				"a zero price is FREE, a real fact; the queue must accept it")
		})

		t.Run(field+" rejects just below the contract floor", func(t *testing.T) {
			assert.Error(t, community.ValidateEntityRequestPayload(
				community.EntityRequestShow, payload(field, contracts.MinShowPrice-1)))
		})
	}
}
