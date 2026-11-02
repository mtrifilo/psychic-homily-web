package auth

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// The re-authentication reasons as the frontend spells them
// (REAUTH_REASON_OAUTH_LINK and REAUTH_REASON_CREDENTIAL_MINT in
// frontend/lib/auth-href.ts). Literals, not the constants: the start reads and
// the auth page writes exactly these values.
const (
	reasonOAuthLinkReauth = "OAUTH_LINK_REAUTH_REQUIRED"
	reasonCredentialMint  = "REAUTH_REQUIRED"
)

// The destination the Settings re-authentication carries.
const reauthSettingsReturnTo = "/profile?tab=settings"

func startQuery(returnTo, reason string) string {
	query := url.Values{}
	if returnTo != "" {
		query.Set("returnTo", returnTo)
	}
	if reason != "" {
		query.Set("reason", reason)
	}
	return query.Encode()
}

func encodedReturnToWithReason(path, reason string) string {
	return encodedReturnTo(path) + "." + reason
}

func returnToCookieValue(t *testing.T, resp *http.Response) string {
	t.Helper()
	cookie := findCookie(resp, oauthReturnToCookieName)
	if cookie == nil {
		t.Fatal("expected the return-to cookie to be set")
	}
	return cookie.Value
}

func TestOAuthLogin_RecordsTheReauthReasonBesideTheDestination(t *testing.T) {
	for _, reason := range []string{reasonOAuthLinkReauth, reasonCredentialMint} {
		t.Run(reason, func(t *testing.T) {
			resp := startSignIn(startQuery(reauthSettingsReturnTo, reason))

			if got, want := returnToCookieValue(t, resp), encodedReturnToWithReason(reauthSettingsReturnTo, reason); got != want {
				t.Errorf("cookie value = %q, want %q", got, want)
			}
		})
	}
}

// A re-authentication that names no destination still records its reason, so
// a refusal reopens the re-authentication.
func TestOAuthLogin_RecordsAReauthReasonWithoutADestination(t *testing.T) {
	resp := startSignIn(startQuery("", reasonCredentialMint))

	if got, want := returnToCookieValue(t, resp), encodedReturnToWithReason("/", reasonCredentialMint); got != want {
		t.Errorf("cookie value = %q, want %q", got, want)
	}
}

// A reason that is not a re-authentication is dropped at the start: with a
// destination the cookie carries the destination alone, and without one the
// start records nothing and clears what an earlier start left.
func TestOAuthLogin_DropsAReasonThatIsNotAReauth(t *testing.T) {
	for _, reason := range []string{
		autherrors.CodeAccountInactive,
		"reauth_required",
		reasonCredentialMint + ".x",
		reasonCredentialMint + " ",
		"<script>",
	} {
		t.Run(reason, func(t *testing.T) {
			resp := startSignIn(startQuery("/shows", reason))
			if got, want := returnToCookieValue(t, resp), encodedReturnTo("/shows"); got != want {
				t.Errorf("cookie value = %q, want the destination alone %q", got, want)
			}

			stale := &http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnToWithReason("/shows", reasonCredentialMint)}
			assertReturnToCleared(t, startSignIn(startQuery("", reason), stale))
		})
	}
}

