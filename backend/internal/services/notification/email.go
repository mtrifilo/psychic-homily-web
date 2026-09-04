package notification

import (
	"fmt"
	"html"
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

// outboundEmail is one message on its way to Resend, named so the five strings
// a caller passes cannot be swapped for each other at the call site.
type outboundEmail struct {
	// kind identifies the message type. It is the Sentry email_type tag, and
	// with its underscores as spaces it is the noun in the send-failure error.
	kind string
	// to is a single recipient. Every message this service sends is addressed
	// to one person; a shared To across recipients would leak the list.
	to string
	// subject is the copy as the sender wrote it. send makes it header-safe,
	// so a sender interpolates freely and never sanitizes.
	subject string
	// html is the rendered body.
	html string
	// unsubscribeURL is empty for a message with no list to leave: account
	// verification, magic link, account recovery. Empty means the
	// List-Unsubscribe headers are omitted rather than sent pointing at "<>".
	unsubscribeURL string
}

// send is the one place this package builds a resend.SendEmailRequest and the
// one place it calls the Resend client, which is what makes headerSafeSubject
// unbypassable: a sender cannot reach the transport without passing through it.
// TestResendRequestsAreBuiltOnlyBySend holds that shape.
func (s *EmailService) send(msg outboundEmail) error {
	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{msg.to},
		Subject: headerSafeSubject(msg.subject),
		Html:    msg.html,
	}
	if msg.unsubscribeURL != "" {
		params.Headers = unsubscribeHeaders(msg.unsubscribeURL)
	}

	if _, err := s.client.Emails.Send(params); err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "email")
			scope.SetTag("email_type", msg.kind)
			sentry.CaptureException(err)
		})
		return fmt.Errorf("failed to send %s email: %w", strings.ReplaceAll(msg.kind, "_", " "), err)
	}

	return nil
}

