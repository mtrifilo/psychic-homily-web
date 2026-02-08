package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
)

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
				map[string]interface{}{"is_headliner": false},                        // no name
				map[string]interface{}{"name": "", "is_headliner": false},            // empty name
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
	svc := &ExtractionService{config: cfg}

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
