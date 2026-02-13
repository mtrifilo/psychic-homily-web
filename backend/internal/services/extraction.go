package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
)

const extractionSystemPrompt = `You are a show information extractor. Given text or an image of a show flyer, extract structured information.

Output ONLY valid JSON with no additional text or markdown formatting:
{
  "artists": [{"name": "Artist Name", "is_headliner": true}],
  "venue": {"name": "Venue Name", "city": "City", "state": "AZ"},
  "date": "YYYY-MM-DD",
  "time": "HH:MM",
  "cost": "$20",
  "ages": "21+"
}

Rules:
- First artist listed is usually the headliner (is_headliner: true), others are is_headliner: false
- Convert dates to YYYY-MM-DD format (assume current year if not specified)
- Convert time to 24-hour format (default to 20:00 if "doors" time is given but show time is ambiguous)
- State should be 2-letter abbreviation (default to AZ for Arizona venues)
- Omit fields if not found (don't include null or empty values)
- For cost, include the dollar sign if it's a paid show, or use "Free" if explicitly stated as free
- For ages, common formats are "21+", "18+", "All Ages"
- If multiple dates are shown, extract only the first/primary date
- Return ONLY the JSON object, no explanation or markdown code blocks`

// ExtractionService handles AI-powered show info extraction
type ExtractionService struct {
	config           *config.Config
	artistService    *ArtistService
	venueService     *VenueService
	httpClient       *http.Client
	anthropicBaseURL string
}

// NewExtractionService creates a new extraction service
func NewExtractionService(database *gorm.DB, cfg *config.Config) *ExtractionService {
	return &ExtractionService{
		config:           cfg,
		artistService:    NewArtistService(database),
		venueService:     NewVenueService(database),
		httpClient:       &http.Client{},
		anthropicBaseURL: "https://api.anthropic.com",
	}
}

// ExtractShowRequest represents the extraction request
type ExtractShowRequest struct {
	Type      string `json:"type"`       // "text", "image", or "both"
	Text      string `json:"text"`       // Text content
	ImageData string `json:"image_data"` // Base64-encoded image
	MediaType string `json:"media_type"` // MIME type of image
}

// MatchSuggestion represents a close-but-not-exact match from the database
type MatchSuggestion struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// VenueMatchSuggestion includes location info
type VenueMatchSuggestion struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	City  string `json:"city"`
	State string `json:"state"`
}

// ExtractedArtist represents an extracted artist with optional DB match
type ExtractedArtist struct {
	Name        string            `json:"name"`
	IsHeadliner bool              `json:"is_headliner"`
	MatchedID   *uint             `json:"matched_id,omitempty"`
	MatchedName *string           `json:"matched_name,omitempty"`
	MatchedSlug *string           `json:"matched_slug,omitempty"`
	Suggestions []MatchSuggestion `json:"suggestions,omitempty"`
}

// ExtractedVenue represents an extracted venue with optional DB match
type ExtractedVenue struct {
	Name        string                 `json:"name"`
	City        string                 `json:"city,omitempty"`
	State       string                 `json:"state,omitempty"`
	MatchedID   *uint                  `json:"matched_id,omitempty"`
	MatchedName *string                `json:"matched_name,omitempty"`
	MatchedSlug *string                `json:"matched_slug,omitempty"`
	Suggestions []VenueMatchSuggestion `json:"suggestions,omitempty"`
}

// ExtractedShowData is the full extraction result
type ExtractedShowData struct {
	Artists     []ExtractedArtist `json:"artists"`
	Venue       *ExtractedVenue   `json:"venue,omitempty"`
	Date        string            `json:"date,omitempty"`
	Time        string            `json:"time,omitempty"`
	Cost        string            `json:"cost,omitempty"`
	Ages        string            `json:"ages,omitempty"`
	Description string            `json:"description,omitempty"`
}

// ExtractShowResponse is the API response wrapper
type ExtractShowResponse struct {
	Success  bool               `json:"success"`
	Data     *ExtractedShowData `json:"data,omitempty"`
	Error    string             `json:"error,omitempty"`
	Warnings []string           `json:"warnings,omitempty"`
}

