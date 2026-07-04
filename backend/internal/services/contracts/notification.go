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

// SceneDigestShow is one this-week show line in the weekly scene digest
// (PSY-1342). DisplayTitle is the resolved title→bill→"Untitled Show" label.
type SceneDigestShow struct {
	DisplayTitle string
	Date         string // human date, e.g. "Fri, Jul 4"
	VenueName    string
	ShowURL      string
}

// SceneDigestArtist is one "new band based here" line (PSY-1342).
type SceneDigestArtist struct {
	Name      string
	ArtistURL string
}

// SceneDigestGroup is one followed scene's section in a user's weekly scene
// digest — this-week shows + new bands based there since the last digest. A
// section is rendered only when at least one of the two is non-empty. PSY-1342.
type SceneDigestGroup struct {
	SceneName  string // "City, ST"
	SceneURL   string
	Shows      []SceneDigestShow
	NewArtists []SceneDigestArtist
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
	// Each takes an HMAC-signed unsubscribeURL (RFC 8058 one-click).
	SendTierPromotionEmail(toEmail, username, oldTier, newTier, reason, unsubscribeURL string, newPermissions []string) error
	SendTierDemotionEmail(toEmail, username, oldTier, newTier, reason, unsubscribeURL string) error
	SendTierDemotionWarningEmail(toEmail, username, currentTier string, currentRate float64, threshold float64, unsubscribeURL string) error
	SendEditApprovedEmail(toEmail, username, entityType, entityName, entityURL, unsubscribeURL string) error
	SendEditRejectedEmail(toEmail, username, entityType, entityName, rejectionReason, unsubscribeURL string) error
	// PSY-289: comment + mention notifications.
	SendCommentNotification(toEmail, commenterName, entityType, entityName, commentExcerpt, entityURL, unsubscribeURL string) error
	SendMentionNotification(toEmail, mentionerName, entityType, entityName, commentExcerpt, commentURL, unsubscribeURL string) error
	// PSY-350: collection digest email — single batched email per user per
	// week grouping items added across all subscribed collections.
	SendCollectionDigestEmail(toEmail string, groups []CollectionDigestGroup, unsubscribeURL string) error
	// PSY-1342: weekly scene digest — single batched email per user grouping
	// this-week shows + new bands across all the scenes they follow.
	SendSceneDigestEmail(toEmail string, groups []SceneDigestGroup, unsubscribeURL string) error
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
	NotifyNewRadioShows(stationName string, newShowNames []string)
}
