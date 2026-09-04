package notification

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/contracts"

	"github.com/resend/resend-go/v2"
)

// EmailService handles sending transactional emails via Resend
type EmailService struct {
	client      *resend.Client
	fromEmail   string
	frontendURL string
}

// sendTimeout bounds every outbound Resend API call.
//
// resend.NewClient hands the SDK http.DefaultClient, which has NO timeout, so a
// hung Resend endpoint blocks the calling goroutine forever. That was survivable
// while email only went out from low-traffic flows; PSY-1871 puts a send on the
// signup request path, where an unbounded call would hang every registration.
// The sends are all best-effort or fail-closed, so timing out is strictly better
// than blocking.
const sendTimeout = 15 * time.Second

// NewEmailService creates a new email service instance
func NewEmailService(cfg *config.Config) *EmailService {
	var client *resend.Client
	if cfg.Email.ResendAPIKey != "" {
		client = resend.NewCustomClient(
			&http.Client{Timeout: sendTimeout},
			cfg.Email.ResendAPIKey,
		)
	}

	return &EmailService{
		client:      client,
		fromEmail:   cfg.Email.FromEmail,
		frontendURL: cfg.Email.FrontendURL,
	}
}

// IsConfigured returns true if the email service is properly configured
func (s *EmailService) IsConfigured() bool {
	return s.client != nil && s.fromEmail != ""
}

// SendVerificationEmail sends an email verification link to the user
func (s *EmailService) SendVerificationEmail(toEmail, token string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", s.frontendURL, token)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Verify your email",
		Html:    verificationEmailHTML(verifyURL),
	}

	_, err := s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "verification")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	return nil
}

// verificationEmailHTML builds the body of the verification email.
//
// The copy leads with alerts because reaching people about shows is the point
// of an address on file, but it stays on the right side of one line: it says
// what a verified address is FOR, never that verifying switches delivery on.
// Nothing in the product gates alert delivery on the flag. The show-alert
// sender resolves a recipient with a bare email lookup in sendFilterEmail and
// never reads email_verified; the only place the flag actually blocks anything
// is show submission, in the catalog create handler. So "it unlocks submitting
// shows" is a present-tense promise the code keeps, and the alert sentence is
// deliberately future-tense.
//
// If alert delivery ever does start gating on verification, this paragraph can
// make the stronger claim. Until then it must not.
func verificationEmailHTML(verifyURL string) string {
	body := emailHeadline("Verify your email.") +
		emailParagraph("A verified email is what lets the index reach you. "+
			"It is where alerts for the artists and venues you follow will "+
			"land, and it unlocks submitting shows to the shared calendar.") +
		emailButton(verifyURL, "Verify email") +
		emailMonoNote("THIS LINK EXPIRES IN 24 HOURS") +
		emailFineprint([]string{
			"Not you? Ignore this email and nothing happens.",
			"If the button fails, paste this link into your browser:",
			verifyURL,
		})

	return emailShell("YOUR ACCOUNT · PENDING VERIFICATION", body)
}

// SendMagicLinkEmail sends a magic link login email to the user
func (s *EmailService) SendMagicLinkEmail(toEmail, token string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	magicLinkURL := fmt.Sprintf("%s/auth/magic-link?token=%s", s.frontendURL, token)

	html, err := renderEmailTemplate(magicLinkEmailTemplate, magicLinkEmailData{
		MagicLinkURL: magicLinkURL,
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Sign in to Psychic Homily",
		Html:    html,
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "magic_link")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send magic link email: %w", err)
	}

	return nil
}

// SendAccountRecoveryEmail sends an account recovery link to the user
func (s *EmailService) SendAccountRecoveryEmail(toEmail, token string, daysRemaining int) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	recoveryURL := fmt.Sprintf("%s/auth/recover?token=%s", s.frontendURL, token)

	html, err := renderEmailTemplate(accountRecoveryEmailTemplate, accountRecoveryEmailData{
		DaysRemaining: daysRemaining,
		RecoveryURL:   recoveryURL,
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Recover your Psychic Homily account",
		Html:    html,
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "account_recovery")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send account recovery email: %w", err)
	}

	return nil
}

