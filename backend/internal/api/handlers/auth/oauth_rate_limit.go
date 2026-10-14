package auth

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// OAuthRedirectRateLimitedCode is the code RedirectRateLimited reports: as
// error_code on the frontend auth page, or as oauth_link_error on the Settings
// link result.
const OAuthRedirectRateLimitedCode = "RATE_LIMITED"

// RedirectRateLimited answers an OAuth start or callback that its rate limit
// refused. The browser reaches both by top-level navigation, so whatever this
// writes is the page the reader is left on: never a JSON body.
//
// A request carrying a pending link intent armed for its own provider and
// state (isForHandshake, the test OAuthCallbackHTTPHandler applies) goes back
// to Settings with the code, where every other link result is read. Everything
// else goes to the frontend auth page with message and the code, the query
// keys a refused sign-in callback uses.
//
// It reads chi's provider parameter, so it runs on a middleware chain chi
// builds after routing.
func (h *OAuthHTTPHandler) RedirectRateLimited(w http.ResponseWriter, r *http.Request, message string) {
	if completesPendingLink(r) {
		redirectToLinkResult(w, r, h.frontendURL(), OAuthRedirectRateLimitedCode)
		return
	}
	query := url.Values{
		authPageErrorParam:     {message},
		authPageErrorCodeParam: {OAuthRedirectRateLimitedCode},
	}
	http.Redirect(w, r, h.frontendURL()+"/auth?"+query.Encode(), http.StatusTemporaryRedirect)
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