// ExtractShow processes text or image input through Claude and matches against the database
func (s *ExtractionService) ExtractShow(req *ExtractShowRequest) (*ExtractShowResponse, error) {
	if s.config.Anthropic.APIKey == "" {
		return &ExtractShowResponse{
			Success: false,
			Error:   "AI service not configured",
		}, nil
	}

	// Validate request
	validMediaTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}

	switch req.Type {
	case "text":
		if strings.TrimSpace(req.Text) == "" {
			return &ExtractShowResponse{Success: false, Error: "Text content is required"}, nil
		}
		if len(req.Text) > 10000 {
			return &ExtractShowResponse{Success: false, Error: "Text content exceeds maximum length of 10,000 characters"}, nil
		}
	case "image", "both":
		if req.ImageData == "" {
			return &ExtractShowResponse{Success: false, Error: "Image data is required"}, nil
		}
		if req.MediaType == "" {
			return &ExtractShowResponse{Success: false, Error: "Image media type is required"}, nil
		}
		if !validMediaTypes[req.MediaType] {
			return &ExtractShowResponse{Success: false, Error: "Invalid image type. Supported formats: image/jpeg, image/png, image/gif, image/webp"}, nil
		}
		if req.Type == "both" && len(req.Text) > 10000 {
			return &ExtractShowResponse{Success: false, Error: "Text content exceeds maximum length of 10,000 characters"}, nil
		}
	default:
		return &ExtractShowResponse{Success: false, Error: "Invalid request type. Use \"text\", \"image\", or \"both\""}, nil
	}

	// Build the Anthropic API request
	userContent := s.buildUserContent(req)

	// Call Claude
	responseText, err := s.callAnthropic(userContent)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "credit") || strings.Contains(strings.ToLower(err.Error()), "billing") {
			return &ExtractShowResponse{Success: false, Error: "AI service temporarily unavailable. Please try again later."}, nil
		}
		return &ExtractShowResponse{Success: false, Error: "AI service error. Please try again."}, nil
	}

	// Parse the JSON response from Claude
	parsed := parseExtractionResponse(responseText)
	if parsed == nil {
		return &ExtractShowResponse{
			Success:  false,
			Error:    "Failed to parse AI response",
			Warnings: []string{"The AI response could not be parsed as JSON. Please try again."},
		}, nil
	}

	warnings := []string{}

	// Extract and match artists
	rawArtists := extractRawArtists(parsed)
	if len(rawArtists) == 0 {
		warnings = append(warnings, "No artists were found in the input")
	}
	matchedArtists := s.matchArtists(rawArtists)

	// Track match statistics for warnings
	matchedCount := 0
	suggestedCount := 0
	for _, a := range matchedArtists {
		if a.MatchedID != nil {
			matchedCount++
		} else if len(a.Suggestions) > 0 {
			suggestedCount++
		}
	}
	newCount := len(matchedArtists) - matchedCount - suggestedCount
	if newCount > 0 || suggestedCount > 0 {
		parts := []string{}
		if matchedCount > 0 {
			parts = append(parts, fmt.Sprintf("%d matched", matchedCount))
		}
		if suggestedCount > 0 {
			parts = append(parts, fmt.Sprintf("%d with suggestions", suggestedCount))
		}
		if newCount > 0 {
			parts = append(parts, fmt.Sprintf("%d new", newCount))
		}
		warnings = append(warnings, fmt.Sprintf("Artists: %s", strings.Join(parts, ", ")))
	}

	// Extract and match venue
	matchedVenue := s.matchVenue(parsed)
	if matchedVenue != nil && matchedVenue.MatchedID == nil {
		if len(matchedVenue.Suggestions) > 0 {
			warnings = append(warnings, fmt.Sprintf("Venue \"%s\" not found — similar venues available", matchedVenue.Name))
		} else {
			warnings = append(warnings, fmt.Sprintf("Venue \"%s\" will be created as new", matchedVenue.Name))
		}
	}

	// Build the extracted data
	data := &ExtractedShowData{
		Artists: matchedArtists,
		Venue:   matchedVenue,
	}
	if dateStr, ok := parsed["date"].(string); ok {
		data.Date = dateStr
	}
	if timeStr, ok := parsed["time"].(string); ok {
		data.Time = timeStr
	}
	if costStr, ok := parsed["cost"].(string); ok {
		data.Cost = costStr
	}
	if agesStr, ok := parsed["ages"].(string); ok {
		data.Ages = agesStr
	}
	if descStr, ok := parsed["description"].(string); ok {
		data.Description = descStr
	}

	resp := &ExtractShowResponse{
		Success: true,
		Data:    data,
	}
	if len(warnings) > 0 {
		resp.Warnings = warnings
	}

	return resp, nil
}

// buildUserContent constructs the Anthropic API message content
func (s *ExtractionService) buildUserContent(req *ExtractShowRequest) []interface{} {
	switch req.Type {
	case "text":
		return []interface{}{
			map[string]string{
				"type": "text",
				"text": req.Text,
			},
		}
	case "image":
		return []interface{}{
			map[string]interface{}{
				"type": "image",
				"source": map[string]string{
					"type":       "base64",
					"media_type": req.MediaType,
					"data":       req.ImageData,
				},
			},
			map[string]string{
				"type": "text",
				"text": "Extract show information from this flyer image.",
			},
		}
	default: // "both"
		contextText := "Extract show information from this flyer image."
		if strings.TrimSpace(req.Text) != "" {
			contextText = fmt.Sprintf("Extract show information from this flyer image. Additional context from user: %s", req.Text)
		}
		return []interface{}{
			map[string]interface{}{
				"type": "image",
				"source": map[string]string{
					"type":       "base64",
					"media_type": req.MediaType,
					"data":       req.ImageData,
				},
			},
			map[string]string{
				"type": "text",
				"text": contextText,
			},
		}
	}
}