// The two date registers of the show reminder. They differ only in the trailing
// clause, so a reader comparing a withheld line against a normal one sees the
// same date in the same shape with nothing substituted for the hour.
const (
	showReminderDateTimeLayout = "Monday, January 2, 2006 at 3:04 PM"
	showReminderDateOnlyLayout = "Monday, January 2, 2006"
)

// SendShowReminderEmail sends a reminder email ~24h before a saved show.
//
// The clock is dropped when the event's zone is unresolved (see
// contracts.LocalizedEventTime): naming an hour there would state a time read
// off the Arizona fallback, which for a non-US room is wrong by hours. The date
// carries the whole line on its own in that case.
func (s *EmailService) SendShowReminderEmail(toEmail, showTitle, showURL, unsubscribeURL string, eventTime contracts.LocalizedEventTime, venues []string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	dateLayout := showReminderDateOnlyLayout
	if eventTime.ZoneResolved {
		dateLayout = showReminderDateTimeLayout
	}
	formattedDate := eventTime.At.Format(dateLayout)

	html, err := renderEmailTemplate(showReminderEmailTemplate, showReminderEmailData{
		ShowTitle:      showTitle,
		FormattedDate:  formattedDate,
		VenueText:      strings.Join(venues, ", "),
		ShowURL:        showURL,
		UnsubscribeURL: unsubscribeURL,
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: fmt.Sprintf("Reminder: %s is tomorrow", showTitle),
		Html:    html,
		Headers: map[string]string{
			"List-Unsubscribe":      fmt.Sprintf("<%s>", unsubscribeURL),
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "show_reminder")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send show reminder email: %w", err)
	}

	return nil
}

// SendFilterNotificationEmail sends a notification email for a matched filter.
// The caller builds the HTML body; this method just sends it with proper headers.
func (s *EmailService) SendFilterNotificationEmail(toEmail, subject, htmlBody, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: subject,
		Html:    htmlBody,
		Headers: map[string]string{
			"List-Unsubscribe":      fmt.Sprintf("<%s>", unsubscribeURL),
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
	}

	_, err := s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "filter_notification")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send filter notification email: %w", err)
	}

	return nil
}

// TierDisplayName maps tier constants to human-readable display names.
func TierDisplayName(tier string) string {
	switch tier {
	case "new_user":
		return "New User"
	case "contributor":
		return "Contributor"
	case "trusted_contributor":
		return "Trusted Contributor"
	case "local_ambassador":
		return "Local Ambassador"
	default:
		return tier
	}
}

// TierPermissions returns the list of permissions unlocked at a given tier.
func TierPermissions(tier string) []string {
	switch tier {
	case "contributor":
		return []string{
			"Submit edits for review",
			"Vote on tags and relationships",
			"Create collections",
		}
	case "trusted_contributor":
		return []string{
			"Edit entities directly (no review needed)",
			"Higher daily edit limit",
		}
	case "local_ambassador":
		return []string{
			"All Trusted Contributor permissions",
			"Featured on city pages",
		}
	default:
		return nil
	}
}

// SendTierPromotionEmail sends a congratulatory email when a user is promoted to a higher tier.
// unsubscribeURL is the HMAC-signed tier-notifications opt-out link (RFC 8058).
func (s *EmailService) SendTierPromotionEmail(toEmail, username, oldTier, newTier, reason, unsubscribeURL string, newPermissions []string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	displayName := TierDisplayName(newTier)
	oldDisplayName := TierDisplayName(oldTier)

	greeting := "there"
	if username != "" {
		greeting = username
	}

	html, err := renderEmailTemplate(tierPromotionEmailTemplate, tierPromotionEmailData{
		Greeting:       greeting,
		OldDisplayName: oldDisplayName,
		DisplayName:    displayName,
		Reason:         reason,
		NewTier:        newTier,
		NewPermissions: newPermissions,
		Unsubscribe:    unsubscribeCard{URL: unsubscribeURL, Label: "tier-change emails"},
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: fmt.Sprintf("You've been promoted to %s!", displayName),
		Html:    html,
		Headers: unsubscribeHeaders(unsubscribeURL),
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "tier_promotion")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send tier promotion email: %w", err)
	}

	return nil
}

