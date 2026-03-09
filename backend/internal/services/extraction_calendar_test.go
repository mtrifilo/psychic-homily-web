package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
)

// =============================================================================
// parseCalendarExtractionResponse TESTS
// =============================================================================

func TestParseCalendarExtractionResponse(t *testing.T) {
	t.Run("valid_json_array", func(t *testing.T) {
		input := `[
			{
				"date": "2026-03-15",
				"time": "20:00",
				"title": "Dinosaur Jr with Guided By Voices",
				"artists": [
					{"name": "Dinosaur Jr", "is_headliner": true},
					{"name": "Guided By Voices", "is_headliner": false}
				],
				"cost": "$35",
				"ages": "All Ages",
				"ticket_url": "https://tickets.example.com/123"
			},
			{
				"date": "2026-03-20",
				"title": "Built to Spill",
				"artists": [
					{"name": "Built to Spill", "is_headliner": true}
				]
			}
		]`

		events := parseCalendarExtractionResponse(input)

		require.NotNil(t, events)
		require.Len(t, events, 2)

		// First event: full fields
		assert.Equal(t, "2026-03-15", events[0].Date)
		require.NotNil(t, events[0].Time)
		assert.Equal(t, "20:00", *events[0].Time)
		assert.Equal(t, "Dinosaur Jr with Guided By Voices", events[0].Title)
		require.Len(t, events[0].Artists, 2)
		assert.Equal(t, "Dinosaur Jr", events[0].Artists[0].Name)
		assert.True(t, events[0].Artists[0].IsHeadliner)
		assert.Equal(t, "Guided By Voices", events[0].Artists[1].Name)
		assert.False(t, events[0].Artists[1].IsHeadliner)
		require.NotNil(t, events[0].Cost)
		assert.Equal(t, "$35", *events[0].Cost)
		require.NotNil(t, events[0].Ages)
		assert.Equal(t, "All Ages", *events[0].Ages)
		require.NotNil(t, events[0].TicketURL)
		assert.Equal(t, "https://tickets.example.com/123", *events[0].TicketURL)

		// Second event: minimal fields (optional fields omitted)
		assert.Equal(t, "2026-03-20", events[1].Date)
		assert.Nil(t, events[1].Time)
		assert.Nil(t, events[1].Cost)
		assert.Nil(t, events[1].Ages)
		assert.Nil(t, events[1].TicketURL)
		require.Len(t, events[1].Artists, 1)
		assert.Equal(t, "Built to Spill", events[1].Artists[0].Name)
		assert.True(t, events[1].Artists[0].IsHeadliner)
	})

	t.Run("markdown_wrapped_json", func(t *testing.T) {
		input := "Here are the events:\n\n```json\n[{\"date\":\"2026-04-01\",\"title\":\"Test Show\",\"artists\":[{\"name\":\"Test Artist\",\"is_headliner\":true}]}]\n```\n\nI found 1 event."

		events := parseCalendarExtractionResponse(input)

		require.NotNil(t, events)
		require.Len(t, events, 1)
		assert.Equal(t, "2026-04-01", events[0].Date)
		assert.Equal(t, "Test Show", events[0].Title)
	})

	t.Run("markdown_no_lang_tag", func(t *testing.T) {
		input := "```\n[{\"date\":\"2026-05-01\",\"title\":\"No Lang\",\"artists\":[]}]\n```"

		events := parseCalendarExtractionResponse(input)

		require.NotNil(t, events)
		require.Len(t, events, 1)
		assert.Equal(t, "No Lang", events[0].Title)
	})

	t.Run("array_embedded_in_text", func(t *testing.T) {
		input := "I extracted the following events: [{\"date\":\"2026-06-01\",\"title\":\"Embedded\",\"artists\":[{\"name\":\"Band\",\"is_headliner\":true}]}] from the calendar."

		events := parseCalendarExtractionResponse(input)

		require.NotNil(t, events)
		require.Len(t, events, 1)
		assert.Equal(t, "Embedded", events[0].Title)
	})

	t.Run("empty_array", func(t *testing.T) {
		input := "[]"

		events := parseCalendarExtractionResponse(input)

		require.NotNil(t, events)
		assert.Len(t, events, 0)
	})

	t.Run("invalid_input_returns_nil", func(t *testing.T) {
		input := "I couldn't find any events on this page."

		events := parseCalendarExtractionResponse(input)

		assert.Nil(t, events)
	})

	t.Run("empty_string_returns_nil", func(t *testing.T) {
		events := parseCalendarExtractionResponse("")

		assert.Nil(t, events)
	})

	t.Run("json_object_not_array_returns_nil", func(t *testing.T) {
		// A JSON object with no embedded array should return nil
		input := `{"message": "no events found", "status": "ok"}`

		events := parseCalendarExtractionResponse(input)

		assert.Nil(t, events)
	})

	t.Run("malformed_json_returns_nil", func(t *testing.T) {
		input := `[{"date": "2026-01-01", "title":}]`

		events := parseCalendarExtractionResponse(input)

		assert.Nil(t, events)
	})
}

