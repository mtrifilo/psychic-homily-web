package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"psychic-homily-backend/internal/config"
)

func TestRedirectRateLimitedSendsTheBrowserToTheAuthPage(t *testing.T) {
	const message = "Rate limit exceeded. Please try again in 60 seconds."
	for _, tc := range []struct {
		name        string
		frontendURL string
		wantPage    string
	}{
		{"configured frontend", "https://psychichomily.com", "https://psychichomily.com/auth"},
		{"unset frontend", "", "http://localhost:3000/auth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewOAuthHTTPHandler(nil, nil, &config.Config{Email: config.EmailConfig{FrontendURL: tc.frontendURL}})
			rr := httptest.NewRecorder()
			h.RedirectRateLimited(rr, httptest.NewRequest(http.MethodGet, "/auth/callback/google?code=spent", nil), message)

			if rr.Code != http.StatusTemporaryRedirect {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusTemporaryRedirect)
			}
			loc, err := url.Parse(rr.Header().Get("Location"))
			if err != nil {
				t.Fatalf("Location %q does not parse: %v", rr.Header().Get("Location"), err)
			}
			if got := loc.Scheme + "://" + loc.Host + loc.Path; got != tc.wantPage {
				t.Errorf("redirects to %q, want %q", got, tc.wantPage)
			}
			want := url.Values{
				authPageErrorParam:     {message},
				authPageErrorCodeParam: {AuthPageRateLimitedCode},
			}
			if got := loc.Query(); got.Encode() != want.Encode() {
				t.Errorf("query = %q, want exactly %q", got.Encode(), want.Encode())
			}
		})
	}
}