// SendTierDemotionEmail sends a notification when a user is demoted to a lower tier.
// unsubscribeURL is the HMAC-signed tier-notifications opt-out link (RFC 8058).
func (s *EmailService) SendTierDemotionEmail(toEmail, username, oldTier, newTier, reason, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	oldDisplayName := TierDisplayName(oldTier)
	newDisplayName := TierDisplayName(newTier)

	greeting := "there"
	if username != "" {
		greeting = username
	}

	html, err := renderEmailTemplate(tierDemotionEmailTemplate, tierDemotionEmailData{
		Greeting:       greeting,
		OldDisplayName: oldDisplayName,
		NewDisplayName: newDisplayName,
		Reason:         reason,
		Unsubscribe:    unsubscribeCard{URL: unsubscribeURL, Label: "tier-change emails"},
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Your contributor tier has changed",
		Html:    html,
		Headers: unsubscribeHeaders(unsubscribeURL),
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "tier_demotion")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send tier demotion email: %w", err)
	}

	return nil
}

// SendTierDemotionWarningEmail sends a warning when a user's approval rate is approaching the demotion threshold.
// unsubscribeURL is the HMAC-signed tier-notifications opt-out link (RFC 8058).
func (s *EmailService) SendTierDemotionWarningEmail(toEmail, username, currentTier string, currentRate float64, threshold float64, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	displayName := TierDisplayName(currentTier)

	greeting := "there"
	if username != "" {
		greeting = username
	}

	html, err := renderEmailTemplate(tierDemotionWarningEmailTemplate, tierDemotionWarningEmailData{
		Greeting:    greeting,
		CurrentRate: currentRate * 100,
		Threshold:   threshold * 100,
		DisplayName: displayName,
		Unsubscribe: unsubscribeCard{URL: unsubscribeURL, Label: "tier-change emails"},
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Your contributor status is at risk",
		Html:    html,
		Headers: unsubscribeHeaders(unsubscribeURL),
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "tier_demotion_warning")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send tier demotion warning email: %w", err)
	}

	return nil
}

// SendEditApprovedEmail sends a notification when a user's pending edit is approved.
// unsubscribeURL is the HMAC-signed edit-notifications opt-out link (RFC 8058).
func (s *EmailService) SendEditApprovedEmail(toEmail, username, entityType, entityName, entityURL, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	greeting := "there"
	if username != "" {
		greeting = username
	}

	// Capitalize first letter for CTA button text (e.g. "artist" -> "Artist")
	entityTypeTitle := strings.ToUpper(entityType[:1]) + entityType[1:]

	html, err := renderEmailTemplate(editApprovedEmailTemplate, editApprovedEmailData{
		Greeting:        greeting,
		EntityType:      entityType,
		EntityName:      entityName,
		EntityURL:       entityURL,
		EntityTypeTitle: entityTypeTitle,
		Unsubscribe:     unsubscribeCard{URL: unsubscribeURL, Label: "edit-review emails"},
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: fmt.Sprintf("Your edit to %s was approved!", entityName),
		Html:    html,
		Headers: unsubscribeHeaders(unsubscribeURL),
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "edit_approved")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send edit approved email: %w", err)
	}

	return nil
}

