package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseICalFeed_ValidEvents(t *testing.T) {
	// Create a future date for testing
	futureDate := time.Now().AddDate(0, 1, 0).Format("20060102")

	icalData := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//Test//EN
BEGIN:VEVENT
UID:event-1@venue.com
SUMMARY:The Black Keys / The Stripes
DTSTART:` + futureDate + `T200000
LOCATION:Test Venue
URL:https://venue.com/events/1
END:VEVENT
BEGIN:VEVENT
UID:event-2@venue.com
SUMMARY:Jazz Night
DTSTART:` + futureDate + `T190000
END:VEVENT
END:VCALENDAR`

	parser := NewFeedParser()
	result, err := parser.ParseICalFeed([]byte(icalData), "Test Venue", "test-venue")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, FeedTypeICal, result.FeedType)
	assert.Equal(t, 2, result.RawCount)
	assert.Len(t, result.Events, 2)

	// Check first event
	assert.Equal(t, "event-1@venue.com", result.Events[0].ID)
	assert.Equal(t, "The Black Keys / The Stripes", result.Events[0].Title)
	assert.Equal(t, "Test Venue", result.Events[0].Venue)
	assert.Equal(t, "test-venue", result.Events[0].VenueSlug)
	assert.NotNil(t, result.Events[0].TicketURL)
	assert.Equal(t, "https://venue.com/events/1", *result.Events[0].TicketURL)

	// Artists should be extracted from title
	assert.Contains(t, result.Events[0].Artists, "The Black Keys")
	assert.Contains(t, result.Events[0].Artists, "The Stripes")
}

func TestParseICalFeed_SkipPastEvents(t *testing.T) {
	icalData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:past@venue.com
SUMMARY:Past Show
DTSTART:20200101T200000
END:VEVENT
END:VCALENDAR`

	parser := NewFeedParser()
	result, err := parser.ParseICalFeed([]byte(icalData), "Venue", "venue")
	require.NoError(t, err)
	assert.Len(t, result.Events, 0)
	assert.Equal(t, 1, result.RawCount)
}

func TestParseICalFeed_DateOnlyFormat(t *testing.T) {
	futureDate := time.Now().AddDate(0, 1, 0).Format("20060102")

	icalData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:allday@venue.com
SUMMARY:Festival Day
DTSTART:` + futureDate + `
END:VEVENT
END:VCALENDAR`

	parser := NewFeedParser()
	result, err := parser.ParseICalFeed([]byte(icalData), "Venue", "venue")
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	assert.Nil(t, result.Events[0].ShowTime) // No time for date-only events
}

func TestParseICalFeed_UTCFormat(t *testing.T) {
	futureDate := time.Now().AddDate(0, 1, 0).Format("20060102")

	icalData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:utc@venue.com
SUMMARY:UTC Event
DTSTART:` + futureDate + `T200000Z
END:VEVENT
END:VCALENDAR`

	parser := NewFeedParser()
	result, err := parser.ParseICalFeed([]byte(icalData), "Venue", "venue")
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
}

func TestParseICalFeed_Empty(t *testing.T) {
	icalData := `BEGIN:VCALENDAR
VERSION:2.0
END:VCALENDAR`

	parser := NewFeedParser()
	result, err := parser.ParseICalFeed([]byte(icalData), "Venue", "venue")
	require.NoError(t, err)
	assert.Len(t, result.Events, 0)
}

func TestParseICalFeed_Malformed(t *testing.T) {
	parser := NewFeedParser()
	_, err := parser.ParseICalFeed([]byte("not an ical file"), "Venue", "venue")
	assert.Error(t, err)
}

func TestParseICalFeed_SkipNoSummary(t *testing.T) {
	futureDate := time.Now().AddDate(0, 1, 0).Format("20060102")

	icalData := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:no-title@venue.com
DTSTART:` + futureDate + `T200000
END:VEVENT
END:VCALENDAR`

	parser := NewFeedParser()
	result, err := parser.ParseICalFeed([]byte(icalData), "Venue", "venue")
	require.NoError(t, err)
	assert.Len(t, result.Events, 0) // No summary = skipped
}

func TestParseRSSFeed_ValidItems(t *testing.T) {
	futureDate := time.Now().AddDate(0, 1, 0).Format(time.RFC1123Z)

	rssData := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Venue Events</title>
    <item>
      <title>Band A, Band B</title>
      <link>https://venue.com/event/1</link>
      <pubDate>` + futureDate + `</pubDate>
      <guid>event-1</guid>
    </item>
  </channel>
</rss>`

	parser := NewFeedParser()
	result, err := parser.ParseRSSFeed([]byte(rssData), "Test Venue", "test-venue")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, FeedTypeRSS, result.FeedType)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "event-1", result.Events[0].ID)
	assert.Equal(t, "Band A, Band B", result.Events[0].Title)
	assert.Equal(t, "Test Venue", result.Events[0].Venue)
	assert.NotNil(t, result.Events[0].TicketURL)
}

func TestParseRSSFeed_SkipPastItems(t *testing.T) {
	rssData := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Old Show</title>
      <pubDate>Mon, 01 Jan 2020 20:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

	parser := NewFeedParser()
	result, err := parser.ParseRSSFeed([]byte(rssData), "Venue", "venue")
	require.NoError(t, err)
	assert.Len(t, result.Events, 0)
}

func TestParseRSSFeed_Malformed(t *testing.T) {
	parser := NewFeedParser()
	_, err := parser.ParseRSSFeed([]byte("not rss"), "Venue", "venue")
	assert.Error(t, err)
}

func TestParseRSSFeed_Empty(t *testing.T) {
	rssData := `<?xml version="1.0"?>
<rss version="2.0"><channel><title>Empty</title></channel></rss>`

	parser := NewFeedParser()
	result, err := parser.ParseRSSFeed([]byte(rssData), "Venue", "venue")
	require.NoError(t, err)
	assert.Len(t, result.Events, 0)
}

func TestParseFeed_Routing(t *testing.T) {
	parser := NewFeedParser()

	t.Run("unsupported type", func(t *testing.T) {
		_, err := parser.ParseFeed([]byte("test"), "json", "Venue", "venue")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported feed type")
	})
}

func TestExtractArtistsFromTitle(t *testing.T) {
	tests := []struct {
		title    string
		expected []string
	}{
		{"Band A / Band B", []string{"Band A", "Band B"}},
		{"Band A, Band B", []string{"Band A", "Band B"}},
		{"Band A w/ Band B", []string{"Band A", "Band B"}},
		{"Band A with Band B", []string{"Band A", "Band B"}},
		{"Band A + Band B", []string{"Band A", "Band B"}},
		{"Solo Artist", []string{"Solo Artist"}},
		{"Band A (SOLD OUT)", []string{"Band A"}},
		{"Band A / Band B (FREE)", []string{"Band A", "Band B"}},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			result := extractArtistsFromTitle(tt.title)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseICalDate(t *testing.T) {
	tests := []struct {
		input    string
		wantErr  bool
		expected string
	}{
		{"20260115T200000Z", false, "2026-01-15"},
		{"20260115T200000", false, "2026-01-15"},
		{"20260115", false, "2026-01-15"},
		{"invalid", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseICalDate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result.Format("2006-01-02"))
			}
		})
	}
}

func TestIsDateOnly(t *testing.T) {
	assert.True(t, isDateOnly("20260115"))
	assert.False(t, isDateOnly("20260115T200000"))
	assert.False(t, isDateOnly("20260115T200000Z"))
}
