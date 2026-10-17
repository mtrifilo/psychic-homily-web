package system

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"psychic-homily-backend/internal/legal"
)

func serveLegalVersions(t *testing.T) (*http.Response, map[string]json.RawMessage) {
	t.Helper()
	_, api := humatest.New(t)
	huma.Get(api, LegalVersionsRoute, LegalVersionsHandler)

	resp := api.Get(LegalVersionsRoute)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", LegalVersionsRoute, resp.Code, resp.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return resp.Result(), body
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "$schema" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// The wire answer with the configuration this package ships: the current
// versions and the minimum age the sign-up validators enforce, and the grace
// fields only while legal.GraceWindow reports a window open.
func TestLegalVersionsServesWhatTheValidatorsEnforce(t *testing.T) {
	resp, body := serveLegalVersions(t)

	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=300" {
		t.Errorf("Cache-Control = %q, want %q", got, "public, max-age=300")
	}

	var terms, privacy string
	var age int
	if err := json.Unmarshal(body["terms_version"], &terms); err != nil || terms != legal.CurrentTermsVersion {
		t.Errorf("terms_version = %s, want %q", body["terms_version"], legal.CurrentTermsVersion)
	}
	if err := json.Unmarshal(body["privacy_version"], &privacy); err != nil || privacy != legal.CurrentPrivacyVersion {
		t.Errorf("privacy_version = %s, want %q", body["privacy_version"], legal.CurrentPrivacyVersion)
	}
	if err := json.Unmarshal(body["min_signup_age"], &age); err != nil || age != legal.MinSignupAge {
		t.Errorf("min_signup_age = %s, want %d", body["min_signup_age"], legal.MinSignupAge)
	}
	if !legal.AcceptedVersions(terms, privacy, time.Now()) {
		t.Errorf("the published pair (%q, %q) is refused by the sign-up validators", terms, privacy)
	}

	if _, _, open := legal.GraceWindow(time.Now()); open {
		if string(body["grace_until"]) == "null" {
			t.Error("a grace window is open but grace_until is null")
		}
		if _, ok := body["previous"]; !ok {
			t.Error("a grace window is open but previous is absent")
		}
		return
	}
	want := []string{"grace_until", "min_signup_age", "privacy_version", "terms_version"}
	if got := sortedKeys(body); !reflect.DeepEqual(got, want) {
		t.Errorf("fields = %v, want %v (previous absent while no window is open)", got, want)
	}
	if string(body["grace_until"]) != "null" {
		t.Errorf("grace_until = %s, want null while no window is open", body["grace_until"])
	}
}

func TestLegalVersionsPublishesAnOpenWindow(t *testing.T) {
	phoenix := time.FixedZone("MST", -7*60*60)
	until := time.Date(2026, time.October, 31, 17, 0, 0, 0, phoenix)
	previous := legal.Versions{Terms: "2025-12-01", Privacy: "2025-12-02"}
	var askedAt time.Time
	now := time.Date(2026, time.October, 30, 0, 0, 0, 0, time.UTC)

	resp := legalVersionsAt(now, func(at time.Time) (legal.Versions, time.Time, bool) {
		askedAt = at
		return previous, until, true
	})
	if !askedAt.Equal(now) {
		t.Errorf("window read at %v, want the request instant %v", askedAt, now)
	}

	wire, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(wire, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, want := string(body["grace_until"]), `"2026-11-01T00:00:00Z"`; got != want {
		t.Errorf("grace_until = %s, want %s (RFC 3339 in UTC)", got, want)
	}
	if got, want := string(body["previous"]), `{"terms_version":"2025-12-01","privacy_version":"2025-12-02"}`; got != want {
		t.Errorf("previous = %s, want %s", got, want)
	}

	closed := legalVersionsAt(now, func(time.Time) (legal.Versions, time.Time, bool) {
		return legal.Versions{}, time.Time{}, false
	})
	if closed.Body.GraceUntil != nil || closed.Body.Previous != nil {
		t.Errorf("closed window published grace fields: %+v", closed.Body)
	}
}