// SendCommentNotification sends a notification when a new comment is posted on an
// entity the recipient is subscribed to. commenterName is the display name of the
// author (falls back to username or "A contributor" upstream — this fn just renders).
func (s *EmailService) SendCommentNotification(toEmail, commenterName, entityType, entityName, commentExcerpt, entityURL, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	if commenterName == "" {
		commenterName = "A contributor"
	}

	// Capitalize first letter of entity type for the subject (e.g. "artist" -> "Artist").
	entityTypeTitle := entityType
	if entityTypeTitle != "" {
		entityTypeTitle = strings.ToUpper(entityTypeTitle[:1]) + entityTypeTitle[1:]
	}

	// The subject is a header, not markup: it carries the entity name as
	// written, while the body below gets it escaped for the context it lands in.
	subject := fmt.Sprintf("New comment on %s", entityName)

	html, err := renderEmailTemplate(commentNotificationEmailTemplate, commentNotificationEmailData{
		EntityName:      entityName,
		CommenterName:   commenterName,
		EntityType:      entityType,
		CommentExcerpt:  commentExcerpt,
		EntityURL:       entityURL,
		EntityTypeTitle: entityTypeTitle,
		UnsubscribeURL:  unsubscribeURL,
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: subject,
		Html:    html,
		Headers: map[string]string{
			"List-Unsubscribe":      fmt.Sprintf("<%s>", unsubscribeURL),
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "comment_notification")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send comment notification email: %w", err)
	}

	return nil
}

// SendMentionNotification sends a notification when the recipient is @-mentioned
// in a comment. commentURL anchors to the specific comment on the entity page.
func (s *EmailService) SendMentionNotification(toEmail, mentionerName, entityType, entityName, commentExcerpt, commentURL, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	if mentionerName == "" {
		mentionerName = "Someone"
	}

	subject := fmt.Sprintf("%s mentioned you in a comment on %s", mentionerName, entityName)

	html, err := renderEmailTemplate(mentionNotificationEmailTemplate, mentionNotificationEmailData{
		MentionerName:  mentionerName,
		EntityType:     entityType,
		EntityName:     entityName,
		CommentExcerpt: commentExcerpt,
		CommentURL:     commentURL,
		UnsubscribeURL: unsubscribeURL,
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: subject,
		Html:    html,
		Headers: map[string]string{
			"List-Unsubscribe":      fmt.Sprintf("<%s>", unsubscribeURL),
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "mention_notification")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send mention notification email: %w", err)
	}

	return nil
}

// SendCollectionDigestEmail sends a single batched email summarizing items
// added across all of the recipient's subscribed collections in the last 7
// days. PSY-350. Caller groups by collection and provides the rendered URLs.
//
// Anti-spam hardening:
//   - The recipient must have explicitly enabled `notify_on_collection_digest`
//     (column default is FALSE / opt-IN). The digest service filters the
//     candidate query on this flag; this function is a dumb sender.
//   - RFC 8058 / RFC 2369 List-Unsubscribe headers are set so Gmail and Yahoo
//     surface the native "Unsubscribe" affordance next to the sender name.
//     The `unsubscribeURL` MUST be an HTTPS endpoint that accepts BOTH a
//     manual GET (renders an HTML confirmation page) and an unauthenticated
//     POST with body `List-Unsubscribe=One-Click` (RFC 8058 one-click).
//   - The email body itself leads with a prominent opt-out block — the
//     in-body link is the same `unsubscribeURL`, not buried in the footer.
//     Mailbox providers and recipients should both have a single visible
//     way out.
func (s *EmailService) SendCollectionDigestEmail(toEmail string, groups []contracts.CollectionDigestGroup, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}
	if len(groups) == 0 {
		return fmt.Errorf("no digest groups provided")
	}

	// Tally totals for subject line.
	totalItems := 0
	for _, g := range groups {
		totalItems += len(g.Items)
	}
	if totalItems == 0 {
		return fmt.Errorf("digest groups contain no items")
	}

	subject := fmt.Sprintf("Your weekly collections digest: %d new %s", totalItems, pluralize("item", totalItems))
	if len(groups) == 1 {
		subject = fmt.Sprintf("New this week in %s: %d %s", groups[0].CollectionTitle, totalItems, pluralize("item", totalItems))
	}

	html, err := renderEmailTemplate(collectionDigestEmailTemplate, collectionDigestEmailData{
		Groups:      groups,
		Unsubscribe: unsubscribeCard{URL: unsubscribeURL, Label: "these weekly digests"},
		FrontendURL: s.frontendURL,
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: subject,
		Html:    html,
		Headers: unsubscribeHeaders(unsubscribeURL),
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "collection_digest")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send collection digest email: %w", err)
	}

	return nil
}

// SendSceneDigestEmail sends a single batched email summarizing the next 7
// days of shows + new bands across every scene the recipient follows
// (PSY-1342). Caller groups by scene and provides the rendered URLs + display
// titles.
//
// The show sections are a ROLLING [now, now+7d) window (SceneDigestService's
// sceneDigestWindowDays feeds GetSceneUpcomingShows), NOT a Monday-to-Sunday
// week, so every string here is worded "next 7 days" and none of them says
// "this week" (PSY-1732 vocabulary, applied to this email by PSY-1766). The
// email's weekly CADENCE is a separate fact and the footer still says so.
//
// Anti-spam hardening mirrors SendCollectionDigestEmail exactly: the recipient
// must have explicitly enabled `notify_on_scene_digest` (column default FALSE /
// opt-IN; the digest service filters on it — this is a dumb sender), and the
// RFC 8058 / RFC 2369 List-Unsubscribe headers + the prominent in-body opt-out
// card use the same HMAC-signed `unsubscribeURL` (GET page + one-click POST).
func (s *EmailService) SendSceneDigestEmail(toEmail string, groups []contracts.SceneDigestGroup, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}
	if len(groups) == 0 {
		return fmt.Errorf("no scene digest groups provided")
	}

	totalShows, totalArtists := 0, 0
	for _, g := range groups {
		totalShows += len(g.Shows)
		totalArtists += len(g.NewArtists)
	}
	if totalShows == 0 && totalArtists == 0 {
		return fmt.Errorf("scene digest groups contain no content")
	}

	subject := "The next 7 days in your scenes on Psychic Homily"
	if len(groups) == 1 {
		subject = fmt.Sprintf("The next 7 days in %s", groups[0].SceneName)
	}

	html, err := renderEmailTemplate(sceneDigestEmailTemplate, sceneDigestEmailData{
		Groups:      groups,
		Unsubscribe: unsubscribeCard{URL: unsubscribeURL, Label: "weekly scene digests"},
		FrontendURL: s.frontendURL,
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: subject,
		Html:    html,
		Headers: unsubscribeHeaders(unsubscribeURL),
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "scene_digest")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send scene digest email: %w", err)
	}

	return nil
}

