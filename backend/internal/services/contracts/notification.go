package contracts

import (
	"context"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
)

// ──────────────────────────────────────────────
// Email types
// ──────────────────────────────────────────────

// CollectionDigestEntry describes a single item added to a subscribed
// collection for inclusion in the weekly digest email. PSY-350.
type CollectionDigestEntry struct {
	EntityType string
	EntityName string
	EntityURL  string
	AddedBy    string
}

// CollectionDigestGroup is one subscribed collection's worth of new items in
// a single user's weekly digest. PSY-350.
type CollectionDigestGroup struct {
	CollectionTitle string
	CollectionURL   string
	Items           []CollectionDigestEntry
}

// ──────────────────────────────────────────────
// Email Service Interface
// ──────────────────────────────────────────────

// EmailServiceInterface defines the contract for email operations.
type EmailServiceInterface interface {
	IsConfigured() bool
	SendVerificationEmail(toEmail, token string) error
	SendMagicLinkEmail(toEmail, token string) error
	SendAccountRecoveryEmail(toEmail, token string, daysRemaining int) error
	SendShowReminderEmail(toEmail, showTitle, showURL, unsubscribeURL string, eventDate time.Time, venues []string) error
	SendFilterNotificationEmail(toEmail, subject, htmlBody, unsubscribeURL string) error
	SendTierPromotionEmail(toEmail, username, oldTier, newTier, reason string, newPermissions []string) error
	SendTierDemotionEmail(toEmail, username, oldTier, newTier, reason string) error
	SendTierDemotionWarningEmail(toEmail, username, currentTier string, currentRate float64, threshold float64) error
	SendEditApprovedEmail(toEmail, username, entityType, entityName, entityURL string) error
	SendEditRejectedEmail(toEmail, username, entityType, entityName, rejectionReason string) error
	// PSY-289: comment + mention notifications.
	SendCommentNotification(toEmail, commenterName, entityType, entityName, commentExcerpt, entityURL, unsubscribeURL string) error
	SendMentionNotification(toEmail, mentionerName, entityType, entityName, commentExcerpt, commentURL, unsubscribeURL string) error
	// PSY-350: collection digest email — single batched email per user per
	// week grouping items added across all subscribed collections.
	SendCollectionDigestEmail(toEmail string, groups []CollectionDigestGroup, unsubscribeURL string) error
}

// ──────────────────────────────────────────────
// Reminder Service Interface
// ──────────────────────────────────────────────

// ReminderServiceInterface defines the contract for the show reminder background service.
type ReminderServiceInterface interface {
	Start(ctx context.Context)
	Stop()
	RunReminderCycleNow()
}

// ──────────────────────────────────────────────
// Discord Service Interface
// ──────────────────────────────────────────────

// DiscordServiceInterface defines the contract for Discord notification operations.
type DiscordServiceInterface interface {
	IsConfigured() bool
	NotifyNewUser(user *authm.User)
	NotifyNewShow(show *ShowResponse, submitterEmail string)
	NotifyShowStatusChange(showTitle string, showID uint, oldStatus, newStatus, actorEmail string)
	NotifyShowApproved(show *ShowResponse)
	NotifyShowRejected(show *ShowResponse, reason string)
	NotifyShowReport(report *communitym.ShowReport, reporterEmail string)
	NotifyArtistReport(report *communitym.ArtistReport, reporterEmail string)
	NotifyNewVenue(venueID uint, venueName, city, state string, address *string, submitterEmail string)
}
