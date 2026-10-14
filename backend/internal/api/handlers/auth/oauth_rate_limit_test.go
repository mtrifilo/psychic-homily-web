package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"psychic-homily-backend/internal/config"
)

func TestRedirectRateLimitedSendsTheBrowserToTheConfiguredAuthPage(t *testing.T) {
	const message = "Rate limit exceeded. Please try again in 60 seconds."
	h := NewOAuthHTTPHandler(nil, nil, &config.Config{Email: config.EmailConfig{FrontendURL: "https://psychichomily.com"}})
	rr := httptest.NewRecorder()
	h.RedirectRateLimited(rr, httptest.NewRequest(http.MethodGet, "/auth/callback/google?code=spent", nil), message)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusTemporaryRedirect)
	}
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location %q does not parse: %v", rr.Header().Get("Location"), err)
	}
	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, "https://psychichomily.com/auth"; got != want {
		t.Errorf("redirects to %q, want %q", got, want)
	}
	want := url.Values{
		authPageErrorParam:     {message},
		authPageErrorCodeParam: {AuthPageRateLimitedCode},
	}
	if got := loc.Query(); got.Encode() != want.Encode() {
		t.Errorf("query = %q, want exactly %q", got.Encode(), want.Encode())
	}
}
