package auth

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

func TestCheckSignupAge(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	// At 12:00 UTC it is already the next day at UTC+14, the latest date
	// anywhere, so a 16th birthday that is tomorrow in UTC counts today, and
	// only one two days ahead is still under the minimum.
	sixteenToday := now.AddDate(-legal.MinSignupAge, 0, 0).Format(legal.DateOfBirthLayout)
	sixteenTomorrowInUTC := now.AddDate(-legal.MinSignupAge, 0, 1).Format(legal.DateOfBirthLayout)
	sixteenAfterLatestDate := now.AddDate(-legal.MinSignupAge, 0, 2).Format(legal.DateOfBirthLayout)

	cases := []struct {
		value   string
		want    signupAgeOutcome
		wantAge int
	}{
		{"", signupAgeMissing, 0},
		{"1990-13-01", signupAgeImplausible, 0},
		{"2001-02-29", signupAgeImplausible, 0},
		{"1990/01/31", signupAgeImplausible, 0},
		{"2026-10-30", signupAgeImplausible, 0},
		{"1870-01-01", signupAgeImplausible, 0},
		{sixteenAfterLatestDate, signupAgeUnderMinimum, 0},
		{sixteenTomorrowInUTC, signupAgeAccepted, legal.MinSignupAge},
		{sixteenToday, signupAgeAccepted, legal.MinSignupAge},
		{"1990-06-15", signupAgeAccepted, 36},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, outcome := checkSignupAge(tc.value, now)
			if outcome != tc.want {
				t.Fatalf("outcome = %d, want %d", outcome, tc.want)
			}
			if got.age != tc.wantAge {
				t.Errorf("age = %d, want %d", got.age, tc.wantAge)
			}
			if outcome == signupAgeAccepted && !got.attestedAt.Equal(now) {
				t.Errorf("attestedAt = %v, want %v", got.attestedAt, now)
			}
		})
	}
}

func TestSignupAgeRecordsTheComputedAge(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	age, outcome := checkSignupAge("1990-06-15", now)
	if outcome != signupAgeAccepted {
		t.Fatalf("outcome = %d", outcome)
	}
	acceptance := age.legalAcceptance("t", "p")
	if !acceptanceAttestsAge(acceptance) {
		t.Errorf("acceptance %+v does not attest the age it was built from", acceptance)
	}
	if acceptance.MinAgeAttested != 36 || acceptance.DateOfBirth == nil ||
		acceptance.DateOfBirth.Format(legal.DateOfBirthLayout) != "1990-06-15" ||
		!acceptance.AgeConfirmedAt.Equal(now) || !acceptance.TermsAcceptedAt.Equal(now) {
		t.Errorf("acceptance = %+v", acceptance)
	}
	consent := age.oauthSignupConsent("t", "p")
	if !consent.AgeConfirmed || consent.MinAgeAttested != 36 || consent.DateOfBirth == nil || !consent.AcceptedAt.Equal(now) {
		t.Errorf("consent = %+v", consent)
	}
}

// --- over the wire ---

// legacyAgeFields are the attestation fields an older client sends in place
// of a date of birth.
var legacyAgeFields = map[string]any{"age_confirmed": true, "min_age_attested": legal.MinSignupAge}

func withBody(base map[string]any, changes map[string]any, drop ...string) map[string]any {
	body := maps.Clone(base)
	maps.Copy(body, changes)
	for _, key := range drop {
		delete(body, key)
	}
	return body
}

func registerAgeWireAPI(t *testing.T, created *bool) humatest.TestAPI {
	t.Helper()
	return newRegisterWireAPI(t, &testhelpers.MockUserService{
		CreateUserWithPasswordWithLegalFn: func(email, _, _, _ string, _ contracts.LegalAcceptance) (*authm.User, error) {
			*created = true
			return &authm.User{ID: 1, Email: &email}, nil
		},
	})
}

func passkeyBeginAgeWireAPI(t *testing.T, began *bool) humatest.TestAPI {
	t.Helper()
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationForEmailFn: func(string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			*began = true
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		StoreSignupChallengeFn: func(contracts.SignupChallenge, *webauthn.SessionData, string) (string, error) { return "c-1", nil },
	}
	users := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
	return newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, users))
}

