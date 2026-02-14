package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// Discord embed colors
const (
	ColorGreen  = 0x00FF00 // New user signups, show approved
	ColorBlue   = 0x0066FF // New show submissions
	ColorOrange = 0xFFA500 // Status changes (unpublish/publish/make-private)
	ColorRed    = 0xFF0000 // Show rejected
	ColorPurple = 0x9B59B6 // Venue needs verification
)

// DiscordEmbed represents a Discord embed object
type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

// DiscordEmbedField represents a field in a Discord embed
type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// DiscordWebhookPayload represents the payload sent to Discord webhooks
type DiscordWebhookPayload struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

// DiscordService handles sending notifications to Discord via webhooks
type DiscordService struct {
	webhookURL  string
	enabled     bool
	frontendURL string
	httpClient  *http.Client
}

// NewDiscordService creates a new Discord notification service
func NewDiscordService(cfg *config.Config) *DiscordService {
	return &DiscordService{
		webhookURL:  cfg.Discord.WebhookURL,
		enabled:     cfg.Discord.Enabled,
		frontendURL: cfg.Email.FrontendURL, // Reuse frontend URL from email config
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// IsConfigured returns true if the Discord service is properly configured
func (s *DiscordService) IsConfigured() bool {
	return s.enabled && s.webhookURL != ""
}

// NotifyNewUser sends a notification when a new user registers
func (s *DiscordService) NotifyNewUser(user *models.User) {
	if !s.IsConfigured() || user == nil {
		return
	}

	email := ""
	if user.Email != nil {
		email = hashEmail(*user.Email)
	}

	name := buildUserName(user)

	embed := DiscordEmbed{
		Title:     "New User Registration",
		Color:     ColorGreen,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []DiscordEmbedField{
			{Name: "User ID", Value: fmt.Sprintf("%d", user.ID), Inline: true},
			{Name: "Email", Value: email, Inline: true},
			{Name: "Name", Value: name, Inline: true},
		},
	}

	go s.sendWebhook(embed)
}

// NotifyNewShow sends a notification when a new show is submitted
func (s *DiscordService) NotifyNewShow(show *ShowResponse, submitterEmail string) {
	if !s.IsConfigured() || show == nil {
		return
	}

	venues := buildVenueList(show.Venues)
	artists := buildArtistList(show.Artists)

	fields := []DiscordEmbedField{
		{Name: "Show ID", Value: fmt.Sprintf("%d", show.ID), Inline: true},
		{Name: "Status", Value: show.Status, Inline: true},
		{Name: "Submitter", Value: hashEmail(submitterEmail), Inline: true},
		{Name: "Venue(s)", Value: venues, Inline: false},
		{Name: "Artist(s)", Value: artists, Inline: false},
	}

	// Add action links for pending shows
	if show.Status == "pending" {
		actions := fmt.Sprintf("[Review Pending Shows](%s/admin)", s.frontendURL)
		fields = append(fields, DiscordEmbedField{Name: "Actions", Value: actions, Inline: false})
	}

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("New Show: %s", show.Title),
		Description: fmt.Sprintf("Event Date: %s", show.EventDate.Format("Jan 2, 2006 3:04 PM")),
		Color:       ColorBlue,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields:      fields,
	}

	go s.sendWebhook(embed)
}

// NotifyShowStatusChange sends a notification when a show's status changes
func (s *DiscordService) NotifyShowStatusChange(showTitle string, showID uint, oldStatus, newStatus, actorEmail string) {
	if !s.IsConfigured() {
		return
	}

	fields := []DiscordEmbedField{
		{Name: "Show ID", Value: fmt.Sprintf("%d", showID), Inline: true},
		{Name: "Changed By", Value: hashEmail(actorEmail), Inline: true},
	}

	// Add action links based on new status
	if newStatus == "pending" {
		actions := fmt.Sprintf("[Review Pending Shows](%s/admin)", s.frontendURL)
		fields = append(fields, DiscordEmbedField{Name: "Actions", Value: actions, Inline: false})
	}

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("Show Status Changed: %s", showTitle),
		Description: fmt.Sprintf("%s → %s", oldStatus, newStatus),
		Color:       ColorOrange,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields:      fields,
	}

	go s.sendWebhook(embed)
}

// NotifyShowApproved sends a notification when an admin approves a show
func (s *DiscordService) NotifyShowApproved(show *ShowResponse) {
	if !s.IsConfigured() || show == nil {
		return
	}

	venues := buildVenueList(show.Venues)
	viewLink := fmt.Sprintf("[View on Calendar](%s)", s.frontendURL)

	embed := DiscordEmbed{
		Title:     fmt.Sprintf("Show Approved: %s", show.Title),
		Color:     ColorGreen,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []DiscordEmbedField{
			{Name: "Show ID", Value: fmt.Sprintf("%d", show.ID), Inline: true},
			{Name: "Event Date", Value: show.EventDate.Format("Jan 2, 2006"), Inline: true},
			{Name: "Venue(s)", Value: venues, Inline: false},
			{Name: "Actions", Value: viewLink, Inline: false},
		},
	}

	go s.sendWebhook(embed)
}