// =============================================================================
// CalendarEventsToDiscoveredEvents TESTS
// =============================================================================

func TestCalendarEventsToDiscoveredEvents(t *testing.T) {
	t.Run("basic_mapping", func(t *testing.T) {
		showTime := "20:00"
		cost := "$25"
		ages := "21+"
		ticketURL := "https://tickets.example.com/abc"

		events := []CalendarEvent{
			{
				Date:      "2026-03-15",
				Time:      &showTime,
				Title:     "Dinosaur Jr with Guided By Voices",
				Artists:   []CalendarArtist{{Name: "Dinosaur Jr", IsHeadliner: true}, {Name: "Guided By Voices", IsHeadliner: false}},
				Cost:      &cost,
				Ages:      &ages,
				TicketURL: &ticketURL,
			},
		}

		discovered := CalendarEventsToDiscoveredEvents("crescent-ballroom", events)

		require.Len(t, discovered, 1)
		de := discovered[0]
		assert.Equal(t, "2026-03-15", de.Date)
		assert.Equal(t, "crescent-ballroom", de.VenueSlug)
		assert.Equal(t, "Dinosaur Jr with Guided By Voices", de.Title)
		require.Len(t, de.Artists, 2)
		assert.Equal(t, "Dinosaur Jr", de.Artists[0])
		assert.Equal(t, "Guided By Voices", de.Artists[1])
		require.NotNil(t, de.ShowTime)
		assert.Equal(t, "20:00", *de.ShowTime)
		require.NotNil(t, de.Price)
		assert.Equal(t, "$25", *de.Price)
		require.NotNil(t, de.AgeRestriction)
		assert.Equal(t, "21+", *de.AgeRestriction)
		require.NotNil(t, de.TicketURL)
		assert.Equal(t, "https://tickets.example.com/abc", *de.TicketURL)
		assert.NotEmpty(t, de.ID)
		assert.NotEmpty(t, de.ScrapedAt)
		assert.Contains(t, de.ID, "cal-") // source_event_id prefix
	})

	t.Run("minimal_event_no_optional_fields", func(t *testing.T) {
		events := []CalendarEvent{
			{
				Date:    "2026-04-01",
				Title:   "Solo Show",
				Artists: []CalendarArtist{{Name: "Solo Artist", IsHeadliner: true}},
			},
		}

		discovered := CalendarEventsToDiscoveredEvents("valley-bar", events)

		require.Len(t, discovered, 1)
		de := discovered[0]
		assert.Equal(t, "2026-04-01", de.Date)
		assert.Equal(t, "valley-bar", de.VenueSlug)
		assert.Nil(t, de.ShowTime)
		assert.Nil(t, de.Price)
		assert.Nil(t, de.AgeRestriction)
		assert.Nil(t, de.TicketURL)
	})

	t.Run("empty_events_returns_nil", func(t *testing.T) {
		discovered := CalendarEventsToDiscoveredEvents("venue", []CalendarEvent{})
		assert.Nil(t, discovered)
	})

	t.Run("nil_events_returns_nil", func(t *testing.T) {
		discovered := CalendarEventsToDiscoveredEvents("venue", nil)
		assert.Nil(t, discovered)
	})

	t.Run("multiple_events_produces_multiple_discovered", func(t *testing.T) {
		events := []CalendarEvent{
			{Date: "2026-03-10", Title: "Show A", Artists: []CalendarArtist{{Name: "Band A", IsHeadliner: true}}},
			{Date: "2026-03-11", Title: "Show B", Artists: []CalendarArtist{{Name: "Band B", IsHeadliner: true}}},
			{Date: "2026-03-12", Title: "Show C", Artists: []CalendarArtist{{Name: "Band C", IsHeadliner: true}}},
		}

		discovered := CalendarEventsToDiscoveredEvents("rebel-lounge", events)

		require.Len(t, discovered, 3)
		for i, de := range discovered {
			assert.Equal(t, "rebel-lounge", de.VenueSlug)
			assert.NotEmpty(t, de.ID)
			// Each should have a unique ID
			for j, other := range discovered {
				if i != j {
					assert.NotEqual(t, de.ID, other.ID, "event IDs should be unique")
				}
			}
		}
	})

	t.Run("stable_source_event_id", func(t *testing.T) {
		events := []CalendarEvent{
			{Date: "2026-03-15", Title: "Test Show", Artists: []CalendarArtist{{Name: "Test Band", IsHeadliner: true}}},
		}

		// Run twice — should produce identical IDs
		d1 := CalendarEventsToDiscoveredEvents("venue-slug", events)
		d2 := CalendarEventsToDiscoveredEvents("venue-slug", events)

		require.Len(t, d1, 1)
		require.Len(t, d2, 1)
		assert.Equal(t, d1[0].ID, d2[0].ID, "source_event_id should be stable/deterministic")
	})

	t.Run("different_venue_produces_different_id", func(t *testing.T) {
		events := []CalendarEvent{
			{Date: "2026-03-15", Title: "Test Show", Artists: []CalendarArtist{{Name: "Test Band", IsHeadliner: true}}},
		}

		d1 := CalendarEventsToDiscoveredEvents("venue-a", events)
		d2 := CalendarEventsToDiscoveredEvents("venue-b", events)

		assert.NotEqual(t, d1[0].ID, d2[0].ID, "different venues should produce different IDs")
	})

	t.Run("event_without_artists_still_hashes", func(t *testing.T) {
		events := []CalendarEvent{
			{Date: "2026-03-15", Title: "Private Event", Artists: nil},
		}

		discovered := CalendarEventsToDiscoveredEvents("venue", events)

		require.Len(t, discovered, 1)
		assert.NotEmpty(t, discovered[0].ID)
		assert.Contains(t, discovered[0].ID, "cal-")
	})
}

