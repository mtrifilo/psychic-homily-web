package handlers

import (
	"context"
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
	Date  string `json:"date" example:"2025-01-01" doc:"Date of the show"`
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
	// TODO: Implement show submission logic
	resp.Body.Success = true
	resp.Body.ShowID = "1234567890"
	return resp, nil
}