// NotifyShowRejected sends a notification when an admin rejects a show
func (s *DiscordService) NotifyShowRejected(show *ShowResponse, reason string) {
	if !s.IsConfigured() || show == nil {
		return
	}

	venues := buildVenueList(show.Venues)
	adminLink := fmt.Sprintf("[View Admin Panel](%s/admin)", s.frontendURL)

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("Show Rejected: %s", show.Title),
		Description: fmt.Sprintf("Reason: %s", reason),
		Color:       ColorRed,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields: []DiscordEmbedField{
			{Name: "Show ID", Value: fmt.Sprintf("%d", show.ID), Inline: true},
			{Name: "Event Date", Value: show.EventDate.Format("Jan 2, 2006"), Inline: true},
			{Name: "Venue(s)", Value: venues, Inline: false},
			{Name: "Actions", Value: adminLink, Inline: false},
		},
	}

	go s.sendWebhook(embed)
}

// NotifyShowReport sends a notification when a user reports a show issue
func (s *DiscordService) NotifyShowReport(report *models.ShowReport, reporterEmail string) {
	if !s.IsConfigured() || report == nil {
		return
	}

	// Format report type for display
	reportTypeDisplay := string(report.ReportType)
	switch report.ReportType {
	case models.ShowReportTypeCancelled:
		reportTypeDisplay = "Cancelled"
	case models.ShowReportTypeSoldOut:
		reportTypeDisplay = "Sold Out"
	case models.ShowReportTypeInaccurate:
		reportTypeDisplay = "Inaccurate Info"
	}

	showTitle := "Unknown Show"
	eventDate := "Unknown Date"
	if report.Show.ID != 0 {
		showTitle = report.Show.Title
		eventDate = report.Show.EventDate.Format("Jan 2, 2006")
	}

	fields := []DiscordEmbedField{
		{Name: "Report Type", Value: reportTypeDisplay, Inline: true},
		{Name: "Show", Value: showTitle, Inline: true},
		{Name: "Event Date", Value: eventDate, Inline: true},
		{Name: "Reporter", Value: hashEmail(reporterEmail), Inline: true},
	}

	// Add details if provided
	if report.Details != nil && *report.Details != "" {
		details := *report.Details
		if len(details) > 200 {
			details = details[:197] + "..."
		}
		fields = append(fields, DiscordEmbedField{Name: "Details", Value: details, Inline: false})
	}

	// Add action link
	actions := fmt.Sprintf("[Review Reports](%s/admin?tab=reports)", s.frontendURL)
	fields = append(fields, DiscordEmbedField{Name: "Actions", Value: actions, Inline: false})

	embed := DiscordEmbed{
		Title:     fmt.Sprintf("Show Report: %s", showTitle),
		Color:     ColorOrange,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields:    fields,
	}

	go s.sendWebhook(embed)
}

// NotifyArtistReport sends a notification when a user reports an artist issue
func (s *DiscordService) NotifyArtistReport(report *models.ArtistReport, reporterEmail string) {
	if !s.IsConfigured() || report == nil {
		return
	}

	// Format report type for display
	reportTypeDisplay := string(report.ReportType)
	switch report.ReportType {
	case models.ArtistReportTypeInaccurate:
		reportTypeDisplay = "Inaccurate Info"
	case models.ArtistReportTypeRemovalRequest:
		reportTypeDisplay = "Removal Request"
	}

	artistName := "Unknown Artist"
	if report.Artist.ID != 0 {
		artistName = report.Artist.Name
	}

	fields := []DiscordEmbedField{
		{Name: "Report Type", Value: reportTypeDisplay, Inline: true},
		{Name: "Artist", Value: artistName, Inline: true},
		{Name: "Reporter", Value: hashEmail(reporterEmail), Inline: true},
	}

	// Add details if provided
	if report.Details != nil && *report.Details != "" {
		details := *report.Details
		if len(details) > 200 {
			details = details[:197] + "..."
		}
		fields = append(fields, DiscordEmbedField{Name: "Details", Value: details, Inline: false})
	}

	// Add action link
	actions := fmt.Sprintf("[Review Reports](%s/admin?tab=reports)", s.frontendURL)
	fields = append(fields, DiscordEmbedField{Name: "Actions", Value: actions, Inline: false})

	embed := DiscordEmbed{
		Title:     fmt.Sprintf("Artist Report: %s", artistName),
		Color:     ColorOrange,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields:    fields,
	}

	go s.sendWebhook(embed)
}