// =============================================================================
// ExtractCalendarPage TESTS
// =============================================================================

func TestExtractCalendarPage(t *testing.T) {
	t.Run("missing_api_key_returns_error", func(t *testing.T) {
		svc := &ExtractionService{
			config: &config.Config{},
		}

		resp, err := svc.ExtractCalendarPage("Test Venue, Phoenix, AZ", "some content", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "AI service not configured", resp.Error)
	})

	t.Run("empty_content_returns_error", func(t *testing.T) {
		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Content is required", resp.Error)
	})

	t.Run("whitespace_only_content_returns_error", func(t *testing.T) {
		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "   \n\t  ", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Content is required", resp.Error)
	})

	t.Run("invalid_content_type_returns_error", func(t *testing.T) {
		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "content", "video")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid content type")
	})

	t.Run("text_too_long_returns_error", func(t *testing.T) {
		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
		}

		longContent := make([]byte, 100001)
		for i := range longContent {
			longContent[i] = 'a'
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", string(longContent), "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "exceeds maximum length")
	})

	t.Run("successful_text_extraction", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify the request is well-formed
			body, _ := io.ReadAll(r.Body)
			var reqBody anthropicRequest
			json.Unmarshal(body, &reqBody)

			// Verify calendar-specific settings
			assert.Equal(t, 4096, reqBody.MaxTokens)
			assert.Equal(t, calendarExtractionSystemPrompt, reqBody.System)
			assert.Equal(t, "claude-haiku-4-5-20251001", reqBody.Model)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{
						Type: "text",
						Text: `[{"date":"2026-03-15","time":"20:00","title":"Dinosaur Jr","artists":[{"name":"Dinosaur Jr","is_headliner":true}],"cost":"$30","ages":"All Ages"}]`,
					},
				},
			})
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		resp, err := svc.ExtractCalendarPage("Crescent Ballroom, Phoenix, AZ", "<html>calendar page</html>", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		require.Len(t, resp.Events, 1)
		assert.Equal(t, "2026-03-15", resp.Events[0].Date)
		assert.Equal(t, "Dinosaur Jr", resp.Events[0].Title)
	})

	t.Run("successful_image_extraction", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var reqBody anthropicRequest
			json.Unmarshal(body, &reqBody)

			// Verify message content includes image block
			assert.Len(t, reqBody.Messages, 1)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{
						Type: "text",
						Text: `[{"date":"2026-04-01","title":"Image Show","artists":[{"name":"Image Band","is_headliner":true}]}]`,
					},
				},
			})
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		resp, err := svc.ExtractCalendarPage("Valley Bar, Phoenix, AZ", "base64screenshotdata", "image")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		require.Len(t, resp.Events, 1)
		assert.Equal(t, "Image Show", resp.Events[0].Title)
	})

	t.Run("api_returns_empty_array", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{Type: "text", Text: `[]`},
				},
			})
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "empty calendar", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Len(t, resp.Events, 0)
		require.NotNil(t, resp.Warnings)
		assert.Contains(t, resp.Warnings[0], "No events were found")
	})

	t.Run("api_returns_unparseable_response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{Type: "text", Text: "Sorry, I can't parse this page."},
				},
			})
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "garbled content", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Failed to parse AI response", resp.Error)
	})

	t.Run("api_error_returns_friendly_message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "content", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "AI service error. Please try again.", resp.Error)
	})

	t.Run("billing_error_returns_friendly_message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusPaymentRequired)
			w.Write([]byte("Your credit balance is too low"))
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "content", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "temporarily unavailable")
	})

	t.Run("events_with_missing_dates_produces_warning", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{
						Type: "text",
						Text: `[{"date":"2026-03-15","title":"Has Date","artists":[{"name":"A","is_headliner":true}]},{"date":"","title":"No Date","artists":[{"name":"B","is_headliner":true}]}]`,
					},
				},
			})
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "content", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Len(t, resp.Events, 2)
		require.NotNil(t, resp.Warnings)
		found := false
		for _, w := range resp.Warnings {
			if w == "1 event(s) missing date information" {
				found = true
			}
		}
		assert.True(t, found, "should warn about missing dates")
	})

	t.Run("events_with_missing_artists_produces_warning", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{
						Type: "text",
						Text: `[{"date":"2026-03-15","title":"No Artists","artists":[]}]`,
					},
				},
			})
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		resp, err := svc.ExtractCalendarPage("Test Venue", "content", "text")

		assert.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		require.NotNil(t, resp.Warnings)
		found := false
		for _, w := range resp.Warnings {
			if w == "1 event(s) have no artist information" {
				found = true
			}
		}
		assert.True(t, found, "should warn about missing artists")
	})

	t.Run("venue_name_included_in_user_content", func(t *testing.T) {
		var capturedBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{Type: "text", Text: `[]`},
				},
			})
		}))
		defer server.Close()

		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       server.Client(),
			anthropicBaseURL: server.URL,
		}

		_, err := svc.ExtractCalendarPage("The Rebel Lounge, Phoenix, AZ", "page content", "text")
		assert.NoError(t, err)

		// Verify the venue name appears in the request
		assert.Contains(t, string(capturedBody), "The Rebel Lounge, Phoenix, AZ")
	})
}
