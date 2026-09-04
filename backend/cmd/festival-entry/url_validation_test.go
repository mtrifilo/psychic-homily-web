package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateFestivalURLs covers the gate this CLI applies in place of the
// handler validation it bypasses by calling the service directly.
func TestValidateFestivalURLs(t *testing.T) {
	t.Run("absent columns pass", func(t *testing.T) {
		assert.NoError(t, validateFestivalURLs(&FestivalInput{Name: "No Links Fest"}))
	})

	t.Run("absolute http urls pass", func(t *testing.T) {
		assert.NoError(t, validateFestivalURLs(&FestivalInput{
			Name:      "Good Fest",
			Website:   "https://goodfest.example.org",
			TicketURL: "http://tickets.example.org/goodfest",
		}))
	})

	// The request-time spelling: no legacy tolerance, because every value here is
	// typed for this run.
	t.Run("a non-http scheme is refused on either column", func(t *testing.T) {
		for _, value := range []string{
			"javascript:alert(1)",
			"data:text/html,<script>alert(1)</script>",
			"goodfest.example.org",
		} {
			assert.Error(t, validateFestivalURLs(&FestivalInput{Name: "Bad Fest", Website: value}), value)
			assert.Error(t, validateFestivalURLs(&FestivalInput{Name: "Bad Fest", TicketURL: value}), value)
		}
	})

	t.Run("the refusal names the festival and the column", func(t *testing.T) {
		err := validateFestivalURLs(&FestivalInput{Name: "Bad Fest", Website: "javascript:alert(1)"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Bad Fest")
		assert.Contains(t, err.Error(), "Website URL")
	})
}

// TestCheckedInExamplesClearTheGate is the tripwire the seed's fixtures have:
// the checked-in example payloads are what an operator copies, so one that the
// gate refuses would teach the wrong shape and fail on first use.
func TestCheckedInExamplesClearTheGate(t *testing.T) {
	paths, err := filepath.Glob("examples/*.json")
	require.NoError(t, err)
	require.NotEmpty(t, paths, "the example path is wrong if this is empty")

	for _, path := range paths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err, path)
		var input FestivalInput
		require.NoError(t, json.Unmarshal(raw, &input), path)
		assert.NoError(t, validateFestivalURLs(&input), path)
	}
}
