package auth

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// The routes the OAuth handlers serve, as chi registers them. The router
// registers each handler on its template here, and RedirectRateLimited reads
// the template a refused request matched to decide where to send it.
const (
	OAuthLoginRoute    = "/auth/login/{provider}"
	OAuthCallbackRoute = "/auth/callback/{provider}"
	OAuthLinkRoute     = "/auth/link/{provider}"
)

// OAuthRedirectRateLimitedCode is the code RedirectRateLimited reports: as
// error_code on the frontend auth page, or as oauth_link_error on the Settings
// link result.
const OAuthRedirectRateLimitedCode = "RATE_LIMITED"

// RedirectRateLimited answers an OAuth start, callback or link start that its
// rate limit refused. The browser reaches all three by top-level navigation,
// so whatever this writes is the page the reader is left on: never a JSON body.
//
// A link start, and a callback carrying a pending link intent armed for its
// own provider and state (isForHandshake, the test OAuthCallbackHTTPHandler
// applies), go back to Settings with the code, where every other link result
// is read. Everything else goes to the frontend auth page with message and the
// code, the query keys a refused sign-in callback uses, and with the sign-in's
// destination where the handler it stands in for would have read one: a
// sign-in start's from its query, a callback's from the cookie its start set,
// which the refusal consumes as the callback would have.
//
// It reads chi's route pattern and provider parameter, so it runs on a
// middleware chain chi builds after routing.
func (h *OAuthHTTPHandler) RedirectRateLimited(w http.ResponseWriter, r *http.Request, message string) {
	if startsLink(r) || completesPendingLink(r) {
		redirectToLinkResult(w, r, h.frontendURL(), OAuthRedirectRateLimitedCode)
		return
	}

	returnTo := oauthReturnToFallback
	switch chi.RouteContext(r.Context()).RoutePattern() {
	case OAuthLoginRoute:
		returnTo = sanitizeOAuthReturnTo(r.URL.Query().Get(oauthReturnToParam))
	case OAuthCallbackRoute:
		returnTo = takeOAuthReturnTo(w, r, h.config != nil && h.config.Session.Secure)
	}

	query := url.Values{
		authPageErrorParam:     {message},
		authPageErrorCodeParam: {OAuthRedirectRateLimitedCode},
	}
	http.Redirect(w, r, authPageURLWithReturnTo(h.frontendURL(), query, returnTo), http.StatusTemporaryRedirect)
}

// startsLink reports whether r was routed to OAuthLinkRoute, the start a
// signed-in reader reaches from Settings.
func startsLink(r *http.Request) bool {
	return chi.RouteContext(r.Context()).RoutePattern() == OAuthLinkRoute
}

// completesPendingLink reports whether r carries a link intent armed for its
// own provider and state. It only reads the intent; the callback consumes it.
func completesPendingLink(r *http.Request) bool {
	cookie, err := r.Cookie(oauthLinkIntentCookieName)
	if err != nil {
		return false
	}
	intent, found := peekOAuthLinkIntent(cookie.Value)
	return found && intent.isForHandshake(chi.URLParam(r, "provider"), r.URL.Query().Get("state"))
}
