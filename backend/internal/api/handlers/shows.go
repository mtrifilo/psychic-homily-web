package handlers

import (
	"context"
	"time"
)

// ShowSubmission represents a show submission request
type ShowSubmission struct {
	Artists []struct {
		ID        string `json:"id" example:"1234567890" doc:"ID of the artist"` // optional
		Name      string `json:"name" example:"Psychic Homily" doc:"Name of the artist"`
		Instagram string `json:"instagram" example:"psychichomily" doc:"Instagram handle of the artist"`
		Bandcamp  string `json:"bandcamp" example:"psychichomily" doc:"Bandcamp link for the artist"`
	} `json:"artists" example:"Psychic Homily, Flying Cows" doc:"Name of the artist"`
	Venue string `json:"venue" example:"The Great Hall" doc:"Name of the venue"`
	Date  string `json:"date" example:"2025-01-01T20:00:00Z" doc:"Date and time of the show in ISO 8601 format (UTC)"`
	Cost  string `json:"cost" example:"10" doc:"Cost of the show"`
	Ages  string `json:"ages" example:"18+" doc:"Ages allowed at the show"`
	City  string `json:"city" example:"New York" doc:"City of the show"`
	State string `json:"state" example:"NY" doc:"State of the show"`
}

// ShowSubmissionResponse represents a show submission response
type ShowSubmissionResponse struct {
	Body struct {
		Success bool   `json:"success" example:"true" doc:"Success of the show submission"`
		ShowID  string `json:"show_id" example:"1234567890" doc:"ID of the show"`
	}
}

// ShowSubmissionHandler handles show submission requests
func ShowSubmissionHandler(ctx context.Context, input *ShowSubmission) (*ShowSubmissionResponse, error) {
	resp := &ShowSubmissionResponse{}

	// Parse the date string as a UTC timestamp
	eventTime, err := time.Parse(time.RFC3339, input.Date)
	if err != nil {
		// Try parsing with different formats if RFC3339 fails
		formats := []string{
			"2006-01-02T15:04:05Z", // ISO 8601 with Z
			"2006-01-02T15:04:05",  // ISO 8601 without timezone
			"2006-01-02 15:04:05",  // Space separated
			"2006-01-02",           // Date only (assume midnight UTC)
		}

		for _, format := range formats {
			if eventTime, err = time.Parse(format, input.Date); err == nil {
				// If no timezone specified, assume UTC
				if format == "2006-01-02T15:04:05" || format == "2006-01-02 15:04:05" || format == "2006-01-02" {
					eventTime = eventTime.UTC()
				}
				break
			}
		}

		if err != nil {
			return nil, err
		}
	}

	// Ensure the time is in UTC
	eventTime = eventTime.UTC()

	// TODO: Implement show submission logic
	// Use eventTime for database insertion
	_ = eventTime // Placeholder to avoid unused variable warning

	resp.Body.Success = true
	resp.Body.ShowID = "1234567890"
	return resp, nil
}