// NotifyNewVenue sends a notification when a new unverified venue is created
func (s *DiscordService) NotifyNewVenue(venueID uint, venueName, city, state string, address *string, submitterEmail string) {
	if !s.IsConfigured() {
		return
	}

	location := city
	if state != "" {
		location = fmt.Sprintf("%s, %s", city, state)
	}

	fields := []DiscordEmbedField{
		{Name: "Venue ID", Value: fmt.Sprintf("%d", venueID), Inline: true},
		{Name: "Location", Value: location, Inline: true},
		{Name: "Submitted By", Value: hashEmail(submitterEmail), Inline: true},
	}

	if address != nil && *address != "" {
		fields = append(fields, DiscordEmbedField{Name: "Address", Value: *address, Inline: false})
	}

	// Add action link
	actions := fmt.Sprintf("[Review Venues](%s/admin?tab=venues)", s.frontendURL)
	fields = append(fields, DiscordEmbedField{Name: "Actions", Value: actions, Inline: false})

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("New Venue: %s", venueName),
		Description: "Needs verification",
		Color:       ColorPurple,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields:      fields,
	}

	go s.sendWebhook(embed)
}

// NotifyPendingVenueEdit sends a notification when a user submits a venue edit for review
func (s *DiscordService) NotifyPendingVenueEdit(editID, venueID uint, venueName, submitterEmail string) {
	if !s.IsConfigured() {
		return
	}

	fields := []DiscordEmbedField{
		{Name: "Venue ID", Value: fmt.Sprintf("%d", venueID), Inline: true},
		{Name: "Edit ID", Value: fmt.Sprintf("%d", editID), Inline: true},
		{Name: "Submitted By", Value: hashEmail(submitterEmail), Inline: true},
	}

	// Add action link
	actions := fmt.Sprintf("[Review Venue Edits](%s/admin?tab=venue-edits)", s.frontendURL)
	fields = append(fields, DiscordEmbedField{Name: "Actions", Value: actions, Inline: false})

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("Venue Edit: %s", venueName),
		Description: "Pending review",
		Color:       ColorPurple,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields:      fields,
	}

	go s.sendWebhook(embed)
}

// sendWebhook sends an embed to the Discord webhook (fire-and-forget)
func (s *DiscordService) sendWebhook(embed DiscordEmbed) {
	payload := DiscordWebhookPayload{
		Embeds: []DiscordEmbed{embed},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		sentry.CaptureException(fmt.Errorf("discord webhook marshal failed: %w", err))
		return
	}

	resp, err := s.httpClient.Post(s.webhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "discord")
			scope.SetExtra("embed_title", embed.Title)
			sentry.CaptureException(fmt.Errorf("discord webhook failed: %w", err))
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "discord")
			scope.SetExtra("status_code", resp.StatusCode)
			scope.SetExtra("embed_title", embed.Title)
			sentry.CaptureMessage(fmt.Sprintf("Discord webhook returned %d", resp.StatusCode))
		})
	}
}

// hashEmail masks an email for privacy (e.g., "jo***@example.com")
func hashEmail(email string) string {
	if email == "" {
		return "N/A"
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "N/A"
	}

	local := parts[0]
	domain := parts[1]

	if len(local) <= 2 {
		return local[:1] + "***@" + domain
	}

	return local[:2] + "***@" + domain
}

// buildUserName builds a display name from user fields
func buildUserName(user *models.User) string {
	if user == nil {
		return "N/A"
	}

	var parts []string
	if user.FirstName != nil && *user.FirstName != "" {
		parts = append(parts, *user.FirstName)
	}
	if user.LastName != nil && *user.LastName != "" {
		parts = append(parts, *user.LastName)
	}

	if len(parts) == 0 {
		return "Not provided"
	}

	return strings.Join(parts, " ")
}

// buildVenueList builds a comma-separated list of venue names
func buildVenueList(venues []VenueResponse) string {
	if len(venues) == 0 {
		return "N/A"
	}

	names := make([]string, len(venues))
	for i, v := range venues {
		names[i] = v.Name
	}

	return strings.Join(names, ", ")
}

// buildArtistList builds a comma-separated list of artist names
func buildArtistList(artists []ArtistResponse) string {
	if len(artists) == 0 {
		return "N/A"
	}

	names := make([]string, len(artists))
	for i, a := range artists {
		name := a.Name
		if a.IsHeadliner != nil && *a.IsHeadliner {
			name += " (headliner)"
		}
		names[i] = name
	}

	return strings.Join(names, ", ")
}
