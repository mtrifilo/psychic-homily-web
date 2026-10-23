package auth

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// The "Create your account" step: a new provider identity that arrives at the
// sign-in callback with no consent is held as a pending sign-up, and the
// frontend step reads it and finishes it. These are the routes and cookies
// that carry it.
const (
	OAuthPendingSignupRoute = "/auth/oauth/pending-signup"
	OAuthFinishSignupRoute  = "/auth/oauth/finish-signup"
)

// oauthSignupStepPath is the frontend page the callback sends a new identity
// to (frontend/app/auth/complete/page.tsx).
const oauthSignupStepPath = "/auth/complete"

// signupStepProviders are the providers whose new identities finish on the
// step. The step names the provider as Google, so any other provider keeps the
// consent refusal it had.
var signupStepProviders = []string{"google"}

// oauthPendingSignupCookie names the pending sign-up for the browser that
// started it, for as long as the pending sign-up lives. Its value is the
// signed token the auth service minted; the bare name is never read when the
// cookie is Secure, since the token is meaningless to any other deployment.
var oauthPendingSignupCookie = oauthHandshakeCookie{
	name:     "oauth_pending_signup",
	lifetime: contracts.OAuthPendingSignupTTL,
}

// signupAgeRefusalCookieName marks a browser that was refused an account for
// a date of birth under legal.MinSignupAge. While it lives, a new identity
// from that browser lands on the refusal again rather than on the form, so a
// different date cannot be tried straight away.
const signupAgeRefusalCookieName = "oauth_signup_age_refused"

// SignupAgeRefusalMarkerLifetime is how long the under-age refusal marker
// lives in the browser that was refused. It is a speed bump, not an identity
// check: clearing cookies removes it.
const SignupAgeRefusalMarkerLifetime = 24 * time.Hour

func signupAgeRefusalCookieNameFor(secure bool) string {
	if secure {
		return oauthHostCookiePrefix + signupAgeRefusalCookieName
	}
	return signupAgeRefusalCookieName
}

func signupAgeRefusalMarker(secure bool) http.Cookie {
	return *newOAuthHandshakeCookie(signupAgeRefusalCookieNameFor(secure), "1",
		int(SignupAgeRefusalMarkerLifetime/time.Second), secure)
}

func clearedPendingSignupCookie(secure bool) http.Cookie {
	return *newOAuthHandshakeCookie(oauthPendingSignupCookie.setName(secure), "", -1, secure)
}

// signupStepCookies are the cookies the step's two routes read, each under
// both of the names a deployment may set it under. Only the one matching the
// deployment's Secure setting counts. Each request struct declares the four
// cookie fields itself, because huma binds no parameter declared on an
// embedded struct; TestSignupStepRequestsDeclareTheirCookies holds the two
// copies together.
type signupStepCookies struct {
	pending, pendingHost, ageRefused, ageRefusedHost string
}

func (c signupStepCookies) pendingToken(secure bool) string {
	if secure {
		return c.pendingHost
	}
	return c.pending
}

func (c signupStepCookies) refusedForAge(secure bool) bool {
	if secure {
		return c.ageRefusedHost != ""
	}
	return c.ageRefused != ""
}

// OAuthSignupHandler serves the "Create your account" step.
type OAuthSignupHandler struct {
	authService contracts.AuthServiceInterface
	config      *config.Config
}

// NewOAuthSignupHandler creates the handler for the "Create your account" step.
func NewOAuthSignupHandler(authService contracts.AuthServiceInterface, cfg *config.Config) *OAuthSignupHandler {
	return &OAuthSignupHandler{authService: authService, config: cfg}
}

func (h *OAuthSignupHandler) secureCookies() bool {
	return h != nil && h.config != nil && h.config.Session.Secure
}

// The states GET OAuthPendingSignupRoute answers with.
const (
	pendingSignupStatusPending    = "pending"
	pendingSignupStatusExpired    = "expired"
	pendingSignupStatusAgeRefused = "age_refused"
)

// PendingOAuthSignupRequest is the step's read of its pending sign-up.
type PendingOAuthSignupRequest struct {
	Pending        string `cookie:"oauth_pending_signup" doc:"Pending sign-up, on a deployment without Secure cookies"`
	PendingHost    string `cookie:"__Host-oauth_pending_signup" doc:"Pending sign-up, on a deployment with Secure cookies"`
	AgeRefused     string `cookie:"oauth_signup_age_refused" doc:"Under-age refusal marker, on a deployment without Secure cookies"`
	AgeRefusedHost string `cookie:"__Host-oauth_signup_age_refused" doc:"Under-age refusal marker, on a deployment with Secure cookies"`
}

