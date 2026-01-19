package services

import (
	"fmt"

	"psychic-homily-backend/internal/config"

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
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	return nil
}