// reasonCallbackCases are the callback's redirects to the auth page, one per
// writer, each with the code it carries.
func reasonCallbackCases(t *testing.T) []struct {
	name     string
	handler  *OAuthHTTPHandler
	query    string
	wantCode string
} {
	linkRequired := func() error {
		return &contracts.OAuthLinkRequiredError{
			Identity: contracts.OAuthIdentity{Provider: "google", Subject: "sub"},
			UserID:   3,
			Refusal:  autherrors.ErrOAuthLinkRefused("taken@example.com"),
		}
	}
	return []struct {
		name     string
		handler  *OAuthHTTPHandler
		query    string
		wantCode string
	}{
		{
			name:     "refusal with its own code",
			handler:  failingCallbackHandler(autherrors.ErrTermsAcceptanceRequired("no consent")),
			query:    "state=s&code=c",
			wantCode: autherrors.CodeTermsAcceptanceRequired,
		},
		{
			name:     "provider fault",
			handler:  failingCallbackHandler(errors.New("oauth2: cannot fetch token")),
			query:    "state=s&code=c",
			wantCode: autherrors.CodeUnknown,
		},
		{
			name:     "declined at Google",
			handler:  declineHandler(t),
			query:    "error=access_denied&state=s",
			wantCode: authPageCodeOAuthCancelled,
		},
		{
			name: "sign-up step store failure",
			handler: callbackHandlerFor(&testhelpers.MockAuthService{
				OAuthCallbackFn: func(http.ResponseWriter, *http.Request, string) (*authm.User, string, error) {
					return nil, "", &contracts.OAuthSignupRequiredError{
						Identity: contracts.OAuthIdentity{Provider: "google", Subject: "sub"},
						Refusal:  autherrors.ErrTermsAcceptanceRequired("new oauth identity arrived without consent"),
					}
				},
				StartOAuthSignupFn: func(contracts.OAuthIdentity, string) (string, error) {
					return "", errors.New("pending sign-up insert failed")
				},
			}),
			query:    "state=s&code=c",
			wantCode: autherrors.CodeUnknown,
		},
		{
			name: "collision store failure",
			handler: callbackHandlerFor(&testhelpers.MockAuthService{
				OAuthCallbackFn: func(http.ResponseWriter, *http.Request, string) (*authm.User, string, error) {
					return nil, "", linkRequired()
				},
				StartOAuthLinkFn: func(contracts.OAuthIdentity, uint, string) (string, error) {
					return "", errors.New("pending link insert failed")
				},
			}),
			query:    "state=s&code=c",
			wantCode: autherrors.CodeOAuthLinkRefused,
		},
		{
			name: "collision held as a pending link",
			handler: callbackHandlerFor(&testhelpers.MockAuthService{
				OAuthCallbackFn: func(http.ResponseWriter, *http.Request, string) (*authm.User, string, error) {
					return nil, "", linkRequired()
				},
				StartOAuthLinkFn: func(contracts.OAuthIdentity, uint, string) (string, error) {
					return "pending-link-token", nil
				},
			}),
			query:    "state=s&code=c",
			wantCode: authPageCodeOAuthLinkPending,
		},
	}
}

// Every redirect the callback sends to the auth page carries the reason its
// start recorded beside the code and the destination, so the page reopens as
// the re-authentication it was.
func TestOAuthCallback_EveryAuthPageRedirectCarriesTheReauthReason(t *testing.T) {
	for _, reason := range []string{reasonOAuthLinkReauth, reasonCredentialMint} {
		for _, tc := range reasonCallbackCases(t) {
			t.Run(reason+"/"+tc.name, func(t *testing.T) {
				req := callbackRequest("google", tc.query)
				req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnToWithReason(reauthSettingsReturnTo, reason)})

				query := errorRedirectQuery(t, tc.handler, req, errorRedirectFrontendURL+"/auth")
				want := url.Values{"error_code": {tc.wantCode}, "reason": {reason}, "returnTo": {reauthSettingsReturnTo}}
				if query.Encode() != want.Encode() {
					t.Errorf("query = %q, want exactly %q", query.Encode(), want.Encode())
				}
			})
		}
	}
}

// A sign-in that recorded no reason reaches the auth page without one. The
// callback URL is the provider's, so a reason on it is not read.
func TestOAuthCallback_AuthPageRedirectWithoutAReasonCarriesNone(t *testing.T) {
	for _, tc := range reasonCallbackCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			req := callbackRequest("google", tc.query+"&reason="+reasonCredentialMint)
			req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo(reauthSettingsReturnTo)})

			query := errorRedirectQuery(t, tc.handler, req, errorRedirectFrontendURL+"/auth")
			if query.Has("reason") {
				t.Errorf("reason = %q, want the parameter absent", query.Get("reason"))
			}
		})
	}
}

// The cookie the start writes is the one the callback reads: a re-auth start
// whose callback is refused lands on the auth page with the code, the reason
// and the destination.
func TestOAuthReauth_RoundTripCarriesTheReasonToARefusal(t *testing.T) {
	start := startSignIn(startQuery(reauthSettingsReturnTo, reasonOAuthLinkReauth))
	cookie := findCookie(start, oauthReturnToCookieName)
	if cookie == nil {
		t.Fatal("the start recorded nothing")
	}

	resp, raw := runCallback(t, failingCallbackHandler(errors.New("gothic: state mismatch")), "/auth/callback/google?state=tampered&code=c",
		&http.Cookie{Name: cookie.Name, Value: cookie.Value})

	location, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("unparseable Location %q: %v", raw, err)
	}
	want := url.Values{"error_code": {autherrors.CodeUnknown}, "reason": {reasonOAuthLinkReauth}, "returnTo": {reauthSettingsReturnTo}}
	if location.Path != "/auth" || location.Query().Encode() != want.Encode() {
		t.Errorf("Location = %q, want the auth page with exactly %q", raw, want.Encode())
	}
	assertReturnToCleared(t, resp)
}