// signupAgeWirePaths are the two sign-up requests that take a date of birth
// in their body, each with a body that passes every other check.
func signupAgeWirePaths(t *testing.T) map[string]struct {
	route   string
	body    map[string]any
	api     humatest.TestAPI
	created *bool
} {
	versions := map[string]any{"terms_version": legal.CurrentTermsVersion, "privacy_version": legal.CurrentPrivacyVersion}
	var registered, began bool
	return map[string]struct {
		route   string
		body    map[string]any
		api     humatest.TestAPI
		created *bool
	}{
		"register":             {"/auth/register", withBody(registerConsentBody, versions), registerAgeWireAPI(t, &registered), &registered},
		"passkey signup/begin": {"/auth/passkey/signup/begin", withBody(passkeySignupBeginBody, versions), passkeyBeginAgeWireAPI(t, &began), &began},
	}
}

func requireAgeRefusal(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d, want the 200 refusal: %s", resp.Code, resp.Body.String())
	}
	body := decodeWireBody(t, resp.Body.Bytes())
	if body["success"] != false || body["error_code"] != autherrors.CodeAgeConfirmationRequired {
		t.Fatalf("body %s, want a refusal coded %s", resp.Body.String(), autherrors.CodeAgeConfirmationRequired)
	}
}

func markerCookieIn(resp *httptest.ResponseRecorder) string {
	for _, header := range resp.Header().Values("Set-Cookie") {
		if strings.HasPrefix(header, signupAgeRefusalCookieName+"=") {
			return header
		}
	}
	return ""
}

// An older client that attests its age with the legacy fields and sends no
// date of birth is refused with AGE_CONFIRMATION_REQUIRED, not refused by the
// schema, and no account or ceremony starts.
func TestSignupAgeWire_LegacyAttestationIsRefused(t *testing.T) {
	for name, path := range signupAgeWirePaths(t) {
		t.Run(name, func(t *testing.T) {
			resp := path.api.Post(path.route, withBody(path.body, legacyAgeFields, "date_of_birth"))
			requireAgeRefusal(t, resp)
			if *path.created {
				t.Error("the sign-up went ahead")
			}
			if marker := markerCookieIn(resp); marker != "" {
				t.Errorf("set %q; a request that gave no age leaves no marker", marker)
			}
		})
	}
}

// An under-age date sets the marker, and the marker then refuses an adult
// date from the same browser.
func TestSignupAgeWire_UnderAgeMarkerRefusesTheNextAttempt(t *testing.T) {
	for name, path := range signupAgeWirePaths(t) {
		t.Run(name, func(t *testing.T) {
			refused := path.api.Post(path.route, withBody(path.body, map[string]any{
				"date_of_birth": signupconsent.UnderAgeDateOfBirth(time.Now()),
			}))
			requireAgeRefusal(t, refused)
			marker := markerCookieIn(refused)
			if marker == "" {
				t.Fatalf("no refusal marker in %v", refused.Header().Values("Set-Cookie"))
			}
			if !strings.Contains(marker, "Max-Age=86400") || !strings.Contains(marker, "HttpOnly") {
				t.Errorf("marker %q, want HttpOnly for a day", marker)
			}

			retry := path.api.Post(path.route, "Cookie: "+signupAgeRefusalCookieName+"=1", path.body)
			requireAgeRefusal(t, retry)
			if *path.created {
				t.Error("the retry with an adult date went ahead")
			}
		})
	}
}

// The marker is read from each sign-up request's own cookie fields (huma binds
// none declared on an embedded struct), under both names a deployment may set
// it under; this holds those copies to the names the marker is set under.
func TestSignupRequestsDeclareTheMarkerCookie(t *testing.T) {
	want := []string{signupAgeRefusalCookieNameFor(false), signupAgeRefusalCookieNameFor(true)}
	for name, request := range map[string]any{
		"RegisterRequest":     RegisterRequest{},
		"BeginSignupRequest":  BeginSignupRequest{},
		"FinishSignupRequest": FinishSignupRequest{},
	} {
		var tags []string
		typ := reflect.TypeOf(request)
		for i := 0; i < typ.NumField(); i++ {
			if tag := typ.Field(i).Tag.Get("cookie"); tag != "" {
				tags = append(tags, tag)
			}
		}
		if !reflect.DeepEqual(tags, want) {
			t.Errorf("%s cookies = %v, want %v", name, tags, want)
		}
	}
}
