package pipeline

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/contracts"
)

func modelReply(w http.ResponseWriter, text string) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"content": []map[string]string{{"type": "text", "text": text}},
	})
}

func TestExtractVenueAddress_SendsVenueAndPageAndParsesTheAddress(t *testing.T) {
	var sent anthropicRequest
	svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sent)
		modelReply(w, `{"venue_address": {"street": "2424 N Lincoln Ave", "city": "Chicago", "state": "IL", "postal_code": "60614"}}`)
	})
	defer server.Close()

	got, err := svc.ExtractVenueAddress(context.Background(), contracts.VenueAddressExtractionRequest{
		VenueName: "Lincoln Hall", City: "Chicago", State: "IL", PageURL: "https://lh-st.com/", PageText: "Lincoln Hall 2424 N Lincoln Ave",
	})
	if err != nil {
		t.Fatalf("ExtractVenueAddress: %v", err)
	}
	want := contracts.VenueAddressExtraction{Found: true, Street: "2424 N Lincoln Ave", City: "Chicago", State: "IL", PostalCode: "60614"}
	if *got != want {
		t.Fatalf("got %+v, want %+v", *got, want)
	}
	if sent.System != venueAddressSystemPrompt || sent.Model != extractionModel {
		t.Fatalf("request used system/model %q / %q", sent.System, sent.Model)
	}
	user, _ := json.Marshal(sent.Messages)
	for _, part := range []string{"Venue: Lincoln Hall", "City: Chicago, IL", "Page URL: https://lh-st.com/", "2424 N Lincoln Ave"} {
		if !strings.Contains(string(user), part) {
			t.Errorf("user message lacks %q: %s", part, user)
		}
	}
}

func TestParseVenueAddressResponse(t *testing.T) {
	tests := []struct {
		name    string
		reply   string
		want    contracts.VenueAddressExtraction
		wantErr bool
	}{
		{name: "null is not found", reply: `{"venue_address": null}`},
		{name: "absent is not found", reply: `{}`},
		{name: "no street is not found", reply: `{"venue_address": {"city": "Chicago"}}`},
		{name: "fenced JSON", reply: "```json\n{\"venue_address\": {\"street\": \" 7 S Broadway \"}}\n```", want: contracts.VenueAddressExtraction{Found: true, Street: "7 S Broadway"}},
		{name: "prose is an error", reply: "I could not find it.", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVenueAddressResponse(tt.reply)
			if tt.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if err != nil || *got != tt.want {
				t.Fatalf("got %+v err=%v, want %+v", got, err, tt.want)
			}
		})
	}
}

func TestExtractVenueAddress_NoKeyIsAnError(t *testing.T) {
	svc := &ExtractionService{config: &config.Config{}}
	if _, err := svc.ExtractVenueAddress(context.Background(), contracts.VenueAddressExtractionRequest{}); err == nil {
		t.Fatal("want an error without an API key")
	}
}

func TestExtractVenueAddress_APIErrorIsAnError(t *testing.T) {
	svc, server := newTestExtractionService(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer server.Close()
	if _, err := svc.ExtractVenueAddress(context.Background(), contracts.VenueAddressExtractionRequest{PageText: "x"}); err == nil {
		t.Fatal("want an error for a failed call")
	}
}
