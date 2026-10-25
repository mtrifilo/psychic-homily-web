package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// The Sign in tab's collision state: a provider identity whose verified
// address belongs to an account it is not attached to is held as a pending
// link, and the person proves the account by its password (POST /auth/login
// with the pending-link cookie) or by its mailbox (OAuthLinkByEmailRoute,
// whose emailed link POST /auth/magic-link/verify finishes). These are the
// routes and the cookie that carry it.
const (
	OAuthPendingLinkRoute = "/auth/oauth/pending-link"
	OAuthLinkByEmailRoute = "/auth/oauth/link-by-email"
)

// authPageCodeOAuthLinkPending is the code the sign-in callback hands the auth
// page (authPageErrorCodeParam) when it holds the identity as a pending link.
// It is not a refusal and no AuthError carries it; the frontend reads it as
// AuthErrorCode.OAUTH_LINK_PENDING (frontend/lib/errors/authErrors.ts).
const authPageCodeOAuthLinkPending = "OAUTH_LINK_PENDING"

// linkStepProviders are the providers whose collisions land on the collision
// state. The state names the provider as Google, so any other provider keeps
// the refusal it had.
var linkStepProviders = []string{"google"}

// oauthPendingLinkCookie names the pending link for the browser that started
// it, for as long as the browser may use it. Its value is the signed token
// the auth service minted.
var oauthPendingLinkCookie = oauthHandshakeCookie{
	name:     "oauth_pending_link",
	lifetime: contracts.OAuthPendingLinkTTL,
}

func clearedPendingLinkCookie(secure bool) http.Cookie {
	return *newOAuthHandshakeCookie(oauthPendingLinkCookie.setName(secure), "", -1, secure)
}

// pendingLinkToken is the pending-link token from the cookie value a huma
// request bound under each of the two names it may be set under. Only the
// one matching the deployment's Secure setting counts.
func pendingLinkToken(bare, host string, secure bool) string {
	if secure {
		return host
	}
	return bare
}

// linkStepRequired is the refusal err holds when the callback for provider
// should land on the collision state rather than refuse.
func linkStepRequired(provider string, err error) (*contracts.OAuthLinkRequiredError, bool) {
	if !slices.Contains(linkStepProviders, provider) {
		return nil, false
	}
	var required *contracts.OAuthLinkRequiredError
	if !errors.As(err, &required) {
		return nil, false
	}
	return required, true
}

// startLinkStep answers a sign-in callback whose identity's verified address
// belongs to an account: it holds the identity as a pending link to that
// account and sends the browser to the auth page with
// authPageCodeOAuthLinkPending.
// The address stays out of the URL; the page reads it from
// OAuthPendingLinkRoute. When the pending link cannot be stored the browser
// gets the plain refusal instead.
func (h *OAuthHTTPHandler) startLinkStep(w http.ResponseWriter, r *http.Request, required *contracts.OAuthLinkRequiredError, returnTo, frontendURL string) {
	ctx := r.Context()
	secure := h.secureCookies()

	token, err := h.authService.StartOAuthLink(required.Identity, required.UserID, returnTo)
	if err != nil {
		logger.AuthError(ctx, "oauth_link_step_start_failed", err, "provider", required.Identity.Provider)
		http.Redirect(w, r, authPageRefusalURL(frontendURL, required.Refusal,
			refusalMessage(required.Refusal, "authentication failed"), returnTo), http.StatusTemporaryRedirect)
		return
	}
	oauthPendingLinkCookie.set(w, token, secure)
	logger.AuthInfo(ctx, "oauth_link_step_started", "provider", required.Identity.Provider, "user_id", required.UserID)
	query := url.Values{authPageErrorCodeParam: {authPageCodeOAuthLinkPending}}
	http.Redirect(w, r, authPageURLWithReturnTo(frontendURL, query, returnTo), http.StatusTemporaryRedirect)
}

// OAuthLinkStepHandler serves the collision state's read and its mailbox
// proof.
type OAuthLinkStepHandler struct {
	authService  contracts.AuthServiceInterface
	emailService contracts.EmailServiceInterface
	emailOutbox  contracts.AuthEmailOutboxInterface
	config       *config.Config
}

// NewOAuthLinkStepHandler creates the handler for the collision state.
func NewOAuthLinkStepHandler(
	authService contracts.AuthServiceInterface,
	emailService contracts.EmailServiceInterface,
	emailOutbox contracts.AuthEmailOutboxInterface,
	cfg *config.Config,
) *OAuthLinkStepHandler {
	return &OAuthLinkStepHandler{authService: authService, emailService: emailService, emailOutbox: emailOutbox, config: cfg}
}

func (h *OAuthLinkStepHandler) secureCookies() bool {
	return h != nil && h.config != nil && h.config.Session.Secure
}

// The states GET OAuthPendingLinkRoute answers with.
const (
	pendingLinkStatusPending = "pending"
	pendingLinkStatusExpired = "expired"
)

// PendingOAuthLinkRequest is the collision state's read of its pending link.
type PendingOAuthLinkRequest struct {
	Pending     string `cookie:"oauth_pending_link" doc:"Pending link, on a deployment without Secure cookies"`
	PendingHost string `cookie:"__Host-oauth_pending_link" doc:"Pending link, on a deployment with Secure cookies"`
}

