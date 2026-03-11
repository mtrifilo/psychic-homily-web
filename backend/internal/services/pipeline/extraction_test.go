package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// NewExtractionService TESTS
// =============================================================================

func TestNewExtractionService(t *testing.T) {
	cfg := &config.Config{
		Anthropic: config.AnthropicConfig{APIKey: "test-key"},
	}
	// Use nil-returning stubs for the local interfaces
	svc := NewExtractionService(nil, cfg, &testArtistSearcher{}, &testVenueSearcher{})
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
		req := &contracts.ExtractShowRequest{Type: "text", Text: "Show at Crescent Ballroom, March 15"}
		content := svc.buildUserContent(req)

		require.Len(t, content, 1)
		textBlock := content[0].(map[string]string)
		assert.Equal(t, "text", textBlock["type"])
		assert.Contains(t, textBlock["text"], "Crescent Ballroom")
	})

	t.Run("image_type", func(t *testing.T) {
		req := &contracts.ExtractShowRequest{
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
		req := &contracts.ExtractShowRequest{
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
		req := &contracts.ExtractShowRequest{
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
	validationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("test server"))
	}))
	defer validationServer.Close()
	svc := &ExtractionService{config: cfg, httpClient: validationServer.Client(), anthropicBaseURL: validationServer.URL}

	t.Run("missing_api_key", func(t *testing.T) {
		svc := &ExtractionService{config: &config.Config{}}
		req := &contracts.ExtractShowRequest{Type: "text", Text: "test"}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "AI service not configured", resp.Error)
	})

	t.Run("invalid_type", func(t *testing.T) {
		req := &contracts.ExtractShowRequest{Type: "invalid"}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid request type")
	})

	t.Run("text_type_empty_text", func(t *testing.T) {
		req := &contracts.ExtractShowRequest{Type: "text", Text: ""}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "Text content is required", resp.Error)
	})

	t.Run("text_type_whitespace_only", func(t *testing.T) {
		req := &contracts.ExtractShowRequest{Type: "text", Text: "   \n\t  "}

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
		req := &contracts.ExtractShowRequest{Type: "text", Text: string(longText)}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "exceeds maximum length")
	})

	t.Run("image_type_missing_data", func(t *testing.T) {
		req := &contracts.ExtractShowRequest{Type: "image", ImageData: "", MediaType: "image/jpeg"}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "Image data is required", resp.Error)
	})

	t.Run("image_type_missing_media_type", func(t *testing.T) {
		req := &contracts.ExtractShowRequest{Type: "image", ImageData: "base64data", MediaType: ""}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "Image media type is required", resp.Error)
	})

	t.Run("image_type_invalid_media_type", func(t *testing.T) {
		req := &contracts.ExtractShowRequest{Type: "image", ImageData: "base64data", MediaType: "image/bmp"}

		resp, err := svc.ExtractShow(req)

		assert.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid image type")
	})

	t.Run("image_type_valid_media_types", func(t *testing.T) {
		validTypes := []string{"image/jpeg", "image/png", "image/gif", "image/webp"}
		for _, mt := range validTypes {
			req := &contracts.ExtractShowRequest{Type: "image", ImageData: "base64data", MediaType: mt}
			resp, err := svc.ExtractShow(req)
			assert.NoError(t, err)
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
		req := &contracts.ExtractShowRequest{
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
// Test implementations of local interfaces for unit tests
// =============================================================================

// testArtistSearcher implements artistSearcher for unit tests with nil DB behavior.
type testArtistSearcher struct {
	db *gorm.DB
}

func (s *testArtistSearcher) SearchArtists(query string) ([]*contracts.ArtistDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var artists []models.Artist
	err := s.db.Where("LOWER(name) LIKE LOWER(?)", "%"+query+"%").Limit(10).Find(&artists).Error
	if err != nil {
		return nil, err
	}
	var results []*contracts.ArtistDetailResponse
	for _, a := range artists {
		slug := ""
		if a.Slug != nil {
			slug = *a.Slug
		}
		results = append(results, &contracts.ArtistDetailResponse{
			ID:   a.ID,
			Name: a.Name,
			Slug: slug,
		})
	}
	return results, nil
}

// testVenueSearcher implements venueSearcher for unit tests with nil DB behavior.
type testVenueSearcher struct {
	db *gorm.DB
}

func (s *testVenueSearcher) SearchVenues(query string) ([]*contracts.VenueDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var venues []models.Venue
	err := s.db.Where("LOWER(name) LIKE LOWER(?)", "%"+query+"%").Limit(10).Find(&venues).Error
	if err != nil {
		return nil, err
	}
	var results []*contracts.VenueDetailResponse
	for _, v := range venues {
		slug := ""
		if v.Slug != nil {
			slug = *v.Slug
		}
		results = append(results, &contracts.VenueDetailResponse{
			ID:    v.ID,
			Name:  v.Name,
			Slug:  slug,
			City:  v.City,
			State: v.State,
		})
	}
	return results, nil
}

// =============================================================================
// matchArtists UNIT TESTS (edge cases, no DB)
// =============================================================================

func TestMatchArtists(t *testing.T) {
	t.Run("empty_input", func(t *testing.T) {
		svc := &ExtractionService{
			artistService: &testArtistSearcher{db: nil},
		}

		result := svc.matchArtists([]rawArtist{})

		assert.Nil(t, result)
	})

	t.Run("nil_input", func(t *testing.T) {
		svc := &ExtractionService{
			artistService: &testArtistSearcher{db: nil},
		}

		result := svc.matchArtists(nil)

		assert.Nil(t, result)
	})

	t.Run("search_error_returns_unmatched", func(t *testing.T) {
		svc := &ExtractionService{
			artistService: &testArtistSearcher{db: nil}, // nil db -> error
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
			venueService: &testVenueSearcher{db: nil},
		}

		result := svc.matchVenue(map[string]interface{}{
			"date": "2025-03-15",
		})

		assert.Nil(t, result)
	})

	t.Run("venue_not_map", func(t *testing.T) {
		svc := &ExtractionService{
			venueService: &testVenueSearcher{db: nil},
		}

		result := svc.matchVenue(map[string]interface{}{
			"venue": "just a string",
		})

		assert.Nil(t, result)
	})

	t.Run("empty_name", func(t *testing.T) {
		svc := &ExtractionService{
			venueService: &testVenueSearcher{db: nil},
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
			venueService: &testVenueSearcher{db: nil}, // nil db -> error
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

func (s *ExtractionIntegrationTestSuite) SetupSuite() {
	s.ctx = context.Background()

	container, err := testcontainers.GenericContainer(s.ctx, testcontainers.GenericContainerRequest{
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
		s.T().Fatalf("failed to start postgres container: %v", err)
	}
	s.container = container

	host, err := container.Host(s.ctx)
	if err != nil {
		s.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(s.ctx, "5432")
	if err != nil {
		s.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		s.T().Fatalf("failed to connect to test database: %v", err)
	}
	s.db = db

	sqlDB, err := db.DB()
	if err != nil {
		s.T().Fatalf("failed to get sql.DB: %v", err)
	}
	testutil.RunAllMigrations(s.T(), sqlDB, filepath.Join("..", "..", "..", "db", "migrations"))

	s.extractionService = &ExtractionService{
		config:        &config.Config{},
		artistService: &testArtistSearcher{db: db},
		venueService:  &testVenueSearcher{db: db},
	}
}

func (s *ExtractionIntegrationTestSuite) TearDownSuite() {
	if s.container != nil {
		if err := s.container.Terminate(s.ctx); err != nil {
			s.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (s *ExtractionIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func (s *ExtractionIntegrationTestSuite) createArtist(name string) models.Artist {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	artist := models.Artist{Name: name, Slug: &slug}
	err := s.db.Create(&artist).Error
	s.Require().NoError(err)
	return artist
}

func (s *ExtractionIntegrationTestSuite) createVenue(name, city, state string) models.Venue {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	venue := models.Venue{Name: name, City: city, State: state, Slug: &slug}
	err := s.db.Create(&venue).Error
	s.Require().NoError(err)
	return venue
}

// --- matchArtists integration tests ---

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_ExactMatch() {
	s.createArtist("Radiohead")

	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "Radiohead", IsHeadliner: true},
	})

	s.Require().Len(result, 1)
	s.NotNil(result[0].MatchedID)
	s.Equal("Radiohead", *result[0].MatchedName)
	s.Equal("radiohead", *result[0].MatchedSlug)
	s.True(result[0].IsHeadliner)
}

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_CaseInsensitive() {
	s.createArtist("The National")

	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "the national", IsHeadliner: false},
	})

	s.Require().Len(result, 1)
	s.NotNil(result[0].MatchedID)
	s.Equal("The National", *result[0].MatchedName)
	s.False(result[0].IsHeadliner)
}

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_NoMatchEmptyDB() {
	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "Nonexistent Band", IsHeadliner: true},
	})

	s.Require().Len(result, 1)
	s.Nil(result[0].MatchedID)
	s.Empty(result[0].Suggestions)
}

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_SuggestionsWhenNoExact() {
	s.createArtist("Radio Moscow")
	s.createArtist("Radio Dept")
	s.createArtist("Radio Birdman")

	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "Radio", IsHeadliner: false},
	})

	s.Require().Len(result, 1)
	s.Nil(result[0].MatchedID, "Should not have exact match")
	s.NotEmpty(result[0].Suggestions)
	for _, sg := range result[0].Suggestions {
		s.NotZero(sg.ID)
		s.NotEmpty(sg.Name)
		s.NotEmpty(sg.Slug)
	}
}

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_SuggestionsCappedAt3() {
	for i := 0; i < 5; i++ {
		s.createArtist(fmt.Sprintf("TestBand%d", i))
	}

	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "TestBand", IsHeadliner: false},
	})

	s.Require().Len(result, 1)
	s.Nil(result[0].MatchedID)
	s.LessOrEqual(len(result[0].Suggestions), 3)
}

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_MultipleArtists() {
	s.createArtist("Turnstile")
	s.createArtist("Ceremony")

	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "Turnstile", IsHeadliner: true},
		{Name: "Ceremony", IsHeadliner: false},
	})

	s.Require().Len(result, 2)
	s.NotNil(result[0].MatchedID)
	s.Equal("Turnstile", *result[0].MatchedName)
	s.True(result[0].IsHeadliner)
	s.NotNil(result[1].MatchedID)
	s.Equal("Ceremony", *result[1].MatchedName)
	s.False(result[1].IsHeadliner)
}

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_InstagramPreservedForNewArtist() {
	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "New Band", IsHeadliner: true, InstagramHandle: "@newband"},
	})

	s.Require().Len(result, 1)
	s.Equal("@newband", result[0].InstagramHandle)
	s.Nil(result[0].MatchedID)
}

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_InstagramClearedForMatchedArtist() {
	s.createArtist("Existing Band")

	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "Existing Band", IsHeadliner: false, InstagramHandle: "@existing_ig"},
	})

	s.Require().Len(result, 1)
	s.NotNil(result[0].MatchedID)
	s.Equal("", result[0].InstagramHandle, "Instagram handle should be cleared for matched artists")
}

