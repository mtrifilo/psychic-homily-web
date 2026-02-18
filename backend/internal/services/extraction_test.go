package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// =============================================================================
// NewExtractionService TESTS
// =============================================================================

func TestNewExtractionService(t *testing.T) {
	cfg := &config.Config{
		Anthropic: config.AnthropicConfig{APIKey: "test-key"},
	}
	svc := NewExtractionService(nil, cfg)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.config)
	assert.NotNil(t, svc.artistService)
	assert.NotNil(t, svc.venueService)
	assert.NotNil(t, svc.httpClient)
	assert.Equal(t, "https://api.anthropic.com", svc.anthropicBaseURL)
}

// =============================================================================
// parseExtractionResponse TESTS
// =============================================================================

func TestParseExtractionResponse(t *testing.T) {
	t.Run("direct_json", func(t *testing.T) {
		input := `{"artists":[{"name":"The National","is_headliner":true}],"venue":{"name":"Crescent Ballroom","city":"Phoenix","state":"AZ"},"date":"2025-03-15","time":"20:00","cost":"$25","ages":"21+"}`

		result := parseExtractionResponse(input)

		require.NotNil(t, result)
		assert.Equal(t, "2025-03-15", result["date"])
		assert.Equal(t, "20:00", result["time"])
		assert.Equal(t, "$25", result["cost"])
		assert.Equal(t, "21+", result["ages"])

		artists := result["artists"].([]interface{})
		assert.Len(t, artists, 1)
		artist := artists[0].(map[string]interface{})
		assert.Equal(t, "The National", artist["name"])
		assert.Equal(t, true, artist["is_headliner"])
	})

	t.Run("markdown_code_block_json", func(t *testing.T) {
		input := "Here is the extracted info:\n\n```json\n{\"artists\":[{\"name\":\"Turnstile\",\"is_headliner\":true}],\"date\":\"2025-04-20\"}\n```\n\nI extracted the above from the flyer."

		result := parseExtractionResponse(input)

		require.NotNil(t, result)
		assert.Equal(t, "2025-04-20", result["date"])
		artists := result["artists"].([]interface{})
		assert.Len(t, artists, 1)
	})

	t.Run("markdown_code_block_no_lang", func(t *testing.T) {
		input := "```\n{\"artists\":[{\"name\":\"Foo\",\"is_headliner\":true}]}\n```"

		result := parseExtractionResponse(input)

		require.NotNil(t, result)
		artists := result["artists"].([]interface{})
		assert.Len(t, artists, 1)
	})

	t.Run("bare_json_in_text", func(t *testing.T) {
		input := "I found the following information: {\"artists\":[{\"name\":\"Bar\",\"is_headliner\":false}],\"cost\":\"Free\"} from the flyer."

		result := parseExtractionResponse(input)

		require.NotNil(t, result)
		assert.Equal(t, "Free", result["cost"])
	})

	t.Run("invalid_json", func(t *testing.T) {
		input := "I couldn't extract any information from this image."

		result := parseExtractionResponse(input)

		assert.Nil(t, result)
	})

	t.Run("empty_string", func(t *testing.T) {
		result := parseExtractionResponse("")
		assert.Nil(t, result)
	})

	t.Run("nested_json_objects", func(t *testing.T) {
		input := `{"artists":[{"name":"Artist A","is_headliner":true},{"name":"Artist B","is_headliner":false}],"venue":{"name":"The Rebel Lounge","city":"Phoenix","state":"AZ"}}`

		result := parseExtractionResponse(input)

		require.NotNil(t, result)
		artists := result["artists"].([]interface{})
		assert.Len(t, artists, 2)

		venue := result["venue"].(map[string]interface{})
		assert.Equal(t, "The Rebel Lounge", venue["name"])
		assert.Equal(t, "Phoenix", venue["city"])
	})

	t.Run("minimal_json_only_artists", func(t *testing.T) {
		input := `{"artists":[{"name":"Solo Act","is_headliner":true}]}`

		result := parseExtractionResponse(input)

		require.NotNil(t, result)
		assert.Nil(t, result["venue"])
		assert.Nil(t, result["date"])
	})
}

// =============================================================================
// extractRawArtists TESTS
// =============================================================================

