package pipeline

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	ical "github.com/arran4/golang-ical"
	"github.com/mmcdole/gofeed"

	"psychic-homily-backend/internal/services/contracts"
)

// FeedParser parses iCal and RSS feeds into DiscoveredEvent structs.
type FeedParser struct{}

// NewFeedParser creates a new FeedParser.
func NewFeedParser() *FeedParser {
	return &FeedParser{}
}

// ParsedFeedResult contains the result of parsing a feed.
type ParsedFeedResult struct {
	Events   []contracts.DiscoveredEvent `json:"events"`
	FeedType string                      `json:"feed_type"`
	FeedURL  string                      `json:"feed_url"`
	ParsedAt time.Time                   `json:"parsed_at"`
	RawCount int                         `json:"raw_count"`
}

// ParseFeed parses a feed based on its type.
func (p *FeedParser) ParseFeed(feedBody []byte, feedType string, venueName string, venueSlug string) (*ParsedFeedResult, error) {
	switch feedType {
	case FeedTypeICal:
		return p.ParseICalFeed(feedBody, venueName, venueSlug)
	case FeedTypeRSS:
		return p.ParseRSSFeed(feedBody, venueName, venueSlug)
	default:
		return nil, fmt.Errorf("unsupported feed type: %s", feedType)
	}
}

// ParseICalFeed parses an iCal feed into DiscoveredEvents.
func (p *FeedParser) ParseICalFeed(feedBody []byte, venueName string, venueSlug string) (*ParsedFeedResult, error) {
	cal, err := ical.ParseCalendar(strings.NewReader(string(feedBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse iCal: %w", err)
	}

	now := time.Now()
	var events []contracts.DiscoveredEvent
	rawCount := 0

	for _, component := range cal.Events() {
		rawCount++

		summary := component.GetProperty(ical.ComponentPropertySummary)
		if summary == nil || summary.Value == "" {
			continue
		}

		dtStart := component.GetProperty(ical.ComponentPropertyDtStart)
		if dtStart == nil || dtStart.Value == "" {
			continue
		}

		// Parse date
		eventDate, err := parseICalDate(dtStart.Value)
		if err != nil {
			continue
		}

		// Skip past events
		if eventDate.Before(now.Truncate(24 * time.Hour)) {
			continue
		}

		title := summary.Value

		// Extract optional fields
		var description, location, eventURL *string
		if desc := component.GetProperty(ical.ComponentPropertyDescription); desc != nil && desc.Value != "" {
			description = &desc.Value
		}
		if loc := component.GetProperty(ical.ComponentPropertyLocation); loc != nil && loc.Value != "" {
			location = &loc.Value
		}
		if u := component.GetProperty(ical.ComponentPropertyUrl); u != nil && u.Value != "" {
			eventURL = &u.Value
		}

		// Extract time if available
		var showTime *string
		if !isDateOnly(dtStart.Value) {
			t := eventDate.Format("3:04 pm")
			showTime = &t
		}

		// Use UID as event ID
		eventID := ""
		if uid := component.GetProperty(ical.ComponentPropertyUniqueId); uid != nil {
			eventID = uid.Value
		}

		event := contracts.DiscoveredEvent{
			ID:        eventID,
			Title:     title,
			Date:      eventDate.Format("2006-01-02"),
			Venue:     venueName,
			VenueSlug: venueSlug,
			ShowTime:  showTime,
			TicketURL: eventURL,
			Artists:   extractArtistsFromTitle(title),
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		}

		// Use location if venue name is empty
		if location != nil && venueName == "" {
			event.Venue = *location
		}

		_ = description // Available for future enrichment

		events = append(events, event)
	}

	return &ParsedFeedResult{
		Events:   events,
		FeedType: FeedTypeICal,
		ParsedAt: time.Now(),
		RawCount: rawCount,
	}, nil
}

// ParseRSSFeed parses an RSS/Atom feed into DiscoveredEvents.
func (p *FeedParser) ParseRSSFeed(feedBody []byte, venueName string, venueSlug string) (*ParsedFeedResult, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseString(string(feedBody))
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	now := time.Now()
	var events []contracts.DiscoveredEvent

	for _, item := range feed.Items {
		if item.Title == "" {
			continue
		}

		// Try to get a date from the item
		var eventDate time.Time
		if item.PublishedParsed != nil {
			eventDate = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			eventDate = *item.UpdatedParsed
		} else {
			continue
		}

		// Skip past events
		if eventDate.Before(now.Truncate(24 * time.Hour)) {
			continue
		}

		var ticketURL *string
		if item.Link != "" {
			ticketURL = &item.Link
		}

		event := contracts.DiscoveredEvent{
			ID:        item.GUID,
			Title:     item.Title,
			Date:      eventDate.Format("2006-01-02"),
			Venue:     venueName,
			VenueSlug: venueSlug,
			TicketURL: ticketURL,
			Artists:   extractArtistsFromTitle(item.Title),
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		}

		events = append(events, event)
	}

	return &ParsedFeedResult{
		Events:   events,
		FeedType: FeedTypeRSS,
		ParsedAt: time.Now(),
		RawCount: len(feed.Items),
	}, nil
}

// parseICalDate parses an iCal date/datetime string.
func parseICalDate(value string) (time.Time, error) {
	// Try common iCal datetime formats
	formats := []string{
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, value); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse iCal date: %s", value)
}

// isDateOnly returns true if the iCal date string has no time component.
func isDateOnly(value string) bool {
	return len(value) == 8 // "20060102"
}

// extractArtistsFromTitle attempts to split a title into artist names.
// Common patterns: "Artist1, Artist2", "Artist1 / Artist2", "Artist1 w/ Artist2".
func extractArtistsFromTitle(title string) []string {
	// Remove common suffixes like "(SOLD OUT)", "(FREE)", etc.
	cleaned := suffixRe.ReplaceAllString(title, "")
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return []string{title}
	}

	// Try splitting by common delimiters
	var artists []string
	for _, sep := range []string{" / ", " | ", ", ", " w/ ", " with ", " & ", " + "} {
		if strings.Contains(cleaned, sep) {
			parts := strings.Split(cleaned, sep)
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					artists = append(artists, p)
				}
			}
			return artists
		}
	}

	return []string{cleaned}
}

// Compile regex once
var suffixRe = regexp.MustCompile(`\s*\([^)]*\)\s*$`)