func (s *ExtractionIntegrationTestSuite) TestMatchArtists_InstagramMixedMatchAndNew() {
	s.createArtist("Known Artist")

	result := s.extractionService.matchArtists([]rawArtist{
		{Name: "Known Artist", IsHeadliner: true, InstagramHandle: "@known_ig"},
		{Name: "Unknown Artist", IsHeadliner: false, InstagramHandle: "@unknown_ig"},
	})

	s.Require().Len(result, 2)
	s.NotNil(result[0].MatchedID)
	s.Equal("", result[0].InstagramHandle)
	s.Nil(result[1].MatchedID)
	s.Equal("@unknown_ig", result[1].InstagramHandle)
}

// --- matchVenue integration tests ---

func (s *ExtractionIntegrationTestSuite) TestMatchVenue_ExactMatch() {
	s.createVenue("Crescent Ballroom", "Phoenix", "AZ")

	result := s.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name":  "Crescent Ballroom",
			"city":  "Tempe",
			"state": "AZ",
		},
	})

	s.Require().NotNil(result)
	s.NotNil(result.MatchedID)
	s.Equal("Crescent Ballroom", *result.MatchedName)
	s.Equal("crescent-ballroom", *result.MatchedSlug)
	s.Equal("Phoenix", result.City)
	s.Equal("AZ", result.State)
}