func TestExtractRawArtists(t *testing.T) {
	t.Run("multiple_artists", func(t *testing.T) {
		parsed := map[string]interface{}{
			"artists": []interface{}{
				map[string]interface{}{"name": "Headliner Band", "is_headliner": true},
				map[string]interface{}{"name": "Opener 1", "is_headliner": false},
				map[string]interface{}{"name": "Opener 2", "is_headliner": false},
			},
		}

		artists := extractRawArtists(parsed)

		require.Len(t, artists, 3)
		assert.Equal(t, "Headliner Band", artists[0].Name)
		assert.True(t, artists[0].IsHeadliner)
		assert.Equal(t, "Opener 1", artists[1].Name)
		assert.False(t, artists[1].IsHeadliner)
		assert.Equal(t, "Opener 2", artists[2].Name)
		assert.False(t, artists[2].IsHeadliner)
	})

	t.Run("no_artists_key", func(t *testing.T) {
		parsed := map[string]interface{}{
			"venue": map[string]interface{}{"name": "Some Venue"},
		}

		artists := extractRawArtists(parsed)
		assert.Nil(t, artists)
	})

	t.Run("empty_artists_array", func(t *testing.T) {
		parsed := map[string]interface{}{
			"artists": []interface{}{},
		}

		artists := extractRawArtists(parsed)
		assert.Nil(t, artists)
	})

	t.Run("skips_entries_without_name", func(t *testing.T) {
		parsed := map[string]interface{}{
			"artists": []interface{}{
				map[string]interface{}{"name": "Valid Artist", "is_headliner": true},
				map[string]interface{}{"is_headliner": false},             // no name
				map[string]interface{}{"name": "", "is_headliner": false}, // empty name
				map[string]interface{}{"name": "Another Valid", "is_headliner": false},
			},
		}

		artists := extractRawArtists(parsed)
		require.Len(t, artists, 2)
		assert.Equal(t, "Valid Artist", artists[0].Name)
		assert.Equal(t, "Another Valid", artists[1].Name)
	})

	t.Run("artists_not_array", func(t *testing.T) {
		parsed := map[string]interface{}{
			"artists": "not an array",
		}

		artists := extractRawArtists(parsed)
		assert.Nil(t, artists)
	})

	t.Run("missing_is_headliner_defaults_false", func(t *testing.T) {
		parsed := map[string]interface{}{
			"artists": []interface{}{
				map[string]interface{}{"name": "Solo Artist"},
			},
		}

		artists := extractRawArtists(parsed)
		require.Len(t, artists, 1)
		assert.Equal(t, "Solo Artist", artists[0].Name)
		assert.False(t, artists[0].IsHeadliner)
	})

	t.Run("parses_instagram_handle", func(t *testing.T) {
		parsed := map[string]interface{}{
			"artists": []interface{}{
				map[string]interface{}{"name": "Artist With IG", "is_headliner": true, "instagram_handle": "@artist_ig"},
				map[string]interface{}{"name": "Artist Without IG", "is_headliner": false},
			},
		}

		artists := extractRawArtists(parsed)
		require.Len(t, artists, 2)
		assert.Equal(t, "@artist_ig", artists[0].InstagramHandle)
		assert.Equal(t, "", artists[1].InstagramHandle)
	})

	t.Run("missing_instagram_handle_defaults_empty", func(t *testing.T) {
		parsed := map[string]interface{}{
			"artists": []interface{}{
				map[string]interface{}{"name": "Band A", "is_headliner": true},
				map[string]interface{}{"name": "Band B", "is_headliner": false},
			},
		}

		artists := extractRawArtists(parsed)
		require.Len(t, artists, 2)
		assert.Equal(t, "", artists[0].InstagramHandle)
		assert.Equal(t, "", artists[1].InstagramHandle)
	})
}

// =============================================================================
// buildUserContent TESTS
// =============================================================================

