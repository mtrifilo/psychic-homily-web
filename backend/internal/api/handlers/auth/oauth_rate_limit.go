package auth

import (
	"net/http"
	"net/url"
)

// AuthPageRateLimitedCode is the error_code the frontend auth page receives
// when a sign-in redirect was refused by its rate limit.
const AuthPageRateLimitedCode = "RATE_LIMITED"

// RedirectRateLimited answers a sign-in redirect that its rate limit refused.
// The browser reaches the start and the callback by top-level navigation, so
// whatever this writes is the page the reader is left on: it sends them to the
// frontend auth page carrying message and AuthPageRateLimitedCode, the same
// query keys a refused callback uses, instead of a JSON body.
func (h *OAuthHTTPHandler) RedirectRateLimited(w http.ResponseWriter, r *http.Request, message string) {
	query := url.Values{
		authPageErrorParam:     {message},
		authPageErrorCodeParam: {AuthPageRateLimitedCode},
	}
	http.Redirect(w, r, h.frontendURL()+"/auth?"+query.Encode(), http.StatusTemporaryRedirect)
}
