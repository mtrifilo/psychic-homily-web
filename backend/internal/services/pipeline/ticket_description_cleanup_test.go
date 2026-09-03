package pipeline

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitTicketDescription(t *testing.T) {
	cases := []struct {
		name        string
		description string
		wantChanged bool
		wantURL     string
		wantDesc    *string
		wantNonURL  bool
	}{
		{
			name:        "vendor line is the whole description",
			description: "Tickets: https://www.ticketweb.com/event/1",
			wantChanged: true,
			wantURL:     "https://www.ticketweb.com/event/1",
			wantDesc:    nil,
		},
		{
			name:        "vendor line trails the time parts",
			description: "Doors: 7ish | Show: 8ish | Tickets: https://dice.fm/event/2",
			wantChanged: true,
			wantURL:     "https://dice.fm/event/2",
			wantDesc:    strPtr("Doors: 7ish | Show: 8ish"),
		},
		{
			name:        "vendor line sits between other parts",
			description: "Doors: 7ish | Tickets: https://dice.fm/event/3 | Show: 8ish",
			wantChanged: true,
			wantURL:     "https://dice.fm/event/3",
			wantDesc:    strPtr("Doors: 7ish | Show: 8ish"),
		},
		{
			name:        "first url wins when the line repeats",
			description: "Tickets: https://a.example/1 | Tickets: https://b.example/2",
			wantChanged: true,
			wantURL:     "https://a.example/1",
			wantDesc:    nil,
		},
		{
			name:        "prose is left alone",
			description: "Tickets: at the door only",
			wantChanged: false,
			wantNonURL:  true,
		},
		{
			name:        "scheme-less value is left alone",
			description: "Tickets: ticketweb.com/event/4",
			wantChanged: false,
			wantNonURL:  true,
		},
		{
			name:        "a non-http scheme is left alone",
			description: "Tickets: javascript:alert(1)",
			wantChanged: false,
			wantNonURL:  true,
		},
		{
			name:        "a description with no vendor line is untouched",
			description: "Doors: 7ish | Show: 8ish",
			wantChanged: false,
		},
		{
			name:        "the prefix mid-part is not the writer's line",
			description: "Doors: 7ish, Tickets: https://a.example/5",
			wantChanged: false,
		},
		{
			name:        "prose part survives beside a stripped url part",
			description: "Tickets: at the door only | Tickets: https://a.example/6",
			wantChanged: true,
			wantURL:     "https://a.example/6",
			wantDesc:    strPtr("Tickets: at the door only"),
			wantNonURL:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitTicketDescription(tc.description)
			assert.Equal(t, tc.wantChanged, got.Changed)
			assert.Equal(t, tc.wantURL, got.TicketURL)
			assert.Equal(t, tc.wantNonURL, got.SawNonURL)
			if tc.wantDesc == nil {
				assert.Nil(t, got.Description)
				return
			}
			require.NotNil(t, got.Description)
			assert.Equal(t, *tc.wantDesc, *got.Description)
		})
	}
}

// Running the split over its own output changes nothing, which is what makes a
// second CLI pass a no-op.
func TestSplitTicketDescriptionIsIdempotent(t *testing.T) {
	first := splitTicketDescription("Doors: 7ish | Tickets: https://dice.fm/event/7")
	require.True(t, first.Changed)
	require.NotNil(t, first.Description)

	second := splitTicketDescription(*first.Description)
	assert.False(t, second.Changed)
}

func TestScrapedTicketURL(t *testing.T) {
	assert.Nil(t, scrapedTicketURL(nil))
	assert.Nil(t, scrapedTicketURL(strPtr("")))
	assert.Nil(t, scrapedTicketURL(strPtr("   ")))

	got := scrapedTicketURL(strPtr("  https://dice.fm/event/8  "))
	require.NotNil(t, got)
	assert.Equal(t, "https://dice.fm/event/8", *got)

	// Wider than the column: dropped rather than truncated into a broken
	// destination.
	oversize := "https://a.example/" + strings.Repeat("x", maxTicketURLLen)
	assert.Nil(t, scrapedTicketURL(&oversize))
}

func TestIsAbsoluteHTTPURL(t *testing.T) {
	assert.True(t, isAbsoluteHTTPURL("https://dice.fm/e/1"))
	assert.True(t, isAbsoluteHTTPURL("http://dice.fm/e/1"))
	assert.False(t, isAbsoluteHTTPURL("dice.fm/e/1"))
	assert.False(t, isAbsoluteHTTPURL("//dice.fm/e/1"))
	assert.False(t, isAbsoluteHTTPURL("https:///e/1"))
	assert.False(t, isAbsoluteHTTPURL("mailto:a@b.example"))
	assert.False(t, isAbsoluteHTTPURL(""))
	assert.False(t, isAbsoluteHTTPURL("https://%zz"))
}