// A sign-in that succeeds has satisfied the re-authentication, so it lands on
// the destination alone.
func TestOAuthCallback_SuccessDropsTheReauthReason(t *testing.T) {
	cases := []struct {
		name   string
		cookie string
		want   string
	}{
		{"with a destination", encodedReturnToWithReason(reauthSettingsReturnTo, reasonOAuthLinkReauth), errorRedirectFrontendURL + reauthSettingsReturnTo},
		{"without a destination", encodedReturnToWithReason("/", reasonCredentialMint), errorRedirectFrontendURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, location := runCallback(t, signInCallbackHandler(), "/auth/callback/google",
				&http.Cookie{Name: oauthReturnToCookieName, Value: tc.cookie})

			if location != tc.want {
				t.Errorf("Location = %q, want %q", location, tc.want)
			}
			assertReturnToCleared(t, resp)
		})
	}
}

// The browser can send any cookie value, so the callback checks both parts
// again: a reason that is not a re-authentication is dropped and the
// destination kept, and a destination that does not decode drops the reason
// with it.
func TestOAuthCallback_RechecksTheRecordedReason(t *testing.T) {
	cases := []struct {
		name         string
		cookie       string
		wantReason   string
		wantReturnTo string
	}{
		{"closed-account reason", encodedReturnToWithReason("/shows", autherrors.CodeAccountInactive), "", "/shows"},
		{"unknown reason", encodedReturnToWithReason("/shows", "SOMETHING_ELSE"), "", "/shows"},
		{"reason with a second separator", encodedReturnToWithReason("/shows", reasonCredentialMint+".x"), "", "/shows"},
		{"undecodable destination", "not base64!." + reasonCredentialMint, "", ""},
		{"refused destination", encodedReturnToWithReason("//evil.example", reasonCredentialMint), reasonCredentialMint, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := callbackRequest("google", "state=s&code=c")
			req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: tc.cookie})

			query := errorRedirectQuery(t, failingCallbackHandler(autherrors.ErrTermsAcceptanceRequired("no consent")), req, errorRedirectFrontendURL+"/auth")
			want := url.Values{"error_code": {autherrors.CodeTermsAcceptanceRequired}}
			if tc.wantReason != "" {
				want.Set("reason", tc.wantReason)
			}
			if tc.wantReturnTo != "" {
				want.Set("returnTo", tc.wantReturnTo)
			}
			if query.Encode() != want.Encode() {
				t.Errorf("query = %q, want exactly %q", query.Encode(), want.Encode())
			}
		})
	}
}

// A rate-limited start or callback is refused like the handler it stands in
// for: a start carries the reason its query names, a callback the reason its
// start recorded.
func TestRedirectRateLimitedCarriesTheReauthReason(t *testing.T) {
	assertRateLimitedQuery := func(t *testing.T, loc *url.URL, want url.Values) {
		t.Helper()
		if loc.Path != "/auth" || loc.Query().Encode() != want.Encode() {
			t.Errorf("Location = %q, want the auth page with exactly %q", loc.String(), want.Encode())
		}
	}

	loc := refuse(t, OAuthLoginRoute, "/auth/login/google?"+startQuery(reauthSettingsReturnTo, reasonOAuthLinkReauth), "google")
	assertRateLimitedQuery(t, loc, url.Values{"error_code": {"RATE_LIMITED"}, "reason": {reasonOAuthLinkReauth}, "returnTo": {reauthSettingsReturnTo}})

	loc = refuse(t, OAuthLoginRoute, "/auth/login/google?"+startQuery(reauthSettingsReturnTo, autherrors.CodeAccountInactive), "google")
	assertRateLimitedQuery(t, loc, url.Values{"error_code": {"RATE_LIMITED"}, "returnTo": {reauthSettingsReturnTo}})

	rr := refuseRecorded(t, OAuthCallbackRoute, "/auth/callback/google?state=s&code=spent&reason=REAUTH_REQUIRED", "google",
		&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnToWithReason("/shows", reasonOAuthLinkReauth)})
	loc, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location %q does not parse: %v", rr.Header().Get("Location"), err)
	}
	assertRateLimitedQuery(t, loc, url.Values{"error_code": {"RATE_LIMITED"}, "reason": {reasonOAuthLinkReauth}, "returnTo": {"/shows"}})
	if !clearsReturnToCookie(rr) {
		t.Error("the refused callback left the return-to cookie set; it is single use")
	}

	// The callback's own query is the provider's, so a reason on it is not read.
	loc = refuse(t, OAuthCallbackRoute, "/auth/callback/google?state=s&code=spent&reason=REAUTH_REQUIRED", "google")
	assertRateLimitedQuery(t, loc, url.Values{"error_code": {"RATE_LIMITED"}})
}
