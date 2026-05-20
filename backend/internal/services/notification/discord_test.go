package notification

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

func stringPtr(s string) *string { return &s }

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
			assert.Equal(t, tt.want, HashEmail(tt.email))
		})
	}
}

// buildUserName is a thin wrapper over the canonical shared.ResolveUserName
// chain (username → first/last → email-prefix), substituting the Discord-only
// "Not provided" label for the chain's anonymous sentinel. Fixtures set ID: 1
// because the canonical resolver short-circuits ID-0 users to anonymous, and
// in production every user reaching this path comes from the DB with a real ID.
func TestBuildUserName(t *testing.T) {
	tests := []struct {
		name string
		user *authm.User
		want string
	}{
		// Canonical chain: username wins over first/last and over email.
		{"prefers username", &authm.User{ID: 1, Username: stringPtr("dj_cool"), FirstName: stringPtr("John"), LastName: stringPtr("Doe"), Email: stringPtr("john@example.com")}, "dj_cool"},
		{"full name when no username", &authm.User{ID: 1, FirstName: stringPtr("John"), LastName: stringPtr("Doe")}, "John Doe"},
		{"first only", &authm.User{ID: 1, FirstName: stringPtr("Jane")}, "Jane"},
		// Email is only ever shown as its local-part (never the full address).
		{"email prefix, not full email", &authm.User{ID: 1, Email: stringPtr("solo@example.com")}, "solo"},
		// Chain bottoms out → Discord-specific terminal label.
		{"neither", &authm.User{ID: 1}, "Not provided"},
		{"empty strings", &authm.User{ID: 1, FirstName: stringPtr(""), LastName: stringPtr("")}, "Not provided"},
		{"nil user", nil, "Not provided"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildUserName(tt.user)
			assert.Equal(t, tt.want, got)
			// Regression guard for PSY-760: a raw email address must never
			// surface as the display string outside the canonical chain.
			if tt.user != nil && tt.user.Email != nil {
				assert.NotContains(t, got, "@", "buildUserName must not leak a raw email")
			}
		})
	}
}