// anthropicRequest is the Anthropic API request structure
type anthropicRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// callAnthropic sends a request to the Anthropic API
func (s *ExtractionService) callAnthropic(userContent []interface{}) (string, error) {
	reqBody := anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 1024,
		System:    extractionSystemPrompt,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: userContent,
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", s.anthropicBaseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.config.Anthropic.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Anthropic API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("Anthropic API error: %s", apiResp.Error.Message)
	}

	// Extract text from response content blocks
	var responseText string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	return responseText, nil
}

// parseExtractionResponse tries to parse Claude's response as JSON
func parseExtractionResponse(text string) map[string]interface{} {
	// Try direct JSON parse
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err == nil {
		return result
	}

	// Try to extract JSON from markdown code block
	codeBlockRe := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")
	if matches := codeBlockRe.FindStringSubmatch(text); len(matches) > 1 {
		if err := json.Unmarshal([]byte(strings.TrimSpace(matches[1])), &result); err == nil {
			return result
		}
	}

	// Try to find a JSON object in the response
	objectRe := regexp.MustCompile(`(?s)\{.*\}`)
	if match := objectRe.FindString(text); match != "" {
		if err := json.Unmarshal([]byte(match), &result); err == nil {
			return result
		}
	}

	return nil
}

// rawArtist is the intermediate type from Claude's JSON output
type rawArtist struct {
	Name        string `json:"name"`
	IsHeadliner bool   `json:"is_headliner"`
}

// extractRawArtists extracts the artists array from parsed JSON
func extractRawArtists(parsed map[string]interface{}) []rawArtist {
	artistsRaw, ok := parsed["artists"]
	if !ok {
		return nil
	}

	artistsArray, ok := artistsRaw.([]interface{})
	if !ok {
		return nil
	}

	var artists []rawArtist
	for _, item := range artistsArray {
		artistMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := artistMap["name"].(string)
		isHeadliner, _ := artistMap["is_headliner"].(bool)
		if name != "" {
			artists = append(artists, rawArtist{Name: name, IsHeadliner: isHeadliner})
		}
	}

	return artists
}

// matchArtists matches extracted artists against the database
func (s *ExtractionService) matchArtists(rawArtists []rawArtist) []ExtractedArtist {
	var matched []ExtractedArtist

	for _, raw := range rawArtists {
		result := ExtractedArtist{
			Name:        raw.Name,
			IsHeadliner: raw.IsHeadliner,
		}

		// Search for the artist in the database
		searchResults, err := s.artistService.SearchArtists(raw.Name)
		if err == nil && len(searchResults) > 0 {
			// Look for exact match (case-insensitive)
			var exactMatch *ArtistDetailResponse
			for _, a := range searchResults {
				if strings.EqualFold(a.Name, raw.Name) {
					exactMatch = a
					break
				}
			}

			if exactMatch != nil {
				result.MatchedID = &exactMatch.ID
				result.MatchedName = &exactMatch.Name
				result.MatchedSlug = &exactMatch.Slug
			} else {
				// No exact match — include top 3 as suggestions
				limit := 3
				if len(searchResults) < limit {
					limit = len(searchResults)
				}
				suggestions := make([]MatchSuggestion, limit)
				for i := 0; i < limit; i++ {
					suggestions[i] = MatchSuggestion{
						ID:   searchResults[i].ID,
						Name: searchResults[i].Name,
						Slug: searchResults[i].Slug,
					}
				}
				result.Suggestions = suggestions
			}
		}

		matched = append(matched, result)
	}

	return matched
}

// matchVenue matches the extracted venue against the database
func (s *ExtractionService) matchVenue(parsed map[string]interface{}) *ExtractedVenue {
	venueRaw, ok := parsed["venue"]
	if !ok {
		return nil
	}

	venueMap, ok := venueRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	name, _ := venueMap["name"].(string)
	if name == "" {
		return nil
	}

	city, _ := venueMap["city"].(string)
	state, _ := venueMap["state"].(string)

	result := &ExtractedVenue{
		Name:  name,
		City:  city,
		State: state,
	}

	// Search for the venue in the database
	searchResults, err := s.venueService.SearchVenues(name)
	if err == nil && len(searchResults) > 0 {
		// Look for exact match (case-insensitive)
		var exactMatch *VenueDetailResponse
		for _, v := range searchResults {
			if strings.EqualFold(v.Name, name) {
				exactMatch = v
				break
			}
		}

		if exactMatch != nil {
			result.MatchedID = &exactMatch.ID
			result.MatchedName = &exactMatch.Name
			result.MatchedSlug = &exactMatch.Slug
			result.City = exactMatch.City
			result.State = exactMatch.State
		} else {
			// No exact match — include top 3 as suggestions
			limit := 3
			if len(searchResults) < limit {
				limit = len(searchResults)
			}
			suggestions := make([]VenueMatchSuggestion, limit)
			for i := 0; i < limit; i++ {
				suggestions[i] = VenueMatchSuggestion{
					ID:    searchResults[i].ID,
					Name:  searchResults[i].Name,
					Slug:  searchResults[i].Slug,
					City:  searchResults[i].City,
					State: searchResults[i].State,
				}
			}
			result.Suggestions = suggestions
		}
	}

	return result
}
