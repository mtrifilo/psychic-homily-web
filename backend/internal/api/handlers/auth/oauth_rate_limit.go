package auth

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// AuthPageRateLimitedCode is the error code a refused OAuth redirect hands the
// page it sends the browser to: the frontend auth page's error_code, or the
// Settings link result's oauth_link_error.
const AuthPageRateLimitedCode = "RATE_LIMITED"

// RedirectRateLimited answers an OAuth start or callback that its rate limit
// refused. The browser reaches both by top-level navigation, so whatever this
// writes is the page the reader is left on: never a JSON body.
//
// A request carrying a pending link intent for its own provider and state,
// which is what a link's callback carries and what OAuthCallbackHTTPHandler
// matches on, goes back to Settings with the code, where every other link
// result is read. Everything else goes to the frontend auth page with message
// and the code, the query keys a refused sign-in callback uses.
func (h *OAuthHTTPHandler) RedirectRateLimited(w http.ResponseWriter, r *http.Request, message string) {
	if completesPendingLink(r) {
		redirectToLinkResult(w, r, h.frontendURL(), AuthPageRateLimitedCode)
		return
	}
	query := url.Values{
		authPageErrorParam:     {message},
		authPageErrorCodeParam: {AuthPageRateLimitedCode},
	}
	http.Redirect(w, r, h.frontendURL()+"/auth?"+query.Encode(), http.StatusTemporaryRedirect)
}

// completesPendingLink reports whether r carries a link intent for its own
// provider and state. It only reads the intent; the callback consumes it.
func completesPendingLink(r *http.Request) bool {
	cookie, err := r.Cookie(oauthLinkIntentCookieName)
	if err != nil {
		return false
	}
	intent, found := peekOAuthLinkIntent(cookie.Value)
	return found &&
		intent.provider == chi.URLParam(r, "provider") &&
		intent.state == r.URL.Query().Get("state")
}
