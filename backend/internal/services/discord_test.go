package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// =============================================================================
// HELPERS
// =============================================================================

func setupDiscordTest(t *testing.T) (*DiscordService, chan []byte, *httptest.Server) {
	t.Helper()
	payloads := make(chan []byte, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		payloads <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	service := &DiscordService{
		webhookURL:  server.URL,
		enabled:     true,
		frontendURL: "http://localhost:3000",
		httpClient:  server.Client(),
	}
	return service, payloads, server
}

func waitForPayload(t *testing.T, ch chan []byte) []byte {
	t.Helper()
	select {
	case payload := <-ch:
		return payload
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook payload")
		return nil
	}
}

func assertNoPayload(t *testing.T, ch chan []byte) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("unexpected webhook payload received")
	case <-time.After(200 * time.Millisecond):
		// OK — no payload expected
	}
}

func parseWebhookPayload(t *testing.T, raw []byte) DiscordWebhookPayload {
	t.Helper()
	var payload DiscordWebhookPayload
	err := json.Unmarshal(raw, &payload)
	require.NoError(t, err, "failed to parse webhook payload JSON")
	return payload
}

// =============================================================================
// Constructor & IsConfigured
// =============================================================================

func TestNewDiscordService_Configured(t *testing.T) {
	cfg := &config.Config{
		Discord: config.DiscordConfig{
			WebhookURL: "https://discord.com/api/webhooks/123/abc",
			Enabled:    true,
		},
		Email: config.EmailConfig{
			FrontendURL: "http://localhost:3000",
		},
	}
	svc := NewDiscordService(cfg)

	assert.True(t, svc.enabled)
	assert.Equal(t, "https://discord.com/api/webhooks/123/abc", svc.webhookURL)
	assert.Equal(t, "http://localhost:3000", svc.frontendURL)
	assert.NotNil(t, svc.httpClient)
}

func TestNewDiscordService_NotConfigured(t *testing.T) {
	cfg := &config.Config{
		Discord: config.DiscordConfig{
			WebhookURL: "",
			Enabled:    false,
		},
	}
	svc := NewDiscordService(cfg)

	assert.False(t, svc.enabled)
	assert.Empty(t, svc.webhookURL)
}

func TestNewDiscordService_DefaultHTTPClient(t *testing.T) {
	cfg := &config.Config{
		Discord: config.DiscordConfig{
			WebhookURL: "https://example.com",
			Enabled:    true,
		},
	}
	svc := NewDiscordService(cfg)

	assert.NotNil(t, svc.httpClient)
	assert.Equal(t, 10*time.Second, svc.httpClient.Timeout)
}

func TestIsConfigured(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		webhookURL string
		want       bool
	}{
		{"enabled with URL", true, "https://example.com", true},
		{"disabled", false, "https://example.com", false},
		{"enabled but empty URL", true, "", false},
		{"disabled and empty URL", false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &DiscordService{enabled: tt.enabled, webhookURL: tt.webhookURL}
			assert.Equal(t, tt.want, svc.IsConfigured())
		})
	}
}

// =============================================================================
// Pure Helpers
// =============================================================================

func TestHashEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"normal email", "john@example.com", "jo***@example.com"},
		{"short local part", "ab@example.com", "a***@example.com"},
		{"single char local", "a@example.com", "a***@example.com"},
		{"empty string", "", "N/A"},
		{"no at sign", "invalid", "N/A"},
		{"long local part", "longname@domain.org", "lo***@domain.org"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hashEmail(tt.email))
		})
	}
}

func TestBuildUserName(t *testing.T) {
	tests := []struct {
		name string
		user *models.User
		want string
	}{
		{"full name", &models.User{FirstName: stringPtr("John"), LastName: stringPtr("Doe")}, "John Doe"},
		{"first only", &models.User{FirstName: stringPtr("Jane")}, "Jane"},
		{"last only", &models.User{LastName: stringPtr("Smith")}, "Smith"},
		{"neither", &models.User{}, "Not provided"},
		{"nil user", nil, "N/A"},
		{"empty strings", &models.User{FirstName: stringPtr(""), LastName: stringPtr("")}, "Not provided"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildUserName(tt.user))
		})
	}
}