// unsubscribeHeaders returns the RFC 8058 / RFC 2369 List-Unsubscribe headers.
// The value MUST be a single HTTPS URL wrapped in <> (RFC 2369 §3);
// List-Unsubscribe-Post advertises RFC 8058 one-click POST so Gmail/Yahoo
// render the native "Unsubscribe" button next to the sender name.
func unsubscribeHeaders(unsubscribeURL string) map[string]string {
	return map[string]string{
		"List-Unsubscribe":      fmt.Sprintf("<%s>", unsubscribeURL),
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}
}

// pluralize returns word with an "s" appended if n != 1.
func pluralize(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// SendEditRejectedEmail sends a notification when a user's pending edit is rejected.
// unsubscribeURL is the HMAC-signed edit-notifications opt-out link (RFC 8058).
func (s *EmailService) SendEditRejectedEmail(toEmail, username, entityType, entityName, rejectionReason, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	greeting := "there"
	if username != "" {
		greeting = username
	}

	html, err := renderEmailTemplate(editRejectedEmailTemplate, editRejectedEmailData{
		EntityName:      entityName,
		Greeting:        greeting,
		EntityType:      entityType,
		RejectionReason: rejectionReason,
		Unsubscribe:     unsubscribeCard{URL: unsubscribeURL, Label: "edit-review emails"},
	})
	if err != nil {
		return err
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: fmt.Sprintf("Update on your edit to %s", entityName),
		Html:    html,
		Headers: unsubscribeHeaders(unsubscribeURL),
	}

	_, err = s.client.Emails.Send(params)
	if err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", "edit_rejected")
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send edit rejected email: %w", err)
	}

	return nil
}