func TestBuildVenueList(t *testing.T) {
	tests := []struct {
		name   string
		venues []contracts.VenueResponse
		want   string
	}{
		{"no venues", nil, "N/A"},
		{"empty slice", []contracts.VenueResponse{}, "N/A"},
		{"one venue", []contracts.VenueResponse{{Name: "The Crescent"}}, "The Crescent"},
		{"multiple venues", []contracts.VenueResponse{{Name: "Venue A"}, {Name: "Venue B"}, {Name: "Venue C"}}, "Venue A, Venue B, Venue C"},
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
		artists []contracts.ArtistResponse
		want    string
	}{
		{"no artists", nil, "N/A"},
		{"empty slice", []contracts.ArtistResponse{}, "N/A"},
		{"one artist, no headliner flag", []contracts.ArtistResponse{{Name: "Band"}}, "Band"},
		{"one headliner", []contracts.ArtistResponse{{Name: "Star", IsHeadliner: &headliner}}, "Star (headliner)"},
		{"mixed", []contracts.ArtistResponse{
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
	user := &authm.User{
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
	svc.NotifyNewUser(&authm.User{Email: &email})
	assertNoPayload(t, payloads)
}

func TestNotifyNewUser_NilUser(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyNewUser(nil)
	assertNoPayload(t, payloads)
}

// PSY-760: a username-less, name-less signup now resolves through the canonical
// chain's email-prefix step (the local-part only — never the full address). This
// replaced the old "Not provided" output for users that have an email on file.
func TestNotifyNewUser_NoName_FallsBackToEmailPrefix(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	email := "anon@example.com"
	user := &authm.User{ID: 1, Email: &email}

	svc.NotifyNewUser(user)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	nameField := payload.Embeds[0].Fields[2]
	assert.Equal(t, "Name", nameField.Name)
	assert.Equal(t, "anon", nameField.Value)
	assert.NotContains(t, nameField.Value, "@", "Discord embed must not leak a raw email")
}

// "Not provided" is still the terminal label when the canonical chain bottoms
// out entirely (no username, no name, no usable email).
func TestNotifyNewUser_NoIdentifiers_NotProvided(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	user := &authm.User{ID: 1}

	svc.NotifyNewUser(user)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	nameField := payload.Embeds[0].Fields[2]
	assert.Equal(t, "Name", nameField.Name)
	assert.Equal(t, "Not provided", nameField.Value)
}

func TestNotifyNewShow_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	headliner := true
	show := &contracts.ShowResponse{
		ID:        10,
		Title:     "Rock Night",
		EventDate: time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC),
		Status:    "pending",
		Venues:    []contracts.VenueResponse{{Name: "Valley Bar"}},
		Artists:   []contracts.ArtistResponse{{Name: "The Band", IsHeadliner: &headliner}},
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

	svc.NotifyNewShow(&contracts.ShowResponse{}, "test@test.com")
	assertNoPayload(t, payloads)
}

func TestNotifyNewShow_NilShow(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyNewShow(nil, "test@test.com")
	assertNoPayload(t, payloads)
}

func TestNotifyShowApproved_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	show := &contracts.ShowResponse{
		ID:        20,
		Title:     "Approved Gig",
		EventDate: time.Date(2026, 8, 1, 20, 0, 0, 0, time.UTC),
		Venues:    []contracts.VenueResponse{{Name: "Main Stage"}},
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

	svc.NotifyShowApproved(&contracts.ShowResponse{})
	assertNoPayload(t, payloads)
}

func TestNotifyShowApproved_NilShow(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyShowApproved(nil)
	assertNoPayload(t, payloads)
}

func TestNotifyShowRejected_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	show := &contracts.ShowResponse{
		ID:        30,
		Title:     "Bad Show",
		EventDate: time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC),
		Venues:    []contracts.VenueResponse{{Name: "Some Venue"}},
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

	svc.NotifyShowRejected(&contracts.ShowResponse{}, "reason")
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
	report := &communitym.ShowReport{
		ReportType: communitym.ShowReportTypeInaccurate,
		Details:    &details,
		Show: catalogm.Show{
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

	svc.NotifyShowReport(&communitym.ShowReport{}, "x@y.com")
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
	report := &communitym.ShowReport{
		ReportType: communitym.ShowReportTypeCancelled,
		Details:    &longDetails,
		Show:       catalogm.Show{Title: "Test"},
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

// =============================================================================
// NotifyArtistReport
// =============================================================================

func TestNotifyArtistReport_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	// Override to sync (not goroutine) for test determinism
	// Since NotifyArtistReport uses `go s.sendWebhook`, we use a channel-based approach
	details := "Wrong genre listed"
	report := &communitym.ArtistReport{
		ReportType: communitym.ArtistReportTypeInaccurate,
		Details:    &details,
		Artist: catalogm.Artist{
			Name: "Test Band",
		},
	}
	report.Artist.ID = 42

	svc.NotifyArtistReport(report, "reporter@test.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	assert.Contains(t, payload.Embeds[0].Title, "Test Band")
	assert.Equal(t, ColorOrange, payload.Embeds[0].Color)

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
	assert.Equal(t, "Wrong genre listed", detailsField)
}

func TestNotifyArtistReport_RemovalRequest(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	report := &communitym.ArtistReport{
		ReportType: communitym.ArtistReportTypeRemovalRequest,
		Artist:     catalogm.Artist{Name: "Remove Me"},
	}
	report.Artist.ID = 1

	svc.NotifyArtistReport(report, "r@t.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	var reportType string
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Report Type" {
			reportType = f.Value
		}
	}
	assert.Equal(t, "Removal Request", reportType)
}

func TestNotifyArtistReport_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}
	payloads := make(chan []byte, 1)

	svc.NotifyArtistReport(&communitym.ArtistReport{}, "x@y.com")
	assertNoPayload(t, payloads)
}

func TestNotifyArtistReport_NilReport(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyArtistReport(nil, "x@y.com")
	assertNoPayload(t, payloads)
}

func TestNotifyArtistReport_LongDetails(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	longDetails := strings.Repeat("x", 250)
	report := &communitym.ArtistReport{
		ReportType: communitym.ArtistReportTypeInaccurate,
		Details:    &longDetails,
		Artist:     catalogm.Artist{Name: "Test"},
	}
	report.Artist.ID = 1

	svc.NotifyArtistReport(report, "r@t.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	for _, f := range payload.Embeds[0].Fields {
		if f.Name == "Details" {
			assert.Len(t, f.Value, 200) // 197 chars + "..."
			assert.True(t, strings.HasSuffix(f.Value, "..."))
		}
	}
}

func TestNotifyArtistReport_UnknownArtist(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)
	report := &communitym.ArtistReport{
		ReportType: communitym.ArtistReportTypeInaccurate,
		Artist:     catalogm.Artist{}, // ID=0
	}

	svc.NotifyArtistReport(report, "r@t.com")

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	assert.Contains(t, payload.Embeds[0].Title, "Unknown Artist")
}

// =============================================================================
// NotifyNewRadioShows (PSY-671)
// =============================================================================

func TestNotifyNewRadioShows_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyNewRadioShows("WFMU", []string{"Three Chord Monte", "Bodega Pop", "Downtown Soulville"})

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	e := payload.Embeds[0]
	assert.Equal(t, "New Radio Shows: WFMU", e.Title)
	assert.Equal(t, "Discovered 3 new show(s)", e.Description)
	assert.Equal(t, ColorBlue, e.Color)
	require.Len(t, e.Fields, 1)
	assert.Equal(t, "Shows", e.Fields[0].Name)
	assert.Contains(t, e.Fields[0].Value, "Three Chord Monte")
	assert.Contains(t, e.Fields[0].Value, "Bodega Pop")
	assert.Contains(t, e.Fields[0].Value, "Downtown Soulville")
}

func TestNotifyNewRadioShows_EmptyList(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyNewRadioShows("WFMU", []string{})

	assertNoPayload(t, payloads)
}

func TestNotifyNewRadioShows_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}

	svc.NotifyNewRadioShows("WFMU", []string{"Show A"})
}