func TestBuildVenueList(t *testing.T) {
	tests := []struct {
		name   string
		venues []VenueResponse
		want   string
	}{
		{"no venues", nil, "N/A"},
		{"empty slice", []VenueResponse{}, "N/A"},
		{"one venue", []VenueResponse{{Name: "The Crescent"}}, "The Crescent"},
		{"multiple venues", []VenueResponse{{Name: "Venue A"}, {Name: "Venue B"}, {Name: "Venue C"}}, "Venue A, Venue B, Venue C"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildVenueList(tt.venues))
		})
	}
}

func TestBuildArtistList(t *testing.T) {
	headliner := true
	notHeadliner := false
	tests := []struct {
		name    string
		artists []ArtistResponse
		want    string
	}{
		{"no artists", nil, "N/A"},
		{"empty slice", []ArtistResponse{}, "N/A"},
		{"one artist, no headliner flag", []ArtistResponse{{Name: "Band"}}, "Band"},
		{"one headliner", []ArtistResponse{{Name: "Star", IsHeadliner: &headliner}}, "Star (headliner)"},
		{"mixed", []ArtistResponse{
			{Name: "Headliner", IsHeadliner: &headliner},
			{Name: "Opener", IsHeadliner: &notHeadliner},
		}, "Headliner (headliner), Opener"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildArtistList(tt.artists))
		})
	}
}

// =============================================================================
// sendWebhook
// =============================================================================

func TestSendWebhook_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	embed := DiscordEmbed{
		Title: "Test Embed",
		Color: ColorBlue,
	}
	svc.sendWebhook(embed)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	assert.Equal(t, "Test Embed", payload.Embeds[0].Title)
	assert.Equal(t, ColorBlue, payload.Embeds[0].Color)
}

func TestSendWebhook_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	svc := &DiscordService{
		webhookURL:  server.URL,
		enabled:     true,
		frontendURL: "http://localhost:3000",
		httpClient:  server.Client(),
	}

	// Should not panic
	svc.sendWebhook(DiscordEmbed{Title: "Error Test"})
}

func TestSendWebhook_InvalidURL(t *testing.T) {
	svc := &DiscordService{
		webhookURL: "://invalid",
		enabled:    true,
		httpClient: &http.Client{Timeout: 1 * time.Second},
	}

	// Should not panic
	svc.sendWebhook(DiscordEmbed{Title: "Bad URL"})
}

func TestSendWebhook_PayloadStructure(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	embed := DiscordEmbed{
		Title:       "Structured Test",
		Description: "A description",
		Color:       ColorGreen,
		Fields: []DiscordEmbedField{
			{Name: "Field1", Value: "Value1", Inline: true},
		},
		Timestamp: "2026-01-15T12:00:00Z",
	}
	svc.sendWebhook(embed)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	e := payload.Embeds[0]
	assert.Equal(t, "Structured Test", e.Title)
	assert.Equal(t, "A description", e.Description)
	assert.Equal(t, ColorGreen, e.Color)
	require.Len(t, e.Fields, 1)
	assert.Equal(t, "Field1", e.Fields[0].Name)
	assert.Equal(t, "Value1", e.Fields[0].Value)
	assert.True(t, e.Fields[0].Inline)
	assert.Equal(t, "2026-01-15T12:00:00Z", e.Timestamp)
}

// =============================================================================
// Notification Methods
// =============================================================================

func TestNotifyNewUser_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	email := "user@example.com"
	user := &models.User{
		ID:        42,
		Email:     &email,
		FirstName: stringPtr("Jane"),
		LastName:  stringPtr("Doe"),
	}

	svc.NotifyNewUser(user)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	assert.Equal(t, "New User Registration", payload.Embeds[0].Title)
	assert.Equal(t, ColorGreen, payload.Embeds[0].Color)

	// Check fields
	fields := payload.Embeds[0].Fields
	assert.Equal(t, "42", fields[0].Value) // User ID
	assert.Equal(t, "us***@example.com", fields[1].Value)
	assert.Equal(t, "Jane Doe", fields[2].Value)
}