// PendingOAuthLinkResponse is what the collision state renders from.
type PendingOAuthLinkResponse struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Status    string     `json:"status" enum:"pending,expired" doc:"pending: the collision state can be shown. expired: the pending link is expired, already used, or not this browser's."`
		Email     string     `json:"email,omitempty" doc:"The address the account and the provider share, when status is pending"`
		ExpiresAt *time.Time `json:"expires_at,omitempty" doc:"When the pending link expires, when status is pending. The browser's cookie that names it lapses at most 10 minutes after the Google sign-in that started it, which can be sooner once its emailed link has been asked for."`
	}
}

// PendingLinkHandler answers whether this browser holds a live pending link,
// and for one, the address to show. It reads only this browser's cookie, so
// the address never travels in a URL.
func (h *OAuthLinkStepHandler) PendingLinkHandler(ctx context.Context, input *PendingOAuthLinkRequest) (*PendingOAuthLinkResponse, error) {
	resp := &PendingOAuthLinkResponse{CacheControl: "no-store"}
	link, err := h.authService.PeekOAuthLink(pendingLinkToken(input.Pending, input.PendingHost, h.secureCookies()))
	if err != nil {
		if isLinkPendingExpired(err) {
			resp.Body.Status = pendingLinkStatusExpired
			return resp, nil
		}
		logger.AuthError(ctx, "oauth_pending_link_read_failed", err)
		return nil, autherrors.ErrServiceUnavailable("oauth_pending_link_read", err)
	}
	resp.Body.Status = pendingLinkStatusPending
	resp.Body.Email = link.Identity.Email
	expiresAt := link.ExpiresAt
	resp.Body.ExpiresAt = &expiresAt
	return resp, nil
}

// OAuthLinkByEmailRequest asks for the emailed link that proves the mailbox
// for this browser's pending link.
type OAuthLinkByEmailRequest struct {
	Pending     string `cookie:"oauth_pending_link" doc:"Pending link, on a deployment without Secure cookies"`
	PendingHost string `cookie:"__Host-oauth_pending_link" doc:"Pending link, on a deployment with Secure cookies"`
}

// OAuthLinkByEmailResponse answers the request.
type OAuthLinkByEmailResponse struct {
	SetCookie []http.Cookie `header:"Set-Cookie" doc:"Clears the pending-link cookie when the pending link is gone"`
	Body      struct {
		Success   bool   `json:"success" doc:"Whether the link was queued for the account's address"`
		Message   string `json:"message" doc:"Response message"`
		ErrorCode string `json:"error_code,omitempty" example:"OAUTH_LINK_PENDING_EXPIRED" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" doc:"Request ID for debugging"`
	}
}

// LinkByEmailHandler queues the sign-in link to the address of the account
// this browser's pending link names. Following it links the provider, turns
// off the account's password and signs out its other sessions
// (AuthServiceInterface.CompleteOAuthLinkByEmail). The browser already
// learned the address is registered from the callback, so the answer names
// the outcome plainly rather than in the enumeration-safe shape of
// /auth/magic-link/send.
func (h *OAuthLinkStepHandler) LinkByEmailHandler(ctx context.Context, input *OAuthLinkByEmailRequest) (*OAuthLinkByEmailResponse, error) {
	resp := &OAuthLinkByEmailResponse{}
	resp.Body.RequestID = logger.GetRequestID(ctx)
	secure := h.secureCookies()

	if !h.emailService.IsConfigured() {
		return h.unavailable(ctx, resp, "oauth_link_by_email_not_configured", errors.New("email service is not configured"))
	}
	link, err := h.authService.HoldOAuthLinkForEmail(pendingLinkToken(input.Pending, input.PendingHost, secure))
	if err != nil {
		if isLinkPendingExpired(err) {
			resp.SetCookie = append(resp.SetCookie, clearedPendingLinkCookie(secure))
			resp.Body.Success = false
			resp.Body.Message = autherrors.ToExternalMessage(autherrors.CodeOAuthLinkPendingExpired)
			resp.Body.ErrorCode = autherrors.CodeOAuthLinkPendingExpired
			logger.AuthWarn(ctx, "oauth_link_by_email_refused", "error_code", autherrors.CodeOAuthLinkPendingExpired)
			return resp, nil
		}
		return h.unavailable(ctx, resp, "oauth_link_by_email_read", err)
	}

	if err := h.emailOutbox.EnqueueOAuthLink(ctx, link.UserID, link.Ref); err != nil {
		return h.unavailable(ctx, resp, "oauth_link_by_email_enqueue", err)
	}

	logger.AuthInfo(ctx, "oauth_link_by_email_queued", "provider", link.Identity.Provider, "user_id", link.UserID)
	resp.Body.Success = true
	resp.Body.Message = "Check your inbox"
	return resp, nil
}

func (h *OAuthLinkStepHandler) unavailable(ctx context.Context, resp *OAuthLinkByEmailResponse, operation string, err error) (*OAuthLinkByEmailResponse, error) {
	logger.AuthError(ctx, operation+"_failed", err)
	resp.Body.Success = false
	resp.Body.Message = autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable)
	resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
	return resp, autherrors.ErrServiceUnavailable(operation, err)
}

func isLinkPendingExpired(err error) bool {
	var authErr *autherrors.AuthError
	return errors.As(err, &authErr) && authErr.Code == autherrors.CodeOAuthLinkPendingExpired
}
