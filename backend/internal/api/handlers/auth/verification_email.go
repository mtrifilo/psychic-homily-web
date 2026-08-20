package auth

import (
	"context"

	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// verificationEmailTrigger names the flow that asked for a verification email.
// It is attached to every log event emitted by sendVerificationEmailBestEffort
// so a single set of event names stays filterable per flow. Adding a flow means
// adding a constant here, not a new event name.
type verificationEmailTrigger string

const (
	// verificationTriggerSignup is password registration (POST /auth/register).
	verificationTriggerSignup verificationEmailTrigger = "signup"
	// verificationTriggerPasskeySignup is passkey-first registration
	// (POST /auth/passkey/signup/finish).
	verificationTriggerPasskeySignup verificationEmailTrigger = "passkey_signup"
	// verificationTriggerMagicLink is the magic-link fallback: an unverified
	// account cannot receive a magic link, so it receives a verification link
	// instead.
	verificationTriggerMagicLink verificationEmailTrigger = "magic_link"
)

// sendVerificationEmailBestEffort mints an email-verification token and sends
// the verification email, swallowing every failure into a log line.
//
// Best-effort on purpose. Every caller has already completed the user-visible
// operation by the time this runs (the account exists, the magic link was
// requested), so an email-service outage must not roll that back or turn a
// successful signup into an error response. The user is never stranded: the
// resend affordance on the submission gate and in Settings re-triggers the same
// send through SendVerificationEmailHandler, which does surface failures.
//
// Package-level rather than a method because two handler types need it:
// AuthHandler (registration, magic link) and PasskeyHandler (passkey signup).
func sendVerificationEmailBestEffort(
	ctx context.Context,
	jwtService contracts.JWTServiceInterface,
	emailService contracts.EmailServiceInterface,
	user *authm.User,
	trigger verificationEmailTrigger,
) {
	if user == nil {
		return
	}

	if user.Email == nil || *user.Email == "" {
		// The one branch that strands a user: the account exists, it is
		// unverified, and there is no address to send the link to. No
		// account-creation path produces this today, so it is logged rather
		// than silently skipped, otherwise a future path that does would
		// regress invisibly.
		logger.AuthWarn(ctx, "verification_email_no_address",
			"user_id", user.ID,
			"trigger", string(trigger),
		)
		return
	}

	if jwtService == nil || emailService == nil || !emailService.IsConfigured() {
		// Expected in local/dev stacks with no RESEND_API_KEY. Logged at warn so
		// a production misconfiguration is visible rather than silent.
		logger.AuthWarn(ctx, "verification_email_unavailable",
			"user_id", user.ID,
			"trigger", string(trigger),
		)
		return
	}

	token, err := jwtService.CreateVerificationToken(user.ID, *user.Email)
	if err != nil {
		logger.AuthError(ctx, "verification_email_token_failed", err,
			"user_id", user.ID,
			"trigger", string(trigger),
		)
		return
	}

	if err := emailService.SendVerificationEmail(*user.Email, token); err != nil {
		logger.AuthError(ctx, "verification_email_send_failed", err,
			"user_id", user.ID,
			"trigger", string(trigger),
		)
		return
	}

	logger.AuthInfo(ctx, "verification_email_sent",
		"user_id", user.ID,
		"trigger", string(trigger),
		"email_hash", logger.HashEmail(*user.Email),
	)
}
