package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// Discord embed colors
const (
	ColorGreen  = 0x00FF00 // New user signups, show approved
	ColorBlue   = 0x0066FF // New show submissions
	ColorOrange = 0xFFA500 // Status changes (unpublish/publish/make-private)
	ColorRed    = 0xFF0000 // Show rejected
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
	webhookURL string
	enabled    bool
	httpClient *http.Client
}

// NewDiscordService creates a new Discord notification service
func NewDiscordService(cfg *config.Config) *DiscordService {
	return &DiscordService{
		webhookURL: cfg.Discord.WebhookURL,
		enabled:    cfg.Discord.Enabled,
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

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("New Show: %s", show.Title),
		Description: fmt.Sprintf("Event Date: %s", show.EventDate.Format("Jan 2, 2006 3:04 PM")),
		Color:       ColorBlue,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields: []DiscordEmbedField{
			{Name: "Show ID", Value: fmt.Sprintf("%d", show.ID), Inline: true},
			{Name: "Status", Value: show.Status, Inline: true},
			{Name: "Submitter", Value: hashEmail(submitterEmail), Inline: true},
			{Name: "Venue(s)", Value: venues, Inline: false},
			{Name: "Artist(s)", Value: artists, Inline: false},
		},
	}

	go s.sendWebhook(embed)
}

// NotifyShowStatusChange sends a notification when a show's status changes
func (s *DiscordService) NotifyShowStatusChange(showTitle string, showID uint, oldStatus, newStatus, actorEmail string) {
	if !s.IsConfigured() {
		return
	}

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("Show Status Changed: %s", showTitle),
		Description: fmt.Sprintf("%s → %s", oldStatus, newStatus),
		Color:       ColorOrange,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields: []DiscordEmbedField{
			{Name: "Show ID", Value: fmt.Sprintf("%d", showID), Inline: true},
			{Name: "Changed By", Value: hashEmail(actorEmail), Inline: true},
		},
	}

	go s.sendWebhook(embed)
}

// NotifyShowApproved sends a notification when an admin approves a show
func (s *DiscordService) NotifyShowApproved(show *ShowResponse) {
	if !s.IsConfigured() || show == nil {
		return
	}

	venues := buildVenueList(show.Venues)

	embed := DiscordEmbed{
		Title:     fmt.Sprintf("Show Approved: %s", show.Title),
		Color:     ColorGreen,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fields: []DiscordEmbedField{
			{Name: "Show ID", Value: fmt.Sprintf("%d", show.ID), Inline: true},
			{Name: "Event Date", Value: show.EventDate.Format("Jan 2, 2006"), Inline: true},
			{Name: "Venue(s)", Value: venues, Inline: false},
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

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("Show Rejected: %s", show.Title),
		Description: fmt.Sprintf("Reason: %s", reason),
		Color:       ColorRed,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields: []DiscordEmbedField{
			{Name: "Show ID", Value: fmt.Sprintf("%d", show.ID), Inline: true},
			{Name: "Event Date", Value: show.EventDate.Format("Jan 2, 2006"), Inline: true},
			{Name: "Venue(s)", Value: venues, Inline: false},
		},
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
		log.Printf("[Discord] Failed to marshal payload: %v", err)
		return
	}

	resp, err := s.httpClient.Post(s.webhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("[Discord] Failed to send webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[Discord] Webhook returned non-2xx status: %d", resp.StatusCode)
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