func (r *PendingOAuthSignupRequest) cookies() signupStepCookies {
	return signupStepCookies{r.Pending, r.PendingHost, r.AgeRefused, r.AgeRefusedHost}
}

// PendingOAuthSignupResponse is what the step renders from.
type PendingOAuthSignupResponse struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Status    string     `json:"status" enum:"pending,expired,age_refused" doc:"pending: the step can be shown. expired: the pending sign-up is expired, already used, or not this browser's. age_refused: this browser was refused an account for its date of birth."`
		Email     string     `json:"email,omitempty" doc:"The provider's address, when status is pending"`
		Name      string     `json:"name,omitempty" doc:"The provider's name for the person, when status is pending"`
		ExpiresAt *time.Time `json:"expires_at,omitempty" doc:"When the pending sign-up stops being usable, when status is pending"`
	}
}

// PendingSignupHandler answers which state the step is in, and for a live
// pending sign-up, the address and name to show. It reads only this browser's
// cookie, so the address never travels in a URL.
func (h *OAuthSignupHandler) PendingSignupHandler(ctx context.Context, input *PendingOAuthSignupRequest) (*PendingOAuthSignupResponse, error) {
	resp := &PendingOAuthSignupResponse{CacheControl: "no-store"}
	secure := h.secureCookies()

	cookies := input.cookies()
	if cookies.refusedForAge(secure) {
		resp.Body.Status = pendingSignupStatusAgeRefused
		return resp, nil
	}

	signup, err := h.authService.PeekOAuthSignup(cookies.pendingToken(secure))
	if err != nil {
		if isSignupExpired(err) {
			resp.Body.Status = pendingSignupStatusExpired
			return resp, nil
		}
		logger.AuthError(ctx, "oauth_pending_signup_read_failed", err)
		return nil, autherrors.ErrServiceUnavailable("oauth_pending_signup_read", err)
	}

	resp.Body.Status = pendingSignupStatusPending
	resp.Body.Email = signup.Identity.Email
	resp.Body.Name = signup.Identity.Name
	expiresAt := signup.ExpiresAt
	resp.Body.ExpiresAt = &expiresAt
	return resp, nil
}

// FinishOAuthSignupRequest is the step's submission.
type FinishOAuthSignupRequest struct {
	Pending        string `cookie:"oauth_pending_signup" doc:"Pending sign-up, on a deployment without Secure cookies"`
	PendingHost    string `cookie:"__Host-oauth_pending_signup" doc:"Pending sign-up, on a deployment with Secure cookies"`
	AgeRefused     string `cookie:"oauth_signup_age_refused" doc:"Under-age refusal marker, on a deployment without Secure cookies"`
	AgeRefusedHost string `cookie:"__Host-oauth_signup_age_refused" doc:"Under-age refusal marker, on a deployment with Secure cookies"`
	Body           struct {
		TermsAccepted  bool    `json:"terms_accepted" doc:"Whether the person ticked the Terms of Service box"`
		TermsVersion   string  `json:"terms_version,omitempty" doc:"Accepted Terms of Service version: the current one, or the previous one while a grace window is open (both published at GET /legal/versions); anything else is refused with TERMS_ACCEPTANCE_REQUIRED"`
		PrivacyVersion string  `json:"privacy_version,omitempty" doc:"Acknowledged Privacy Policy version, under the same rule as terms_version"`
		Name           *string `json:"name,omitempty" required:"false" doc:"The name the account takes as its display name. Omitted: the provider's names. Blank: no name."`
		DateOfBirth    string  `json:"date_of_birth,omitempty" doc:"Date of birth, YYYY-MM-DD. The age is computed from it in UTC at submission; under the minimum age the account is refused with AGE_CONFIRMATION_REQUIRED and the pending sign-up is used up."`
	}
}

func (r *FinishOAuthSignupRequest) cookies() signupStepCookies {
	return signupStepCookies{r.Pending, r.PendingHost, r.AgeRefused, r.AgeRefusedHost}
}

// FinishOAuthSignupResponse is RegisterResponse's shape plus the destination
// the sign-in named.
type FinishOAuthSignupResponse struct {
	SetCookie []http.Cookie `header:"Set-Cookie" doc:"Authentication cookie, and the pending sign-up and refusal marker cookies"`
	Body      struct {
		Success   bool        `json:"success" doc:"Success status"`
		Message   string      `json:"message" doc:"Response message"`
		Token     string      `json:"token,omitempty" doc:"JWT token for non-cookie clients"`
		ErrorCode string      `json:"error_code,omitempty" example:"OAUTH_SIGNUP_EXPIRED" doc:"Error code for programmatic handling"`
		RequestID string      `json:"request_id,omitempty" doc:"Request ID for debugging"`
		User      *authm.User `json:"user,omitempty" doc:"User information"`
		ReturnTo  string      `json:"return_to,omitempty" doc:"Where the person was headed when they started signing in, a path on the frontend"`
	}
}