func TestNotifyNewUser_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	email := "test@test.com"
	svc.NotifyNewUser(&models.User{Email: &email})
	assertNoPayload(t, payloads)
}

func TestNotifyNewUser_NilUser(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyNewUser(nil)
	assertNoPayload(t, payloads)
}

func TestNotifyNewUser_NoName(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	email := "anon@example.com"
	user := &models.User{ID: 1, Email: &email}

	svc.NotifyNewUser(user)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	// Name field should say "Not provided"
	nameField := payload.Embeds[0].Fields[2]
	assert.Equal(t, "Name", nameField.Name)
	assert.Equal(t, "Not provided", nameField.Value)
}

func TestNotifyNewShow_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	headliner := true
	show := &ShowResponse{
		ID:        10,
		Title:     "Rock Night",
		EventDate: time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC),
		Status:    "pending",
		Venues:    []VenueResponse{{Name: "Valley Bar"}},
		Artists:   []ArtistResponse{{Name: "The Band", IsHeadliner: &headliner}},
	}

	svc.NotifyNewShow(show, "submitter@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	assert.Contains(t, payload.Embeds[0].Title, "Rock Night")
	assert.Equal(t, ColorBlue, payload.Embeds[0].Color)
	// Pending shows should have action link
	hasActions := false
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Actions" {
			hasActions = true
			assert.Contains(t, f.Value, "/admin")
		}
	}
	assert.True(t, hasActions, "pending show should have actions field")
}

func TestNotifyNewShow_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	svc.NotifyNewShow(&ShowResponse{}, "test@test.com")
	assertNoPayload(t, payloads)
}

func TestNotifyNewShow_NilShow(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyNewShow(nil, "test@test.com")
	assertNoPayload(t, payloads)
}

func TestNotifyShowApproved_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	show := &ShowResponse{
		ID:        20,
		Title:     "Approved Gig",
		EventDate: time.Date(2026, 8, 1, 20, 0, 0, 0, time.UTC),
		Venues:    []VenueResponse{{Name: "Main Stage"}},
	}

	svc.NotifyShowApproved(show)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	assert.Contains(t, payload.Embeds[0].Title, "Approved")
	assert.Equal(t, ColorGreen, payload.Embeds[0].Color)
}

func TestNotifyShowApproved_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	svc.NotifyShowApproved(&ShowResponse{})
	assertNoPayload(t, payloads)
}

func TestNotifyShowApproved_NilShow(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyShowApproved(nil)
	assertNoPayload(t, payloads)
}

func TestNotifyShowRejected_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	show := &ShowResponse{
		ID:        30,
		Title:     "Bad Show",
		EventDate: time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC),
		Venues:    []VenueResponse{{Name: "Some Venue"}},
	}

	svc.NotifyShowRejected(show, "Duplicate listing")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	assert.Contains(t, payload.Embeds[0].Title, "Rejected")
	assert.Contains(t, payload.Embeds[0].Description, "Duplicate listing")
	assert.Equal(t, ColorRed, payload.Embeds[0].Color)
}

func TestNotifyShowRejected_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	svc.NotifyShowRejected(&ShowResponse{}, "reason")
	assertNoPayload(t, payloads)
}

func TestNotifyShowRejected_NilShow(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyShowRejected(nil, "reason")
	assertNoPayload(t, payloads)
}

func TestNotifyShowStatusChange_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyShowStatusChange("Test Show", 5, "approved", "pending", "admin@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	assert.Contains(t, payload.Embeds[0].Title, "Status Changed")
	assert.Contains(t, payload.Embeds[0].Description, "approved → pending")
	assert.Equal(t, ColorOrange, payload.Embeds[0].Color)
	// Pending status should have action link
	hasActions := false
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Actions" {
			hasActions = true
		}
	}
	assert.True(t, hasActions)
}

func TestNotifyShowStatusChange_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	svc.NotifyShowStatusChange("Test", 1, "a", "b", "x@y.com")
	assertNoPayload(t, payloads)
}

