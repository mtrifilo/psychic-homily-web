package pipeline

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

const calendarExtractionSystemPrompt = `You are a venue calendar event extractor. Given text or an image of a venue's event calendar page, extract ALL upcoming events into structured JSON.

The venue name and location will be provided as context — do not try to extract venue info.

Output ONLY a valid JSON array with no additional text, markdown formatting, or explanation:
[
  {
    "date": "YYYY-MM-DD",
    "time": "HH:MM",
    "title": "Event Title as Shown on Calendar",
    "artists": [
      {"name": "Artist Name", "set_type": "headliner", "billing_order": 1},
      {"name": "Supporting Act", "set_type": "support", "billing_order": 2}
    ],
    "cost": "$20",
    "ages": "21+",
    "ticket_url": "https://...",
    "is_music_event": true
  }
]

Rules:
- Extract EVERY event visible on the page — do not skip any
- Set is_music_event to false for non-music events like karaoke nights, trivia, comedy shows, open mic (non-music), DJ nights without named artists, private events, and venue closures. Set to true for concerts, live music, album release shows, and music festivals. Default to true if uncertain
- Convert dates to YYYY-MM-DD format. If only a month/year header is shown, combine with day numbers
- Convert times to 24-hour HH:MM format. If "doors" and "show" times are both listed, use the show time. Default to 20:00 if only doors time is given
- Determine billing position from visual prominence, text size, and ordering. The first/largest name is typically the headliner
- set_type values: "headliner" (top of bill), "support" (direct support act, indicated by "w/" or "with"), "opener" (opening act), "special_guest" (indicated by "special guest" or "featuring"), "dj" (DJ set), "host" (event host/MC)
- billing_order: 1 = top of bill, 2 = second, etc. Assign based on prominence/position
- If the event title IS the artist name (common for concerts), put the same name in both title and artists array
- For multi-artist events (e.g., "Band A with Band B" or "Band A / Band B"), split into separate artist entries. The artist before "with"/"w/" is typically the headliner, the artist after is support
- For cost, include dollar sign for paid shows, use "Free" for free events. Omit if not shown
- For ages, common formats are "21+", "18+", "All Ages". Omit if not shown
- Include ticket_url only if a direct link to purchase is visible. Omit if not shown
- Omit fields that are not present (do not include null values)
- If events that are clearly marked as past (e.g., greyed out, marked "PAST"), skip them
- Handle various calendar layouts: chronological lists, card grids, weekly/monthly calendar views
- Return ONLY the JSON array, no explanation or markdown code blocks
- If no events are found, return an empty array: []`

// ExtractCalendarPage processes a venue calendar page (text or image) through Claude
// and returns structured event data. Optional extractionNotes provide per-venue hints
// to improve extraction quality (e.g., "skip karaoke Tuesdays").
func (s *ExtractionService) ExtractCalendarPage(venueName string, content string, contentType string, extractionNotes ...string) (*contracts.CalendarExtractionResponse, error) {
	if s.config.Anthropic.APIKey == "" {
		return &contracts.CalendarExtractionResponse{
			Success: false,
			Error:   "AI service not configured",
		}, nil
	}

	if strings.TrimSpace(content) == "" {
		return &contracts.CalendarExtractionResponse{
			Success: false,
			Error:   "Content is required",
		}, nil
	}

	// Build user content based on content type
	var userContent []interface{}
	contextMsg := fmt.Sprintf("Extract all events from this venue calendar page. Venue: %s", venueName)
	if len(extractionNotes) > 0 && extractionNotes[0] != "" {
		contextMsg += fmt.Sprintf("\n\nVenue-specific notes: %s", extractionNotes[0])
	}

	switch contentType {
	case "text":
		if len(content) > 100000 {
			return &contracts.CalendarExtractionResponse{
				Success: false,
				Error:   "Content exceeds maximum length of 100,000 characters",
			}, nil
		}
		userContent = []interface{}{
			map[string]string{
				"type": "text",
				"text": fmt.Sprintf("%s\n\nPage content:\n%s", contextMsg, content),
			},
		}
	case "image":
		userContent = []interface{}{
			map[string]interface{}{
				"type": "image",
				"source": map[string]string{
					"type":       "base64",
					"media_type": "image/png",
					"data":       content,
				},
			},
			map[string]string{
				"type": "text",
				"text": contextMsg,
			},
		}
	default:
		return &contracts.CalendarExtractionResponse{
			Success: false,
			Error:   "Invalid content type. Use \"text\" or \"image\"",
		}, nil
	}

	// Call Claude with the calendar-specific system prompt and higher token limit
	responseText, err := s.callAnthropicWithOptions(userContent, calendarExtractionSystemPrompt, 4096)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "credit") || strings.Contains(strings.ToLower(err.Error()), "billing") {
			return &contracts.CalendarExtractionResponse{Success: false, Error: "AI service temporarily unavailable. Please try again later."}, nil
		}
		return &contracts.CalendarExtractionResponse{Success: false, Error: "AI service error. Please try again."}, nil
	}

	// Parse the JSON array response
	events := parseCalendarExtractionResponse(responseText)
	if events == nil {
		return &contracts.CalendarExtractionResponse{
			Success:  false,
			Error:    "Failed to parse AI response",
			Warnings: []string{"The AI response could not be parsed as a JSON array. Please try again."},
		}, nil
	}

	warnings := []string{}
	if len(events) == 0 {
		warnings = append(warnings, "No events were found on the calendar page")
	}

	// Count events with/without dates for warnings
	missingDates := 0
	missingArtists := 0
	for _, event := range events {
		if event.Date == "" {
			missingDates++
		}
		if len(event.Artists) == 0 {
			missingArtists++
		}
	}
	if missingDates > 0 {
		warnings = append(warnings, fmt.Sprintf("%d event(s) missing date information", missingDates))
	}
	if missingArtists > 0 {
		warnings = append(warnings, fmt.Sprintf("%d event(s) have no artist information", missingArtists))
	}

	resp := &contracts.CalendarExtractionResponse{
		Success: true,
		Events:  events,
	}
	if len(warnings) > 0 {
		resp.Warnings = warnings
	}

	return resp, nil
}