func (s *ExtractionIntegrationTestSuite) TestMatchVenue_CaseInsensitive() {
	s.createVenue("Valley Bar", "Phoenix", "AZ")

	result := s.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name": "valley bar",
		},
	})

	s.Require().NotNil(result)
	s.NotNil(result.MatchedID)
	s.Equal("Valley Bar", *result.MatchedName)
}

func (s *ExtractionIntegrationTestSuite) TestMatchVenue_NoMatchEmptyDB() {
	result := s.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name":  "Nonexistent Venue",
			"city":  "Phoenix",
			"state": "AZ",
		},
	})

	s.Require().NotNil(result)
	s.Nil(result.MatchedID)
	s.Empty(result.Suggestions)
}

func (s *ExtractionIntegrationTestSuite) TestMatchVenue_SuggestionsWhenNoExact() {
	s.createVenue("The Rebel Lounge", "Phoenix", "AZ")
	s.createVenue("The Rebel Room", "Mesa", "AZ")

	result := s.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name": "Rebel",
		},
	})

	s.Require().NotNil(result)
	s.Nil(result.MatchedID, "Should not have exact match")
	s.NotEmpty(result.Suggestions)
	for _, sg := range result.Suggestions {
		s.NotZero(sg.ID)
		s.NotEmpty(sg.Name)
		s.NotEmpty(sg.Slug)
		s.NotEmpty(sg.City)
		s.NotEmpty(sg.State)
	}
}

func (s *ExtractionIntegrationTestSuite) TestMatchVenue_SuggestionsCappedAt3() {
	for i := 0; i < 5; i++ {
		s.createVenue(fmt.Sprintf("TestVenue%d", i), "Phoenix", "AZ")
	}

	result := s.extractionService.matchVenue(map[string]interface{}{
		"venue": map[string]interface{}{
			"name": "TestVenue",
		},
	})

	s.Require().NotNil(result)
	s.Nil(result.MatchedID)
	s.LessOrEqual(len(result.Suggestions), 3)
}

func TestExtractionIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ExtractionIntegrationTestSuite))
}
