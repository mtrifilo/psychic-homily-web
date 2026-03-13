package pipeline

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
	"psychic-homily-backend/internal/services/contracts"
)

const extractionSystemPrompt = `You are a show information extractor. Given text or an image of a show flyer, extract structured information.

Output ONLY valid JSON with no additional text or markdown formatting:
{
  "artists": [{"name": "Artist Name", "set_type": "headliner", "billing_order": 1, "instagram_handle": "@handle"}],
  "venue": {"name": "Venue Name", "city": "City", "state": "AZ"},
  "date": "YYYY-MM-DD",
  "time": "HH:MM",
  "cost": "$20",
  "ages": "21+"
}

Rules:
- Determine billing position from visual prominence, text size, and ordering. The first/largest name is typically the headliner
- set_type values: "headliner" (top of bill), "support" (direct support act, indicated by "w/" or "with"), "opener" (opening act), "special_guest" (indicated by "special guest" or "featuring"), "dj" (DJ set), "host" (event host/MC)
- billing_order: 1 = top of bill, 2 = second, etc. Assign based on prominence/position
- For instagram_handle, extract Instagram handles (like @username) when visible on the flyer or in the text. Include the @ prefix. Omit if not found.
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
	artistService    artistSearcher
	venueService     venueSearcher
	httpClient       *http.Client
	anthropicBaseURL string
}

// artistSearcher is the subset of ArtistService used by ExtractionService.
type artistSearcher interface {
	SearchArtists(query string) ([]*contracts.ArtistDetailResponse, error)
}

// venueSearcher is the subset of VenueService used by ExtractionService.
type venueSearcher interface {
	SearchVenues(query string) ([]*contracts.VenueDetailResponse, error)
}

// NewExtractionService creates a new extraction service.
// It accepts a *gorm.DB to instantiate internal artist/venue search helpers.
func NewExtractionService(database *gorm.DB, cfg *config.Config, artistSvc artistSearcher, venueSvc venueSearcher) *ExtractionService {
	return &ExtractionService{
		config:           cfg,
		artistService:    artistSvc,
		venueService:     venueSvc,
		httpClient:       &http.Client{},
		anthropicBaseURL: "https://api.anthropic.com",
	}
}

// ExtractShow processes text or image input through Claude and matches against the database
func (s *ExtractionService) ExtractShow(req *contracts.ExtractShowRequest) (*contracts.ExtractShowResponse, error) {
	if s.config.Anthropic.APIKey == "" {
		return &contracts.ExtractShowResponse{
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
			return &contracts.ExtractShowResponse{Success: false, Error: "Text content is required"}, nil
		}
		if len(req.Text) > 10000 {
			return &contracts.ExtractShowResponse{Success: false, Error: "Text content exceeds maximum length of 10,000 characters"}, nil
		}
	case "image", "both":
		if req.ImageData == "" {
			return &contracts.ExtractShowResponse{Success: false, Error: "Image data is required"}, nil
		}
		if req.MediaType == "" {
			return &contracts.ExtractShowResponse{Success: false, Error: "Image media type is required"}, nil
		}
		if !validMediaTypes[req.MediaType] {
			return &contracts.ExtractShowResponse{Success: false, Error: "Invalid image type. Supported formats: image/jpeg, image/png, image/gif, image/webp"}, nil
		}
		if req.Type == "both" && len(req.Text) > 10000 {
			return &contracts.ExtractShowResponse{Success: false, Error: "Text content exceeds maximum length of 10,000 characters"}, nil
		}
	default:
		return &contracts.ExtractShowResponse{Success: false, Error: "Invalid request type. Use \"text\", \"image\", or \"both\""}, nil
	}

	// Build the Anthropic API request
	userContent := s.buildUserContent(req)

	// Call Claude
	responseText, err := s.callAnthropic(userContent)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "credit") || strings.Contains(strings.ToLower(err.Error()), "billing") {
			return &contracts.ExtractShowResponse{Success: false, Error: "AI service temporarily unavailable. Please try again later."}, nil
		}
		return &contracts.ExtractShowResponse{Success: false, Error: "AI service error. Please try again."}, nil
	}

	// Parse the JSON response from Claude
	parsed := parseExtractionResponse(responseText)
	if parsed == nil {
		return &contracts.ExtractShowResponse{
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
	data := &contracts.ExtractedShowData{
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

	resp := &contracts.ExtractShowResponse{
		Success: true,
		Data:    data,
	}
	if len(warnings) > 0 {
		resp.Warnings = warnings
	}

	return resp, nil
}

// buildUserContent constructs the Anthropic API message content
func (s *ExtractionService) buildUserContent(req *contracts.ExtractShowRequest) []interface{} {
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
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
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

// callAnthropic sends a request to the Anthropic API using the default extraction system prompt.
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

	return s.sendAnthropicRequest(reqBody)
}

// sendAnthropicRequest sends a pre-built request to the Anthropic API and returns the response text.
func (s *ExtractionService) sendAnthropicRequest(reqBody anthropicRequest) (string, error) {
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
	Name            string `json:"name"`
	IsHeadliner     bool   `json:"is_headliner"`
	SetType         string `json:"set_type"`
	BillingOrder    int    `json:"billing_order"`
	InstagramHandle string `json:"instagram_handle"`
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
		setType, _ := artistMap["set_type"].(string)
		billingOrder := 0
		if bo, ok := artistMap["billing_order"].(float64); ok {
			billingOrder = int(bo)
		}
		instagramHandle, _ := artistMap["instagram_handle"].(string)

		// Derive IsHeadliner from SetType if set_type is present but is_headliner was not explicitly set
		if setType == "headliner" && !isHeadliner {
			isHeadliner = true
		}

		if name != "" {
			artists = append(artists, rawArtist{
				Name:            name,
				IsHeadliner:     isHeadliner,
				SetType:         setType,
				BillingOrder:    billingOrder,
				InstagramHandle: instagramHandle,
			})
		}
	}

	return artists
}

// matchArtists matches extracted artists against the database
func (s *ExtractionService) matchArtists(rawArtists []rawArtist) []contracts.ExtractedArtist {
	var matched []contracts.ExtractedArtist

	for _, raw := range rawArtists {
		result := contracts.ExtractedArtist{
			Name:            raw.Name,
			IsHeadliner:     raw.IsHeadliner,
			SetType:         raw.SetType,
			BillingOrder:    raw.BillingOrder,
			InstagramHandle: raw.InstagramHandle,
		}

		// Search for the artist in the database
		searchResults, err := s.artistService.SearchArtists(raw.Name)
		if err == nil && len(searchResults) > 0 {
			// Look for exact match (case-insensitive)
			var exactMatch *contracts.ArtistDetailResponse
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
				// Clear instagram handle for matched artists — their socials are managed via artist edit
				result.InstagramHandle = ""
			} else {
				// No exact match — include top 3 as suggestions
				limit := 3
				if len(searchResults) < limit {
					limit = len(searchResults)
				}
				suggestions := make([]contracts.MatchSuggestion, limit)
				for i := 0; i < limit; i++ {
					suggestions[i] = contracts.MatchSuggestion{
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
func (s *ExtractionService) matchVenue(parsed map[string]interface{}) *contracts.ExtractedVenue {
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

	result := &contracts.ExtractedVenue{
		Name:  name,
		City:  city,
		State: state,
	}

	// Search for the venue in the database
	searchResults, err := s.venueService.SearchVenues(name)
	if err == nil && len(searchResults) > 0 {
		// Look for exact match (case-insensitive)
		var exactMatch *contracts.VenueDetailResponse
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
			suggestions := make([]contracts.VenueMatchSuggestion, limit)
			for i := 0; i < limit; i++ {
				suggestions[i] = contracts.VenueMatchSuggestion{
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