func TestBuildUserContent(t *testing.T) {
	cfg := &config.Config{}
	svc := &ExtractionService{config: cfg}

	t.Run("text_type", func(t *testing.T) {
		req := &ExtractShowRequest{Type: "text", Text: "Show at Crescent Ballroom, March 15"}
		content := svc.buildUserContent(req)

		require.Len(t, content, 1)
		textBlock := content[0].(map[string]string)
		assert.Equal(t, "text", textBlock["type"])
		assert.Contains(t, textBlock["text"], "Crescent Ballroom")
	})

	t.Run("image_type", func(t *testing.T) {
		req := &ExtractShowRequest{
			Type:      "image",
			ImageData: "base64data",
			MediaType: "image/jpeg",
		}
		content := svc.buildUserContent(req)

		require.Len(t, content, 2)
		imageBlock := content[0].(map[string]interface{})
		assert.Equal(t, "image", imageBlock["type"])
		source := imageBlock["source"].(map[string]string)
		assert.Equal(t, "base64", source["type"])
		assert.Equal(t, "image/jpeg", source["media_type"])
		assert.Equal(t, "base64data", source["data"])

		textBlock := content[1].(map[string]string)
		assert.Equal(t, "text", textBlock["type"])
	})

	t.Run("both_type_with_text", func(t *testing.T) {
		req := &ExtractShowRequest{
			Type:      "both",
			Text:      "Extra context",
			ImageData: "base64data",
			MediaType: "image/png",
		}
		content := svc.buildUserContent(req)

		require.Len(t, content, 2)
		textBlock := content[1].(map[string]string)
		assert.Contains(t, textBlock["text"], "Extra context")
	})

	t.Run("both_type_empty_text", func(t *testing.T) {
		req := &ExtractShowRequest{
			Type:      "both",
			Text:      "",
			ImageData: "base64data",
			MediaType: "image/png",
		}
		content := svc.buildUserContent(req)

		require.Len(t, content, 2)
		textBlock := content[1].(map[string]string)
		assert.NotContains(t, textBlock["text"], "Additional context")
	})
}

// =============================================================================
// ExtractShow REQUEST VALIDATION TESTS
// =============================================================================

func TestExtractShowValidation(t *testing.T) {
	cfg := &config.Config{
		Anthropic: config.AnthropicConfig{APIKey: "test-key"},
	}
	// Use a local server that returns a non-validation error so tests that pass
	// validation but call callAnthropic fail fast instead of timing out.
	validationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("test server"))
	}))
	defer validationServer.Close()
	svc := &ExtractionService{config: cfg, httpClient: validationServer.Client(), anthropicBaseURL: validationServer.URL}

	t.Run("missing_api_key", func(t *testing.T) {
		svc := &ExtractionService{config: &config.Config{}}
		req := &ExtractShowRequest{Type: "text", Text: "test"}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "AI service not configured", resp.Error)
	})

	t.Run("invalid_type", func(t *testing.T) {
		req := &ExtractShowRequest{Type: "invalid"}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid request type")
	})

	t.Run("text_type_empty_text", func(t *testing.T) {
		req := &ExtractShowRequest{Type: "text", Text: ""}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "Text content is required", resp.Error)
	})

	t.Run("text_type_whitespace_only", func(t *testing.T) {
		req := &ExtractShowRequest{Type: "text", Text: "   \n\t  "}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "Text content is required", resp.Error)
	})

	t.Run("text_type_too_long", func(t *testing.T) {
		longText := make([]byte, 10001)
		for i := range longText {
			longText[i] = 'a'
		}
		req := &ExtractShowRequest{Type: "text", Text: string(longText)}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "exceeds maximum length")
	})

	t.Run("image_type_missing_data", func(t *testing.T) {
		req := &ExtractShowRequest{Type: "image", ImageData: "", MediaType: "image/jpeg"}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "Image data is required", resp.Error)
	})

	t.Run("image_type_missing_media_type", func(t *testing.T) {
		req := &ExtractShowRequest{Type: "image", ImageData: "base64data", MediaType: ""}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "Image media type is required", resp.Error)
	})

	t.Run("image_type_invalid_media_type", func(t *testing.T) {
		req := &ExtractShowRequest{Type: "image", ImageData: "base64data", MediaType: "image/bmp"}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid image type")
	})

	t.Run("image_type_valid_media_types", func(t *testing.T) {
		validTypes := []string{"image/jpeg", "image/png", "image/gif", "image/webp"}
		for _, mt := range validTypes {
			req := &ExtractShowRequest{Type: "image", ImageData: "base64data", MediaType: mt}
			resp, err := svc.ExtractShow(req)
			assert.NoError(t, err)
			// Should get past validation (fail on the API call, not validation)
			if !resp.Success {
				assert.NotContains(t, resp.Error, "Invalid image type", "media type %s should be valid", mt)
			}
		}
	})

	t.Run("both_type_text_too_long", func(t *testing.T) {
		longText := make([]byte, 10001)
		for i := range longText {
			longText[i] = 'a'
		}
		req := &ExtractShowRequest{
			Type:      "both",
			Text:      string(longText),
			ImageData: "base64data",
			MediaType: "image/jpeg",
		}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "exceeds maximum length")
	})
}