// FinishSignupHandler creates the account for this browser's pending sign-up
// and signs it in.
//
// A refusal answers 200 with success false and its code, the way registration
// does. A field the person can fix (name, terms, a date that is not a date)
// leaves the pending sign-up in place. A date of birth under
// legal.MinSignupAge uses it up and sets the refusal marker, so the step
// cannot simply be retried with another date.
func (h *OAuthSignupHandler) FinishSignupHandler(ctx context.Context, input *FinishOAuthSignupRequest) (*FinishOAuthSignupResponse, error) {
	resp := &FinishOAuthSignupResponse{}
	resp.Body.RequestID = logger.GetRequestID(ctx)
	secure := h.secureCookies()
	cookies := input.cookies()
	token := cookies.pendingToken(secure)

	refuse := func(err *autherrors.AuthError) (*FinishOAuthSignupResponse, error) {
		resp.Body.Success = false
		resp.Body.Message = err.UserMessage()
		resp.Body.ErrorCode = err.Code
		return resp, nil
	}
	refuseUnderAge := func(detail string) (*FinishOAuthSignupResponse, error) {
		// Whatever the token names is spent: the refusal is the end of this
		// sign-up, and the marker is what keeps the next one from the form.
		if _, err := h.authService.ConsumeOAuthSignup(token); err != nil && !isSignupExpired(err) {
			logger.AuthError(ctx, "oauth_finish_signup_consume_failed", err)
		}
		resp.SetCookie = append(resp.SetCookie, clearedPendingSignupCookie(secure), signupAgeRefusalMarker(secure))
		logger.AuthWarn(ctx, "oauth_finish_signup_refused", "error_code", autherrors.CodeAgeConfirmationRequired, "detail", detail)
		return refuse(autherrors.ErrAgeConfirmationRequired(detail))
	}
	refuseExpired := func(err error) (*FinishOAuthSignupResponse, error) {
		resp.SetCookie = append(resp.SetCookie, clearedPendingSignupCookie(secure))
		logger.AuthWarn(ctx, "oauth_finish_signup_refused", "error_code", autherrors.CodeOAuthSignupExpired)
		return refuse(autherrors.ErrOAuthSignupExpired(err))
	}
	refuseField := func(err *autherrors.AuthError) (*FinishOAuthSignupResponse, error) {
		logger.AuthWarn(ctx, "oauth_finish_signup_refused", "error_code", err.Code)
		return refuse(err)
	}

	if cookies.refusedForAge(secure) {
		return refuseUnderAge("browser carries the under-age refusal marker")
	}

	if _, err := h.authService.PeekOAuthSignup(token); err != nil {
		if isSignupExpired(err) {
			return refuseExpired(err)
		}
		return h.unavailable(ctx, resp, "oauth_finish_signup_read", err)
	}

	var displayName *string
	if input.Body.Name != nil {
		name, message, ok := validateProfileName("Name", *input.Body.Name)
		if !ok {
			return refuseField(autherrors.ErrValidationFailed(message))
		}
		displayName = &name
	}

	if !input.Body.TermsAccepted {
		return refuseField(autherrors.ErrTermsAcceptanceRequired("terms box not ticked on the sign-up step"))
	}
	if refusal := refuseStaleConsentVersions(input.Body.TermsVersion, input.Body.PrivacyVersion); refusal != nil {
		return refuseField(refusal)
	}

	now := time.Now().UTC()
	dateOfBirth, err := legal.ParseDateOfBirth(input.Body.DateOfBirth, now)
	if err != nil {
		return refuseField(autherrors.ErrValidationFailed(dateOfBirthInvalidMessage))
	}
	age := legal.AgeOn(dateOfBirth, now)
	if age < legal.MinSignupAge {
		return refuseUnderAge("date of birth below the minimum age")
	}

	signup, err := h.authService.ConsumeOAuthSignup(token)
	if err != nil {
		if isSignupExpired(err) {
			return refuseExpired(err)
		}
		return h.unavailable(ctx, resp, "oauth_finish_signup_consume", err)
	}
	resp.SetCookie = append(resp.SetCookie, clearedPendingSignupCookie(secure))

	identity := signup.Identity
	if displayName != nil && *displayName == "" {
		// A cleared name stores no name at all, not the provider's.
		identity.FirstName, identity.LastName = "", ""
		displayName = nil
	}
	signup.Identity = identity

	user, sessionToken, err := h.authService.CompleteOAuthSignup(signup, &contracts.OAuthSignupConsent{
		TermsAccepted:  true,
		TermsVersion:   input.Body.TermsVersion,
		PrivacyVersion: input.Body.PrivacyVersion,
		AcceptedAt:     now,
		AgeConfirmed:   true,
		MinAgeAttested: age,
		DateOfBirth:    &dateOfBirth,
	}, displayName)
	if err != nil {
		if refusal, ok := refusalWithOwnCopy(err); ok {
			logger.AuthWarn(ctx, "oauth_finish_signup_refused", "provider", identity.Provider, "error_code", refusal.Code)
			return refuse(refusal)
		}
		return h.unavailable(ctx, resp, "oauth_finish_signup_create", err)
	}

	resp.SetCookie = append(resp.SetCookie, h.config.Session.NewAuthCookie(sessionToken, oauthSessionLifetime))
	logger.AuthInfo(ctx, "oauth_finish_signup_success", "provider", identity.Provider, "user_id", user.ID)

	resp.Body.Success = true
	resp.Body.Message = "Account created"
	resp.Body.Token = sessionToken
	resp.Body.User = user
	resp.Body.ReturnTo = sanitizeOAuthReturnTo(signup.ReturnTo)
	return resp, nil
}