func TestNotifyShowStatusChange_NonPendingNoActions(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyShowStatusChange("Test Show", 5, "pending", "approved", "admin@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	for _, f := range payload.Embeds[0].Fields {
		assert.NotEqual(t, "Actions", f.Name, "approved status should not have actions field")
	}
}

func TestNotifyShowReport_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	details := "Wrong date listed"
	report := &models.ShowReport{
		ReportType: models.ShowReportTypeInaccurate,
		Details:    &details,
		Show: models.Show{
			Title:     "Reported Show",
			EventDate: time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC),
		},
	}
	report.Show.ID = 99

	svc.NotifyShowReport(report, "reporter@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	assert.Contains(t, payload.Embeds[0].Title, "Reported Show")
	assert.Equal(t, ColorOrange, payload.Embeds[0].Color)
	// Check fields
	var reportType, detailsField string
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Report Type" {
			reportType = f.Value
		}
		if f.Name == "Details" {
			detailsField = f.Value
		}
	}
	assert.Equal(t, "Inaccurate Info", reportType)
	assert.Equal(t, "Wrong date listed", detailsField)
}

func TestNotifyShowReport_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	svc.NotifyShowReport(&models.ShowReport{}, "x@y.com")
	assertNoPayload(t, payloads)
}

func TestNotifyShowReport_NilReport(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyShowReport(nil, "x@y.com")
	assertNoPayload(t, payloads)
}

func TestNotifyShowReport_LongDetails(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	longDetails := ""
	for i := 0; i < 250; i++ {
		longDetails += "x"
	}
	report := &models.ShowReport{
		ReportType: models.ShowReportTypeCancelled,
		Details:    &longDetails,
		Show:       models.Show{Title: "Test"},
	}
	report.Show.ID = 1

	svc.NotifyShowReport(report, "r@t.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Details" {
			assert.Len(t, f.Value, 200) // 197 chars + "..."
			assert.True(t, f.Value[len(f.Value)-3:] == "...")
		}
	}
}

func TestNotifyNewVenue_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	addr := "123 Main St"

	svc.NotifyNewVenue(50, "New Club", "Phoenix", "AZ", &addr, "sub@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	assert.Contains(t, payload.Embeds[0].Title, "New Club")
	assert.Equal(t, "Needs verification", payload.Embeds[0].Description)
	assert.Equal(t, ColorPurple, payload.Embeds[0].Color)
	// Check location field
	var location, address string
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Location" {
			location = f.Value
		}
		if f.Name == "Address" {
			address = f.Value
		}
	}
	assert.Equal(t, "Phoenix, AZ", location)
	assert.Equal(t, "123 Main St", address)
}

func TestNotifyNewVenue_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	svc.NotifyNewVenue(1, "V", "C", "S", nil, "x@y.com")
	assertNoPayload(t, payloads)
}

func TestNotifyNewVenue_NoAddress(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyNewVenue(50, "No Addr Club", "Phoenix", "AZ", nil, "sub@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	for _, f := range payload.Embeds[0].Fields {
		assert.NotEqual(t, "Address", f.Name, "should not have address field when nil")
	}
}

func TestNotifyNewVenue_CityOnly(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyNewVenue(50, "City Club", "Phoenix", "", nil, "sub@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Location" {
			assert.Equal(t, "Phoenix", f.Value) // No state means no comma
		}
	}
}

func TestNotifyPendingVenueEdit_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyPendingVenueEdit(100, 50, "Updated Venue", "editor@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	assert.Contains(t, payload.Embeds[0].Title, "Updated Venue")
	assert.Equal(t, "Pending review", payload.Embeds[0].Description)
	assert.Equal(t, ColorPurple, payload.Embeds[0].Color)
	// Check fields
	var venueID, editID string
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Venue ID" {
			venueID = f.Value
		}
		if f.Name == "Edit ID" {
			editID = f.Value
		}
	}
	assert.Equal(t, "50", venueID)
	assert.Equal(t, "100", editID)
}

func TestNotifyPendingVenueEdit_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	svc.NotifyPendingVenueEdit(1, 1, "V", "x@y.com")
	assertNoPayload(t, payloads)
}