// =============================================================================
// callAnthropic TESTS (httptest mock)
// =============================================================================

// newTestExtractionService creates an ExtractionService with an httptest server
func newTestExtractionService(handler http.HandlerFunc) (*ExtractionService, *httptest.Server) {
	server := httptest.NewServer(handler)
	svc := &ExtractionService{
		config: &config.Config{
			Anthropic: config.AnthropicConfig{APIKey: "test-api-key"},
		},
		httpClient:       server.Client(),
		anthropicBaseURL: server.URL,
	}
	return svc, server
}

func TestCallAnthropic(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{Type: "text", Text: "hello"},
				},
			})
		})
		defer server.Close()

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.NoError(t, err)
		assert.Equal(t, "hello", result)
	})

	t.Run("multiple_content_blocks", func(t *testing.T) {
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{Type: "text", Text: "first"},
					{Type: "text", Text: " second"},
				},
			})
		})
		defer server.Close()

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.NoError(t, err)
		assert.Equal(t, "first second", result)
	})

	t.Run("empty_content", func(t *testing.T) {
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{},
			})
		})
		defer server.Close()

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("non_text_blocks_ignored", func(t *testing.T) {
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Write raw JSON to include non-text blocks
			w.Write([]byte(`{"content":[{"type":"tool_use","text":"ignored"},{"type":"text","text":"kept"}]}`))
		})
		defer server.Close()

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.NoError(t, err)
		assert.Equal(t, "kept", result)
	})

	t.Run("non_200_status", func(t *testing.T) {
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
		})
		defer server.Close()

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.Error(t, err)
		assert.Empty(t, result)
		assert.Contains(t, err.Error(), "status 429")
	})

	t.Run("api_error_response", func(t *testing.T) {
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"error":{"type":"overloaded_error","message":"Overloaded"}}`))
		})
		defer server.Close()

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.Error(t, err)
		assert.Empty(t, result)
		assert.Contains(t, err.Error(), "Overloaded")
	})

	t.Run("credit_billing_error", func(t *testing.T) {
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Your credit balance is too low"))
		})
		defer server.Close()

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.Error(t, err)
		assert.Empty(t, result)
		assert.Contains(t, err.Error(), "credit")
	})

	t.Run("invalid_response_json", func(t *testing.T) {
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not json"))
		})
		defer server.Close()

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.Error(t, err)
		assert.Empty(t, result)
		assert.Contains(t, err.Error(), "failed to parse")
	})

	t.Run("request_headers", func(t *testing.T) {
		var capturedHeaders http.Header
		var capturedBody []byte
		svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
			capturedHeaders = r.Header.Clone()
			capturedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(anthropicResponse{
				Content: []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{
					{Type: "text", Text: "ok"},
				},
			})
		})
		defer server.Close()

		_, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})
		assert.NoError(t, err)

		// Verify headers
		assert.Equal(t, "application/json", capturedHeaders.Get("Content-Type"))
		assert.Equal(t, "test-api-key", capturedHeaders.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", capturedHeaders.Get("anthropic-version"))

		// Verify body contains expected fields
		var reqBody anthropicRequest
		err = json.Unmarshal(capturedBody, &reqBody)
		assert.NoError(t, err)
		assert.Equal(t, "claude-haiku-4-5-20251001", reqBody.Model)
		assert.Equal(t, 1024, reqBody.MaxTokens)
		assert.Equal(t, extractionSystemPrompt, reqBody.System)
	})

	t.Run("http_client_error", func(t *testing.T) {
		svc := &ExtractionService{
			config: &config.Config{
				Anthropic: config.AnthropicConfig{APIKey: "test-key"},
			},
			httpClient:       &http.Client{Timeout: 1 * time.Millisecond},
			anthropicBaseURL: "http://192.0.2.1:1", // RFC 5737 TEST-NET, unreachable
		}

		result, err := svc.callAnthropic([]interface{}{map[string]string{"type": "text", "text": "test"}})

		assert.Error(t, err)
		assert.Empty(t, result)
		assert.Contains(t, err.Error(), "request failed")
	})
}

// =============================================================================
// matchArtists UNIT TESTS (edge cases, no DB)
// =============================================================================

func TestMatchArtists(t *testing.T) {
	t.Run("empty_input", func(t *testing.T) {
		svc := &ExtractionService{
			artistService: &ArtistService{db: nil},
		}

		result := svc.matchArtists([]rawArtist{})

		assert.Nil(t, result)
	})

	t.Run("nil_input", func(t *testing.T) {
		svc := &ExtractionService{
			artistService: &ArtistService{db: nil},
		}

		result := svc.matchArtists(nil)

		assert.Nil(t, result)
	})

	t.Run("search_error_returns_unmatched", func(t *testing.T) {
		svc := &ExtractionService{
			artistService: &ArtistService{db: nil}, // nil db → "database not initialized" error
		}

		result := svc.matchArtists([]rawArtist{
			{Name: "Test Artist", IsHeadliner: true},
		})

		require.Len(t, result, 1)
		assert.Equal(t, "Test Artist", result[0].Name)
		assert.True(t, result[0].IsHeadliner)
		assert.Nil(t, result[0].MatchedID)
		assert.Nil(t, result[0].Suggestions)
	})
}

// =============================================================================
// matchVenue UNIT TESTS (edge cases, no DB)
// =============================================================================

func TestMatchVenue(t *testing.T) {
	t.Run("no_venue_in_parsed", func(t *testing.T) {
		svc := &ExtractionService{
			venueService: &VenueService{db: nil},
		}

		result := svc.matchVenue(map[string]interface{}{
			"date": "2025-03-15",
		})

		assert.Nil(t, result)
	})

	t.Run("venue_not_map", func(t *testing.T) {
		svc := &ExtractionService{
			venueService: &VenueService{db: nil},
		}

		result := svc.matchVenue(map[string]interface{}{
			"venue": "just a string",
		})

		assert.Nil(t, result)
	})

	t.Run("empty_name", func(t *testing.T) {
		svc := &ExtractionService{
			venueService: &VenueService{db: nil},
		}

		result := svc.matchVenue(map[string]interface{}{
			"venue": map[string]interface{}{
				"name": "",
			},
		})

		assert.Nil(t, result)
	})

	t.Run("search_error_returns_unmatched", func(t *testing.T) {
		svc := &ExtractionService{
			venueService: &VenueService{db: nil}, // nil db → error
		}

		result := svc.matchVenue(map[string]interface{}{
			"venue": map[string]interface{}{
				"name":  "Test Venue",
				"city":  "Phoenix",
				"state": "AZ",
			},
		})

		require.NotNil(t, result)
		assert.Equal(t, "Test Venue", result.Name)
		assert.Equal(t, "Phoenix", result.City)
		assert.Equal(t, "AZ", result.State)
		assert.Nil(t, result.MatchedID)
		assert.Nil(t, result.Suggestions)
	})
}

// =============================================================================
// matchArtists / matchVenue INTEGRATION TESTS (testcontainers)
// =============================================================================

type ExtractionIntegrationTestSuite struct {
	suite.Suite
	container         testcontainers.Container
	db                *gorm.DB
	extractionService *ExtractionService
	ctx               context.Context
}

func (suite *ExtractionIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		suite.T().Fatalf("failed to start postgres container: %v", err)
	}
	suite.container = container

	host, err := container.Host(suite.ctx)
	if err != nil {
		suite.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(suite.ctx, "5432")
	if err != nil {
		suite.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		suite.T().Fatalf("failed to connect to test database: %v", err)
	}
	suite.db = db

	// Run migrations
	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}

	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000002_add_artist_search_indexes.up.sql",
		"000003_add_venue_search_indexes.up.sql",
		"000004_update_venue_constraints.up.sql",
		"000005_add_show_status.up.sql",
		"000007_add_private_show_status.up.sql",
		"000008_add_pending_venue_edits.up.sql",
		"000009_add_bandcamp_embed_url.up.sql",
		"000010_add_scraper_source_fields.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000013_add_slugs.up.sql",
		"000014_add_account_lockout.up.sql",
		"000020_add_show_status_flags.up.sql",
		"000023_rename_scraper_to_discovery.up.sql",
		"000026_add_duplicate_of_show_id.up.sql",
		"000028_change_event_date_to_timestamptz.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
	}
	for _, m := range migrations {
		migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", m))
		if err != nil {
			suite.T().Fatalf("failed to read migration file %s: %v", m, err)
		}
		_, err = sqlDB.Exec(string(migrationSQL))
		if err != nil {
			suite.T().Fatalf("failed to run migration %s: %v", m, err)
		}
	}

	// Run migration 000027 with CONCURRENTLY stripped
	migration27, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000027_add_index_duplicate_of_show_id.up.sql"))
	if err != nil {
		suite.T().Fatalf("failed to read migration 000027: %v", err)
	}
	sql27 := strings.ReplaceAll(string(migration27), "CONCURRENTLY ", "")
	_, err = sqlDB.Exec(sql27)
	if err != nil {
		suite.T().Fatalf("failed to run migration 000027: %v", err)
	}

	suite.extractionService = &ExtractionService{
		config:        &config.Config{},
		artistService: &ArtistService{db: db},
		venueService:  &VenueService{db: db},
	}
}

func (suite *ExtractionIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *ExtractionIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func (suite *ExtractionIntegrationTestSuite) createArtist(name string) models.Artist {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	artist := models.Artist{Name: name, Slug: &slug}
	err := suite.db.Create(&artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *ExtractionIntegrationTestSuite) createVenue(name, city, state string) models.Venue {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	venue := models.Venue{Name: name, City: city, State: state, Slug: &slug}
	err := suite.db.Create(&venue).Error
	suite.Require().NoError(err)
	return venue
}

// --- matchArtists integration tests ---

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_ExactMatch() {
	suite.createArtist("Radiohead")

	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "Radiohead", IsHeadliner: true},
	})

	suite.Require().Len(result, 1)
	suite.NotNil(result[0].MatchedID)
	suite.Equal("Radiohead", *result[0].MatchedName)
	suite.Equal("radiohead", *result[0].MatchedSlug)
	suite.True(result[0].IsHeadliner)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_CaseInsensitive() {
	suite.createArtist("The National")

	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "the national", IsHeadliner: false},
	})

	suite.Require().Len(result, 1)
	suite.NotNil(result[0].MatchedID)
	suite.Equal("The National", *result[0].MatchedName)
	suite.False(result[0].IsHeadliner)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_NoMatchEmptyDB() {
	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "Nonexistent Band", IsHeadliner: true},
	})

	suite.Require().Len(result, 1)
	suite.Nil(result[0].MatchedID)
	suite.Empty(result[0].Suggestions)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_SuggestionsWhenNoExact() {
	suite.createArtist("Radio Moscow")
	suite.createArtist("Radio Dept")
	suite.createArtist("Radio Birdman")

	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "Radio", IsHeadliner: false},
	})

	suite.Require().Len(result, 1)
	suite.Nil(result[0].MatchedID, "Should not have exact match")
	suite.NotEmpty(result[0].Suggestions)
	for _, s := range result[0].Suggestions {
		suite.NotZero(s.ID)
		suite.NotEmpty(s.Name)
		suite.NotEmpty(s.Slug)
	}
}

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_SuggestionsCappedAt3() {
	for i := 0; i < 5; i++ {
		suite.createArtist(fmt.Sprintf("TestBand%d", i))
	}

	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "TestBand", IsHeadliner: false},
	})

	suite.Require().Len(result, 1)
	suite.Nil(result[0].MatchedID)
	suite.LessOrEqual(len(result[0].Suggestions), 3)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_MultipleArtists() {
	suite.createArtist("Turnstile")
	suite.createArtist("Ceremony")

	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "Turnstile", IsHeadliner: true},
		{Name: "Ceremony", IsHeadliner: false},
	})

	suite.Require().Len(result, 2)
	suite.NotNil(result[0].MatchedID)
	suite.Equal("Turnstile", *result[0].MatchedName)
	suite.True(result[0].IsHeadliner)
	suite.NotNil(result[1].MatchedID)
	suite.Equal("Ceremony", *result[1].MatchedName)
	suite.False(result[1].IsHeadliner)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_InstagramPreservedForNewArtist() {
	// No artists in DB — new artist should preserve instagram handle
	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "New Band", IsHeadliner: true, InstagramHandle: "@newband"},
	})

	suite.Require().Len(result, 1)
	suite.Equal("@newband", result[0].InstagramHandle)
	suite.Nil(result[0].MatchedID)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_InstagramClearedForMatchedArtist() {
	suite.createArtist("Existing Band")

	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "Existing Band", IsHeadliner: false, InstagramHandle: "@existing_ig"},
	})

	suite.Require().Len(result, 1)
	suite.NotNil(result[0].MatchedID)
	suite.Equal("", result[0].InstagramHandle, "Instagram handle should be cleared for matched artists")
}

func (suite *ExtractionIntegrationTestSuite) TestMatchArtists_InstagramMixedMatchAndNew() {
	suite.createArtist("Known Artist")

	result := suite.extractionService.matchArtists([]rawArtist{
		{Name: "Known Artist", IsHeadliner: true, InstagramHandle: "@known_ig"},
		{Name: "Unknown Artist", IsHeadliner: false, InstagramHandle: "@unknown_ig"},
	})

	suite.Require().Len(result, 2)
	// Matched artist: instagram cleared
	suite.NotNil(result[0].MatchedID)
	suite.Equal("", result[0].InstagramHandle)
	// New artist: instagram preserved
	suite.Nil(result[1].MatchedID)
	suite.Equal("@unknown_ig", result[1].InstagramHandle)
}

// --- matchVenue integration tests ---

func (suite *ExtractionIntegrationTestSuite) TestMatchVenue_ExactMatch() {
	suite.createVenue("Crescent Ballroom", "Phoenix", "AZ")

	result := suite.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name":  "Crescent Ballroom",
			"city":  "Tempe",
			"state": "AZ",
		},
	})

	suite.Require().NotNil(result)
	suite.NotNil(result.MatchedID)
	suite.Equal("Crescent Ballroom", *result.MatchedName)
	suite.Equal("crescent-ballroom", *result.MatchedSlug)
	// City/State overridden from DB
	suite.Equal("Phoenix", result.City)
	suite.Equal("AZ", result.State)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchVenue_CaseInsensitive() {
	suite.createVenue("Valley Bar", "Phoenix", "AZ")

	result := suite.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name": "valley bar",
		},
	})

	suite.Require().NotNil(result)
	suite.NotNil(result.MatchedID)
	suite.Equal("Valley Bar", *result.MatchedName)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchVenue_NoMatchEmptyDB() {
	result := suite.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name":  "Nonexistent Venue",
			"city":  "Phoenix",
			"state": "AZ",
		},
	})

	suite.Require().NotNil(result)
	suite.Nil(result.MatchedID)
	suite.Empty(result.Suggestions)
}

func (suite *ExtractionIntegrationTestSuite) TestMatchVenue_SuggestionsWhenNoExact() {
	suite.createVenue("The Rebel Lounge", "Phoenix", "AZ")
	suite.createVenue("The Rebel Room", "Mesa", "AZ")

	result := suite.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name": "Rebel",
		},
	})

	suite.Require().NotNil(result)
	suite.Nil(result.MatchedID, "Should not have exact match")
	suite.NotEmpty(result.Suggestions)
	for _, s := range result.Suggestions {
		suite.NotZero(s.ID)
		suite.NotEmpty(s.Name)
		suite.NotEmpty(s.Slug)
		suite.NotEmpty(s.City)
		suite.NotEmpty(s.State)
	}
}

func (suite *ExtractionIntegrationTestSuite) TestMatchVenue_SuggestionsCappedAt3() {
	for i := 0; i < 5; i++ {
		suite.createVenue(fmt.Sprintf("TestVenue%d", i), "Phoenix", "AZ")
	}

	result := suite.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name": "TestVenue",
		},
	})

	suite.Require().NotNil(result)
	suite.Nil(result.MatchedID)
	suite.LessOrEqual(len(result.Suggestions), 3)
}

func TestExtractionIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ExtractionIntegrationTestSuite))
}