// dateOfBirthInvalidMessage is the refusal for a date of birth that is not a
// plausible calendar date (legal.ParseDateOfBirth).
const dateOfBirthInvalidMessage = "Enter a valid date of birth."

func (h *OAuthSignupHandler) unavailable(ctx context.Context, resp *FinishOAuthSignupResponse, operation string, err error) (*FinishOAuthSignupResponse, error) {
	logger.AuthError(ctx, operation+"_failed", err)
	resp.Body.Success = false
	resp.Body.Message = autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable)
	resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
	return resp, autherrors.ErrServiceUnavailable(operation, err)
}

func isSignupExpired(err error) bool {
	var authErr *autherrors.AuthError
	return errors.As(err, &authErr) && authErr.Code == autherrors.CodeOAuthSignupExpired
}

// oauthSessionLifetime is how long the session cookie a provider sign-in sets
// lives, on the callback and on the step that finishes one.
const oauthSessionLifetime = 7 * 24 * time.Hour

// startSignupStep answers a sign-in callback whose identity needs an account
// created: it holds the identity as a pending sign-up and sends the browser
// to the step. A browser carrying the under-age refusal marker gets no
// pending sign-up; the step shows it the refusal.
func (h *OAuthHTTPHandler) startSignupStep(w http.ResponseWriter, r *http.Request, identity contracts.OAuthIdentity, returnTo, frontendURL string) {
	ctx := r.Context()
	secure := h.secureCookies()
	stepURL := frontendURL + oauthSignupStepPath

	if marker, err := r.Cookie(signupAgeRefusalCookieNameFor(secure)); err == nil && marker.Value != "" {
		oauthPendingSignupCookie.clear(w, r, secure)
		logger.AuthWarn(ctx, "oauth_signup_step_refused_age_marker", "provider", identity.Provider)
		http.Redirect(w, r, stepURL, http.StatusTemporaryRedirect)
		return
	}

	token, _, err := h.authService.StartOAuthSignup(identity, returnTo)
	if err != nil {
		logger.AuthError(ctx, "oauth_signup_step_start_failed", err, "provider", identity.Provider)
		http.Redirect(w, r, authPageRefusalURL(frontendURL, err, "authentication failed", returnTo), http.StatusTemporaryRedirect)
		return
	}
	oauthPendingSignupCookie.set(w, token, secure)
	logger.AuthInfo(ctx, "oauth_signup_step_started", "provider", identity.Provider)
	http.Redirect(w, r, stepURL, http.StatusTemporaryRedirect)
}

// signupStepIdentity is the identity err holds when the callback for provider
// should finish on the step rather than refuse: a new identity with no
// consent, on a provider the step serves.
func signupStepIdentity(provider string, err error) (contracts.OAuthIdentity, bool) {
	if !slices.Contains(signupStepProviders, provider) {
		return contracts.OAuthIdentity{}, false
	}
	var required *contracts.OAuthSignupRequiredError
	if !errors.As(err, &required) {
		return contracts.OAuthIdentity{}, false
	}
	return required.Identity, true
}