// SendVerificationEmail sends an email verification link to the user
func (s *EmailService) SendVerificationEmail(toEmail, token string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", s.frontendURL, token)

	return s.send(outboundEmail{
		kind:    "verification",
		to:      toEmail,
		subject: "Verify your email",
		html:    verificationEmailHTML(verifyURL),
	})
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

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">Sign in to your account</h2>
        <p>Click the button below to sign in to your Psychic Homily account. This link will expire in 15 minutes.</p>
        <p style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">Sign In</a>
        </p>
        <p style="font-size: 14px; color: #666;">For security, this link expires in 15 minutes and can only be used once.</p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>If you didn't request this email, you can safely ignore it.</p>
        <p>If the button doesn't work, copy and paste this link into your browser:</p>
        <p style="word-break: break-all; color: #666;">%s</p>
    </div>
</body>
</html>
`, magicLinkURL, magicLinkURL)

	return s.send(outboundEmail{
		kind:    "magic_link",
		to:      toEmail,
		subject: "Sign in to Psychic Homily",
		html:    html,
	})
}

// SendAccountRecoveryEmail sends an account recovery link to the user
func (s *EmailService) SendAccountRecoveryEmail(toEmail, token string, daysRemaining int) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	recoveryURL := fmt.Sprintf("%s/auth/recover?token=%s", s.frontendURL, token)

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">Recover Your Account</h2>
        <p>We received a request to recover your deleted Psychic Homily account. You have <strong>%d days remaining</strong> to recover your account before it is permanently deleted.</p>
        <p style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">Recover Account</a>
        </p>
        <p style="font-size: 14px; color: #666;">This link will expire in 1 hour.</p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>If you didn't request this, you can safely ignore this email. Your account will remain scheduled for deletion.</p>
        <p>If the button doesn't work, copy and paste this link into your browser:</p>
        <p style="word-break: break-all; color: #666;">%s</p>
    </div>
</body>
</html>
`, daysRemaining, recoveryURL, recoveryURL)

	return s.send(outboundEmail{
		kind:    "account_recovery",
		to:      toEmail,
		subject: "Recover your Psychic Homily account",
		html:    html,
	})
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
	venueText := ""
	if len(venues) > 0 {
		venueText = fmt.Sprintf(`<p style="font-size: 16px; color: #444;">Venue: <strong>%s</strong></p>`, strings.Join(venues, ", "))
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">%s is tomorrow!</h2>
        <p style="font-size: 16px; color: #444;">%s</p>
        %s
        <p style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">View Show</a>
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't want these reminders? <a href="%s" style="color: #666;">Unsubscribe</a></p>
    </div>
</body>
</html>
`, showTitle, formattedDate, venueText, showURL, unsubscribeURL)

	return s.send(outboundEmail{
		kind:           "show_reminder",
		to:             toEmail,
		subject:        fmt.Sprintf("Reminder: %s is tomorrow", showTitle),
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
}

// SendFilterNotificationEmail sends a notification email for a matched filter.
// The caller builds the HTML body; this method just sends it with proper headers.
func (s *EmailService) SendFilterNotificationEmail(toEmail, subject, htmlBody, unsubscribeURL string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	return s.send(outboundEmail{
		kind:           "filter_notification",
		to:             toEmail,
		subject:        subject,
		html:           htmlBody,
		unsubscribeURL: unsubscribeURL,
	})
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

	permissionsHTML := ""
	if len(newPermissions) > 0 {
		permissionsHTML = `<h3 style="color: #1a1a1a; margin-bottom: 8px;">New permissions unlocked:</h3><ul style="padding-left: 20px; color: #444;">`
		for _, perm := range newPermissions {
			permissionsHTML += fmt.Sprintf(`<li style="margin-bottom: 4px;">%s</li>`, perm)
		}
		permissionsHTML += `</ul>`
	}

	nextTierHTML := ""
	switch newTier {
	case "contributor":
		nextTierHTML = `<p style="font-size: 14px; color: #666; margin-top: 20px;">Keep contributing quality edits to reach <strong>Trusted Contributor</strong> status (25 approved edits with 95%+ approval rate).</p>`
	case "trusted_contributor":
		nextTierHTML = `<p style="font-size: 14px; color: #666; margin-top: 20px;">Keep contributing to your local scene to reach <strong>Local Ambassador</strong> status (50 approved edits with 10+ city edits).</p>`
	case "local_ambassador":
		nextTierHTML = `<p style="font-size: 14px; color: #666; margin-top: 20px;">You've reached the highest contributor tier. Thank you for your dedication to the community!</p>`
	}

	greeting := "there"
	if username != "" {
		greeting = username
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f0fdf4; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #bbf7d0;">
        <h2 style="margin-top: 0; color: #166534;">Congratulations, %s!</h2>
        <p style="font-size: 16px;">You've been promoted from <strong>%s</strong> to <strong>%s</strong>.</p>
        <p style="color: #444;">%s</p>
        %s
        %s
    </div>

    %s

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Thank you for contributing to the Psychic Homily community.</p>
    </div>
</body>
</html>
`, greeting, oldDisplayName, displayName, reason, permissionsHTML, nextTierHTML, unsubscribeCardHTML(unsubscribeURL, "tier-change emails"))

	return s.send(outboundEmail{
		kind:           "tier_promotion",
		to:             toEmail,
		subject:        fmt.Sprintf("You've been promoted to %s!", displayName),
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
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

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #fef9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #fecaca;">
        <h2 style="margin-top: 0; color: #991b1b;">Your contributor tier has changed</h2>
        <p>Hi %s,</p>
        <p>Your tier has changed from <strong>%s</strong> to <strong>%s</strong>.</p>
        <p style="color: #444;"><strong>Reason:</strong> %s</p>
        <h3 style="color: #1a1a1a; margin-bottom: 8px;">How to recover your tier:</h3>
        <ul style="padding-left: 20px; color: #444;">
            <li style="margin-bottom: 4px;">Focus on submitting accurate, high-quality edits</li>
            <li style="margin-bottom: 4px;">Double-check your information before submitting</li>
            <li style="margin-bottom: 4px;">Review the contribution guidelines for best practices</li>
        </ul>
    </div>

    %s

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Your contributions are valued. Keep at it and you'll regain your tier.</p>
    </div>
</body>
</html>
`, greeting, oldDisplayName, newDisplayName, reason, unsubscribeCardHTML(unsubscribeURL, "tier-change emails"))

	return s.send(outboundEmail{
		kind:           "tier_demotion",
		to:             toEmail,
		subject:        "Your contributor tier has changed",
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
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

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #fffbeb; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #fde68a;">
        <h2 style="margin-top: 0; color: #92400e;">Your contributor status is at risk</h2>
        <p>Hi %s,</p>
        <p>Your current approval rate of <strong>%.0f%%</strong> is approaching the <strong>%.0f%%</strong> threshold required to maintain your <strong>%s</strong> status.</p>
        <h3 style="color: #1a1a1a; margin-bottom: 8px;">Tips to improve your approval rate:</h3>
        <ul style="padding-left: 20px; color: #444;">
            <li style="margin-bottom: 4px;">Verify information from multiple sources before submitting</li>
            <li style="margin-bottom: 4px;">Pay attention to formatting and data accuracy</li>
            <li style="margin-bottom: 4px;">Review feedback on previously rejected edits</li>
        </ul>
    </div>

    %s

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>This is a friendly heads-up to help you maintain your contributor status.</p>
    </div>
</body>
</html>
`, greeting, currentRate*100, threshold*100, displayName, unsubscribeCardHTML(unsubscribeURL, "tier-change emails"))

	return s.send(outboundEmail{
		kind:           "tier_demotion_warning",
		to:             toEmail,
		subject:        "Your contributor status is at risk",
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
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

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f0fdf4; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #bbf7d0;">
        <h2 style="margin-top: 0; color: #166534;">Your edit was approved!</h2>
        <p>Hi %s,</p>
        <p>Your edit to the %s <strong>%s</strong> has been reviewed and approved. Your changes are now live!</p>
        <p style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #16a34a; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">View %s</a>
        </p>
        <p style="font-size: 14px; color: #444;">Thank you for improving the Psychic Homily database. Every contribution helps the community discover great music.</p>
    </div>

    %s

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Keep contributing to build your reputation and unlock new permissions.</p>
    </div>
</body>
</html>
`, greeting, entityType, entityName, entityURL, entityTypeTitle, unsubscribeCardHTML(unsubscribeURL, "edit-review emails"))

	return s.send(outboundEmail{
		kind:           "edit_approved",
		to:             toEmail,
		subject:        fmt.Sprintf("Your edit to %s was approved!", entityName),
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
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

	// User-controlled strings enter an HTML body below — escape at the
	// boundary (display/first names and comment bodies are free-form text;
	// entity names are community-editable).
	commenterName = html.EscapeString(commenterName)
	entityName = html.EscapeString(entityName)
	commentExcerpt = html.EscapeString(commentExcerpt)

	// Capitalize first letter of entity type for the subject (e.g. "artist" -> "Artist").
	entityTypeTitle := entityType
	if entityTypeTitle != "" {
		entityTypeTitle = strings.ToUpper(entityTypeTitle[:1]) + entityTypeTitle[1:]
	}

	subject := fmt.Sprintf("New comment on %s", entityName)

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">New comment on %s</h2>
        <p style="font-size: 15px; color: #444;"><strong>%s</strong> commented on the %s <strong>%s</strong>:</p>
        <blockquote style="border-left: 4px solid #f97316; padding-left: 16px; margin: 16px 0; color: #555; font-style: italic;">%s</blockquote>
        <p style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">View Discussion</a>
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>You're receiving this because you're subscribed to %s on %s.</p>
        <p>Don't want these notifications? <a href="%s" style="color: #666;">Unsubscribe</a></p>
    </div>
</body>
</html>
`, entityName, commenterName, entityType, entityName, commentExcerpt, entityURL, entityTypeTitle, entityName, unsubscribeURL)

	return s.send(outboundEmail{
		kind:           "comment_notification",
		to:             toEmail,
		subject:        subject,
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
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

	// Subject stays unescaped (plain-text header); the HTML body below gets
	// escaped copies of every user-controlled string.
	subject := fmt.Sprintf("%s mentioned you in a comment on %s", mentionerName, entityName)
	mentionerName = html.EscapeString(mentionerName)
	entityName = html.EscapeString(entityName)
	commentExcerpt = html.EscapeString(commentExcerpt)

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">You were mentioned</h2>
        <p style="font-size: 15px; color: #444;"><strong>%s</strong> mentioned you in a comment on the %s <strong>%s</strong>:</p>
        <blockquote style="border-left: 4px solid #f97316; padding-left: 16px; margin: 16px 0; color: #555; font-style: italic;">%s</blockquote>
        <p style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">Reply</a>
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't want mention notifications? <a href="%s" style="color: #666;">Unsubscribe</a></p>
    </div>
</body>
</html>
`, mentionerName, entityType, entityName, commentExcerpt, commentURL, unsubscribeURL)

	return s.send(outboundEmail{
		kind:           "mention_notification",
		to:             toEmail,
		subject:        subject,
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
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

	// Render each group as its own block.
	var groupsHTML strings.Builder
	for _, g := range groups {
		fmt.Fprintf(&groupsHTML, `<div style="margin-bottom: 24px;">
				<h3 style="margin: 0 0 8px; color: #1a1a1a;"><a href="%s" style="color: #1a1a1a; text-decoration: none;">%s</a></h3>
				<ul style="margin: 0; padding-left: 20px; color: #444;">`,
			g.CollectionURL,
			htmlEscape(g.CollectionTitle))
		for _, item := range g.Items {
			fmt.Fprintf(&groupsHTML, `<li style="margin-bottom: 4px;"><a href="%s" style="color: #f97316; text-decoration: none;">%s</a> <span style="color: #888;">(%s, added by %s)</span></li>`,
				item.EntityURL,
				htmlEscape(item.EntityName),
				htmlEscape(item.EntityType),
				htmlEscape(item.AddedBy))
		}
		groupsHTML.WriteString(`</ul></div>`)
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">New in your collections</h2>
        <p style="font-size: 15px; color: #444;">Items added to collections you follow over the past week.</p>
        %s
    </div>

    <div style="background: #fff7ed; border: 1px solid #fed7aa; border-radius: 8px; padding: 16px 20px; margin-bottom: 20px;">
        <p style="margin: 0; font-size: 14px; color: #444;">
            Don&rsquo;t want these weekly digests?
            <a href="%s" style="color: #c2410c; font-weight: 600;">Unsubscribe in one click</a> &mdash;
            no login required.
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>You&rsquo;re receiving this because you opted in to weekly digests for collections you follow on Psychic Homily.</p>
        <p>Manage all notifications in your <a href="%s/settings" style="color: #666;">notification settings</a>.</p>
    </div>
</body>
</html>
`, groupsHTML.String(), unsubscribeURL, s.frontendURL)

	return s.send(outboundEmail{
		kind:           "collection_digest",
		to:             toEmail,
		subject:        subject,
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
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

	// Render each scene as its own block: shows sub-list, then new-bands sub-list.
	var groupsHTML strings.Builder
	for _, g := range groups {
		fmt.Fprintf(&groupsHTML, `<div style="margin-bottom: 28px;">
				<h3 style="margin: 0 0 8px; color: #1a1a1a;"><a href="%s" style="color: #1a1a1a; text-decoration: none;">%s</a></h3>`,
			g.SceneURL, htmlEscape(g.SceneName))
		if len(g.Shows) > 0 {
			groupsHTML.WriteString(`<p style="margin: 4px 0; font-size: 13px; font-weight: 600; color: #666; text-transform: uppercase; letter-spacing: 0.04em;">Next 7 days</p>`)
			groupsHTML.WriteString(`<ul style="margin: 0 0 10px; padding-left: 20px; color: #444;">`)
			for _, sh := range g.Shows {
				venue := ""
				if sh.VenueName != "" {
					venue = " · " + htmlEscape(sh.VenueName)
				}
				fmt.Fprintf(&groupsHTML, `<li style="margin-bottom: 4px;"><a href="%s" style="color: #f97316; text-decoration: none;">%s</a> <span style="color: #888;">(%s%s)</span></li>`,
					sh.ShowURL, htmlEscape(sh.DisplayTitle), htmlEscape(sh.Date), venue)
			}
			groupsHTML.WriteString(`</ul>`)
		}
		if len(g.NewArtists) > 0 {
			groupsHTML.WriteString(`<p style="margin: 4px 0; font-size: 13px; font-weight: 600; color: #666; text-transform: uppercase; letter-spacing: 0.04em;">New bands based here</p>`)
			groupsHTML.WriteString(`<ul style="margin: 0; padding-left: 20px; color: #444;">`)
			for _, a := range g.NewArtists {
				fmt.Fprintf(&groupsHTML, `<li style="margin-bottom: 4px;"><a href="%s" style="color: #f97316; text-decoration: none;">%s</a></li>`,
					a.ArtistURL, htmlEscape(a.Name))
			}
			if g.MoreNewArtists > 0 {
				fmt.Fprintf(&groupsHTML, `<li style="margin-bottom: 4px; list-style: none; color: #888;"><a href="%s" style="color: #888;">+%d more new %s — see the scene</a></li>`,
					g.SceneURL, g.MoreNewArtists, pluralize("band", g.MoreNewArtists))
			}
			groupsHTML.WriteString(`</ul>`)
		}
		groupsHTML.WriteString(`</div>`)
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">Your scenes: the next 7 days</h2>
        <p style="font-size: 15px; color: #444;">Shows in the next 7 days and new bands, for the scenes you follow.</p>
        %s
    </div>

    %s

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>You&rsquo;re receiving this because you opted in to weekly scene digests on Psychic Homily.</p>
        <p>Manage all notifications in your <a href="%s/settings" style="color: #666;">notification settings</a>.</p>
    </div>
</body>
</html>
`, groupsHTML.String(), unsubscribeCardHTML(unsubscribeURL, "weekly scene digests"), s.frontendURL)

	return s.send(outboundEmail{
		kind:           "scene_digest",
		to:             toEmail,
		subject:        subject,
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
}

// unsubscribeCardHTML renders the prominent in-body opt-out block shared by
// the notification emails. `label` describes the category in the recipient's
// words (e.g. "tier-change emails"). The same `unsubscribeURL`
// goes in the List-Unsubscribe header — RFC 8058 one-click and the visible
// in-body link are the same endpoint, so a recipient and a mailbox provider
// both have a single way out. The endpoint requires no login (HMAC-signed).
func unsubscribeCardHTML(unsubscribeURL, label string) string {
	return fmt.Sprintf(`
    <div style="background: #fff7ed; border: 1px solid #fed7aa; border-radius: 8px; padding: 16px 20px; margin-bottom: 20px;">
        <p style="margin: 0; font-size: 14px; color: #444;">
            Don&rsquo;t want %s?
            <a href="%s" style="color: #c2410c; font-weight: 600;">Unsubscribe in one click</a> &mdash;
            no login required.
        </p>
    </div>`, label, unsubscribeURL)
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

// htmlEscape replaces a small set of characters with their HTML entity
// equivalents. Intentionally minimal — the digest builder controls every
// string passed in (titles, names, URLs come from our DB), but HTML-escaping
// names is still the right hygiene to prevent the rare display issue with
// "&", "<", ">", or quotes in entity names.
func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
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

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>

    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #e5e7eb;">
        <h2 style="margin-top: 0; color: #1a1a1a;">Update on your edit to %s</h2>
        <p>Hi %s,</p>
        <p>Your edit to the %s <strong>%s</strong> was not accepted this time.</p>
        <p style="background: #fef3c7; border-radius: 6px; padding: 12px 16px; color: #92400e;"><strong>Reason:</strong> %s</p>
        <h3 style="color: #1a1a1a; margin-bottom: 8px;">Tips for future edits:</h3>
        <ul style="padding-left: 20px; color: #444;">
            <li style="margin-bottom: 4px;">Double-check facts against official sources (venue websites, artist pages)</li>
            <li style="margin-bottom: 4px;">Include a clear summary explaining why you are making the change</li>
            <li style="margin-bottom: 4px;">Ensure spelling and formatting are accurate</li>
        </ul>
    </div>

    %s

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't be discouraged — your contributions are valued. Feel free to submit a revised edit.</p>
    </div>
</body>
</html>
`, entityName, greeting, entityType, entityName, rejectionReason, unsubscribeCardHTML(unsubscribeURL, "edit-review emails"))

	return s.send(outboundEmail{
		kind:           "edit_rejected",
		to:             toEmail,
		subject:        fmt.Sprintf("Update on your edit to %s", entityName),
		html:           html,
		unsubscribeURL: unsubscribeURL,
	})
}