func TestNotifyNewRadioShows_CapsAtTwentyFive(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	names := make([]string, 30)
	for i := range names {
		names[i] = "Show " + string(rune('A'+i%26))
	}

	svc.NotifyNewRadioShows("WFMU", names)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	value := payload.Embeds[0].Fields[0].Value
	assert.Contains(t, value, "…and 5 more")
	assert.Equal(t, "Discovered 30 new show(s)", payload.Embeds[0].Description)
}

// =============================================================================
// NotifyBackfillCompleted (PSY-672)
// =============================================================================

func TestNotifyBackfillCompleted_Success(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyBackfillCompleted("WFMU", []string{"Three Chord Monte", "Bodega Pop"}, 26, 412)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	e := payload.Embeds[0]
	assert.Equal(t, "Backfill Complete: WFMU", e.Title)
	assert.Equal(t, "Backfilled 2 show(s) — 26 episodes, 412 plays matched", e.Description)
	assert.Equal(t, ColorGreen, e.Color)
	require.Len(t, e.Fields, 1)
	assert.Contains(t, e.Fields[0].Value, "Three Chord Monte")
	assert.Contains(t, e.Fields[0].Value, "Bodega Pop")
}

func TestNotifyBackfillCompleted_EmptyList(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	svc.NotifyBackfillCompleted("WFMU", []string{}, 0, 0)

	assertNoPayload(t, payloads)
}

func TestNotifyBackfillCompleted_NotConfigured(t *testing.T) {
	svc := &DiscordService{enabled: false}

	svc.NotifyBackfillCompleted("WFMU", []string{"Show A"}, 5, 50)
}

func TestNotifyBackfillCompleted_CapsAtTwentyFive(t *testing.T) {
	svc, payloads, _ := setupDiscordTest(t)

	names := make([]string, 30)
	for i := range names {
		names[i] = "Show " + string(rune('A'+i%26))
	}

	svc.NotifyBackfillCompleted("WFMU", names, 390, 8000)

	raw := waitForPayload(t, payloads)
	payload := parseWebhookPayload(t, raw)
	require.Len(t, payload.Embeds, 1)
	assert.Contains(t, payload.Embeds[0].Fields[0].Value, "…and 5 more")
	assert.Contains(t, payload.Embeds[0].Description, "30 show(s)")
}