// callAnthropicWithOptions sends a request to the Anthropic API with custom system prompt and max tokens.
func (s *ExtractionService) callAnthropicWithOptions(userContent []interface{}, systemPrompt string, maxTokens int) (string, error) {
	reqBody := anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: userContent,
			},
		},
	}

	return s.sendAnthropicRequest(reqBody)
}

// parseCalendarExtractionResponse tries to parse Claude's response as a JSON array of CalendarEvents.
func parseCalendarExtractionResponse(text string) []contracts.CalendarEvent {
	text = strings.TrimSpace(text)

	// Try direct JSON array parse
	var events []contracts.CalendarEvent
	if err := json.Unmarshal([]byte(text), &events); err == nil {
		return events
	}

	// Try to extract JSON from markdown code block
	codeBlockRe := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")
	if matches := codeBlockRe.FindStringSubmatch(text); len(matches) > 1 {
		if err := json.Unmarshal([]byte(strings.TrimSpace(matches[1])), &events); err == nil {
			return events
		}
	}

	// Try to find a JSON array in the response
	arrayRe := regexp.MustCompile(`(?s)\[.*\]`)
	if match := arrayRe.FindString(text); match != "" {
		if err := json.Unmarshal([]byte(match), &events); err == nil {
			return events
		}
	}

	return nil
}

// CalendarEventsToDiscoveredEvents converts calendar extraction results to DiscoveredEvent format
// for import into the discovery pipeline.
func CalendarEventsToDiscoveredEvents(venueSlug string, events []contracts.CalendarEvent) []contracts.DiscoveredEvent {
	var discovered []contracts.DiscoveredEvent
	now := time.Now().UTC().Format(time.RFC3339)

	for _, event := range events {
		// Generate stable source_event_id from hash of venue slug + date + first artist name
		hashInput := venueSlug + "|" + event.Date
		if len(event.Artists) > 0 {
			hashInput += "|" + strings.ToLower(event.Artists[0].Name)
		}
		hash := sha256.Sum256([]byte(hashInput))
		sourceEventID := fmt.Sprintf("cal-%x", hash[:8])

		// Map artists to string array and billing artists
		var artists []string
		var billingArtists []contracts.DiscoveredArtist
		hasBillingInfo := false
		for _, a := range event.Artists {
			artists = append(artists, a.Name)
			da := contracts.DiscoveredArtist{
				Name:         a.Name,
				SetType:      a.SetType,
				BillingOrder: a.BillingOrder,
			}
			if a.SetType != "" || a.BillingOrder > 0 {
				hasBillingInfo = true
			}
			billingArtists = append(billingArtists, da)
		}

		de := contracts.DiscoveredEvent{
			ID:        sourceEventID,
			Title:     event.Title,
			Date:      event.Date,
			VenueSlug: venueSlug,
			Artists:   artists,
			ScrapedAt: now,
		}
		// Only include billing artists if any have non-default billing info
		if hasBillingInfo {
			de.BillingArtists = billingArtists
		}

		if event.Time != nil {
			de.ShowTime = event.Time
		}
		if event.Cost != nil {
			de.Price = event.Cost
		}
		if event.Ages != nil {
			de.AgeRestriction = event.Ages
		}
		if event.TicketURL != nil {
			de.TicketURL = event.TicketURL
		}

		discovered = append(discovered, de)
	}

	return discovered
}
