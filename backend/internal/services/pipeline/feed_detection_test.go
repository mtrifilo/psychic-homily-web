package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectFeeds_LinkTagICal(t *testing.T) {
	html := `<html><head>
		<link rel="alternate" type="text/calendar" href="/events.ics" title="Events Calendar">
	</head><body></body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://example.com/events", html)
	require.NoError(t, err)
	require.Len(t, feeds, 1)

	assert.Equal(t, "https://example.com/events.ics", feeds[0].URL)
	assert.Equal(t, FeedTypeICal, feeds[0].FeedType)
	assert.Equal(t, FeedSourceLinkTag, feeds[0].Source)
	assert.Equal(t, 0.95, feeds[0].Confidence)
}

func TestDetectFeeds_LinkTagRSS(t *testing.T) {
	html := `<html><head>
		<link rel="alternate" type="application/rss+xml" href="/feed.xml" title="Events RSS">
	</head><body></body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://example.com/", html)
	require.NoError(t, err)
	require.Len(t, feeds, 1)

	assert.Equal(t, "https://example.com/feed.xml", feeds[0].URL)
	assert.Equal(t, FeedTypeRSS, feeds[0].FeedType)
	assert.Equal(t, FeedSourceLinkTag, feeds[0].Source)
}

func TestDetectFeeds_AnchorICS(t *testing.T) {
	html := `<html><body>
		<a href="/calendar/events.ics">Download Calendar</a>
	</body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://venue.com/events", html)
	require.NoError(t, err)
	require.Len(t, feeds, 1)

	assert.Equal(t, "https://venue.com/calendar/events.ics", feeds[0].URL)
	assert.Equal(t, FeedTypeICal, feeds[0].FeedType)
	assert.Equal(t, FeedSourceAnchor, feeds[0].Source)
	assert.Equal(t, 0.8, feeds[0].Confidence)
}

func TestDetectFeeds_AnchorWebcal(t *testing.T) {
	html := `<html><body>
		<a href="webcal://venue.com/calendar">Subscribe</a>
	</body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://venue.com/events", html)
	require.NoError(t, err)
	require.Len(t, feeds, 1)

	assert.Equal(t, "https://venue.com/calendar", feeds[0].URL)
	assert.Equal(t, FeedTypeICal, feeds[0].FeedType)
	assert.Equal(t, 0.85, feeds[0].Confidence)
}

func TestDetectFeeds_AnchorKeywordICal(t *testing.T) {
	html := `<html><body>
		<a href="/cal-export">subscribe</a>
	</body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://venue.com/", html)
	require.NoError(t, err)
	require.Len(t, feeds, 1)

	assert.Equal(t, FeedTypeICal, feeds[0].FeedType)
	assert.Equal(t, 0.6, feeds[0].Confidence)
}

func TestDetectFeeds_AnchorKeywordRSS(t *testing.T) {
	html := `<html><body>
		<a href="/news-feed">RSS Feed</a>
	</body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://venue.com/", html)
	require.NoError(t, err)
	require.Len(t, feeds, 1)

	assert.Equal(t, FeedTypeRSS, feeds[0].FeedType)
}

func TestDetectFeeds_NoFeeds(t *testing.T) {
	html := `<html><body><h1>No feeds here</h1></body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://example.com/", html)
	require.NoError(t, err)
	assert.Len(t, feeds, 0)
}

func TestDetectFeeds_MultipleFeeds(t *testing.T) {
	html := `<html><head>
		<link rel="alternate" type="text/calendar" href="/events.ics">
		<link rel="alternate" type="application/rss+xml" href="/feed.xml">
	</head><body></body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://venue.com/", html)
	require.NoError(t, err)
	assert.Len(t, feeds, 2)
}

func TestDetectFeeds_Deduplication(t *testing.T) {
	html := `<html><head>
		<link rel="alternate" type="text/calendar" href="/events.ics">
	</head><body>
		<a href="/events.ics">Calendar</a>
	</body></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://venue.com/", html)
	require.NoError(t, err)
	assert.Len(t, feeds, 1) // Deduped
}

func TestDetectFeeds_AbsoluteURL(t *testing.T) {
	html := `<html><head>
		<link rel="alternate" type="text/calendar" href="https://other.com/cal.ics">
	</head></html>`

	d := NewFeedDetector()
	feeds, err := d.DetectFeeds("https://venue.com/events", html)
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Equal(t, "https://other.com/cal.ics", feeds[0].URL)
}

func TestDetectFeeds_InvalidCalendarURL(t *testing.T) {
	d := NewFeedDetector()
	_, err := d.DetectFeeds("://invalid", `<html></html>`)
	assert.Error(t, err)
}

func TestExtractAttr(t *testing.T) {
	tests := []struct {
		tag, attr, expected string
	}{
		{`<link rel="alternate" type="text/calendar" href="/events.ics">`, "href", "/events.ics"},
		{`<link rel='alternate' type='text/calendar' href='/events.ics'>`, "href", "/events.ics"},
		{`<link rel="alternate">`, "href", ""},
		{`<link type="TEXT/CALENDAR">`, "type", "TEXT/CALENDAR"},
	}

	for _, tt := range tests {
		result := extractAttr(tt.tag, tt.attr)
		assert.Equal(t, tt.expected, result)
	}
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name, base, href, expected string
	}{
		{"relative", "https://example.com/events", "/cal.ics", "https://example.com/cal.ics"},
		{"absolute", "https://example.com/events", "https://other.com/cal.ics", "https://other.com/cal.ics"},
		{"webcal", "https://example.com/events", "webcal://example.com/cal.ics", "https://example.com/cal.ics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use DetectFeeds with a link tag to test URL resolution
			html := `<link rel="alternate" type="text/calendar" href="` + tt.href + `">`
			d := NewFeedDetector()
			feeds, err := d.DetectFeeds(tt.base, html)
			require.NoError(t, err)
			if len(feeds) > 0 {
				assert.Equal(t, tt.expected, feeds[0].URL)
			}
		})
	}
}
