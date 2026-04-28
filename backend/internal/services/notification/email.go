package notification

import (
	"fmt"
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

// NewEmailService creates a new email service instance
func NewEmailService(cfg *config.Config) *EmailService {
	var client *resend.Client
	if cfg.Email.ResendAPIKey != "" {
		client = resend.NewClient(cfg.Email.ResendAPIKey)
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
        <h2 style="margin-top: 0; color: #1a1a1a;">Verify Your Email Address</h2>
        <p>Thanks for signing up! Please verify your email address to start submitting shows to the Arizona music calendar.</p>
        <p style="text-align: center; margin: 30px 0;">
            <a href="%s" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">Verify Email</a>
        </p>
        <p style="font-size: 14px; color: #666;">This link will expire in 24 hours.</p>
    </div>
    
    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>If you didn't create an account, you can safely ignore this email.</p>
        <p>If the button doesn't work, copy and paste this link into your browser:</p>
        <p style="word-break: break-all; color: #666;">%s</p>
    </div>
</body>
</html>
`, verifyURL, verifyURL)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Verify your email address - Psychic Homily",
		Html:    html,
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

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Sign in to Psychic Homily",
		Html:    html,
	}

	_, err := s.client.Emails.Send(params)
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

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Recover your Psychic Homily account",
		Html:    html,
	}

	_, err := s.client.Emails.Send(params)
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

// SendShowReminderEmail sends a reminder email ~24h before a saved show
func (s *EmailService) SendShowReminderEmail(toEmail, showTitle, showURL, unsubscribeURL string, eventDate time.Time, venues []string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email service is not configured")
	}

	formattedDate := eventDate.Format("Monday, January 2, 2006 at 3:04 PM")
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

	_, err := s.client.Emails.Send(params)
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
func (s *EmailService) SendTierPromotionEmail(toEmail, username, oldTier, newTier, reason string, newPermissions []string) error {
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

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Thank you for contributing to the Psychic Homily community.</p>
    </div>
</body>
</html>
`, greeting, oldDisplayName, displayName, reason, permissionsHTML, nextTierHTML)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: fmt.Sprintf("You've been promoted to %s!", displayName),
		Html:    html,
	}

	_, err := s.client.Emails.Send(params)
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
func (s *EmailService) SendTierDemotionEmail(toEmail, username, oldTier, newTier, reason string) error {
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

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Your contributions are valued. Keep at it and you'll regain your tier.</p>
    </div>
</body>
</html>
`, greeting, oldDisplayName, newDisplayName, reason)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Your contributor tier has changed",
		Html:    html,
	}

	_, err := s.client.Emails.Send(params)
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
func (s *EmailService) SendTierDemotionWarningEmail(toEmail, username, currentTier string, currentRate float64, threshold float64) error {
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

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>This is a friendly heads-up to help you maintain your contributor status.</p>
    </div>
</body>
</html>
`, greeting, currentRate*100, threshold*100, displayName)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: "Your contributor status is at risk",
		Html:    html,
	}

	_, err := s.client.Emails.Send(params)
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
func (s *EmailService) SendEditApprovedEmail(toEmail, username, entityType, entityName, entityURL string) error {
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

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Keep contributing to build your reputation and unlock new permissions.</p>
    </div>
</body>
</html>
`, greeting, entityType, entityName, entityURL, entityTypeTitle)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: fmt.Sprintf("Your edit to %s was approved!", entityName),
		Html:    html,
	}

	_, err := s.client.Emails.Send(params)
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

	_, err := s.client.Emails.Send(params)
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

	_, err := s.client.Emails.Send(params)
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
// added across all of the recipient's subscribed collections in the last 24h.
// PSY-350. Caller groups by collection and provides the rendered URLs.
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

	subject := fmt.Sprintf("Your collections digest: %d new %s", totalItems, pluralize("item", totalItems))
	if len(groups) == 1 {
		subject = fmt.Sprintf("New in %s: %d %s", groups[0].CollectionTitle, totalItems, pluralize("item", totalItems))
	}

	// Render each group as its own block.
	var groupsHTML strings.Builder
	for _, g := range groups {
		groupsHTML.WriteString(fmt.Sprintf(
			`<div style="margin-bottom: 24px;">
				<h3 style="margin: 0 0 8px; color: #1a1a1a;"><a href="%s" style="color: #1a1a1a; text-decoration: none;">%s</a></h3>
				<ul style="margin: 0; padding-left: 20px; color: #444;">`,
			g.CollectionURL,
			htmlEscape(g.CollectionTitle),
		))
		for _, item := range g.Items {
			groupsHTML.WriteString(fmt.Sprintf(
				`<li style="margin-bottom: 4px;"><a href="%s" style="color: #f97316; text-decoration: none;">%s</a> <span style="color: #888;">(%s, added by %s)</span></li>`,
				item.EntityURL,
				htmlEscape(item.EntityName),
				htmlEscape(item.EntityType),
				htmlEscape(item.AddedBy),
			))
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
        <p style="font-size: 15px; color: #444;">Items added since your last digest.</p>
        %s
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>You're receiving this because you're subscribed to one or more collections on Psychic Homily.</p>
        <p>Don't want these digests? <a href="%s" style="color: #666;">Unsubscribe</a></p>
    </div>
</body>
</html>
`, groupsHTML.String(), unsubscribeURL)

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

	_, err := s.client.Emails.Send(params)
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
func (s *EmailService) SendEditRejectedEmail(toEmail, username, entityType, entityName, rejectionReason string) error {
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

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't be discouraged — your contributions are valued. Feel free to submit a revised edit.</p>
    </div>
</body>
</html>
`, entityName, greeting, entityType, entityName, rejectionReason)

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("Psychic Homily <%s>", s.fromEmail),
		To:      []string{toEmail},
		Subject: fmt.Sprintf("Update on your edit to %s", entityName),
		Html:    html,
	}

	_, err := s.client.Emails.Send(params)
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
