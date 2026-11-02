package auth

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	autherrors "psychic-homily-backend/internal/errors"
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

// A re-authentication records its reason beside the destination, and records
// it without one too, so a refusal reopens the re-authentication either way.
func TestOAuthLogin_RecordsTheReauthReason(t *testing.T) {
	for _, reason := range []string{reasonOAuthLinkReauth, reasonCredentialMint} {
		for _, tc := range []struct{ returnTo, recordedPath string }{
			{reauthSettingsReturnTo, reauthSettingsReturnTo},
			{"", "/"},
		} {
			t.Run(reason+"/"+tc.recordedPath, func(t *testing.T) {
				resp := startSignIn(startQuery(tc.returnTo, reason))

				if got, want := returnToCookieValue(t, resp), encodedReturnToWithReason(tc.recordedPath, reason); got != want {
					t.Errorf("cookie value = %q, want %q", got, want)
				}
			})
		}
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

// reasonCallbackCase is one of the callback's redirects to the auth page and
// the code it carries.
type reasonCallbackCase struct {
	name     string
	handler  *OAuthHTTPHandler
	query    string
	wantCode string
}

// reasonCallbackCases are the callback's redirects to the auth page, one per
// writer.
func reasonCallbackCases(t *testing.T) []reasonCallbackCase {
	return []reasonCallbackCase{
		{"refusal with its own code", failingCallbackHandler(autherrors.ErrTermsAcceptanceRequired("no consent")), "state=s&code=c", autherrors.CodeTermsAcceptanceRequired},
		{"provider fault", failingCallbackHandler(errors.New("oauth2: cannot fetch token")), "state=s&code=c", autherrors.CodeUnknown},
		{"declined at Google", declineHandler(t), "error=access_denied&state=s", authPageCodeOAuthCancelled},
		{"sign-up step store failure", signupStepStoreFailureHandler(), "state=s&code=c", autherrors.CodeUnknown},
		{"collision store failure", linkStepHandler("", errors.New("pending link insert failed")), "state=s&code=c", autherrors.CodeOAuthLinkRefused},
		{"collision held as a pending link", linkStepHandler("pending-link-token", nil), "state=s&code=c", authPageCodeOAuthLinkPending},
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
	recorded := returnToCookieValue(t, startSignIn(startQuery(reauthSettingsReturnTo, reasonOAuthLinkReauth)))

	resp, raw := runCallback(t, failingCallbackHandler(errors.New("gothic: state mismatch")), "/auth/callback/google?state=tampered&code=c",
		&http.Cookie{Name: oauthReturnToCookieName, Value: recorded})

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
