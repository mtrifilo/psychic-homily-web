package system

import (
	"context"
	"time"

	"psychic-homily-backend/internal/legal"
)

// LegalVersionsRoute is the path LegalVersionsHandler is registered on.
const LegalVersionsRoute = "/legal/versions"

// legalVersionsCacheControl bounds how long a client or a shared cache may
// answer with versions this deploy no longer publishes. A version bump's grace
// window outlasts it, so a response cached just before the bump names a pair
// the backend still accepts.
const legalVersionsCacheControl = "public, max-age=300"

// LegalVersionsPrevious is the pair the sign-up forms sent before the most
// recent version bump.
type LegalVersionsPrevious struct {
	TermsVersion   string `json:"terms_version" example:"2026-01-31" doc:"Terms of Service version accepted until grace_until"`
	PrivacyVersion string `json:"privacy_version" example:"2026-02-15" doc:"Privacy Policy version accepted until grace_until"`
}

// LegalVersionsResponse publishes what an account creation must send to be
// accepted. It is the same for every caller.
type LegalVersionsResponse struct {
	CacheControl string `header:"Cache-Control"`

	Body struct {
		TermsVersion   string     `json:"terms_version" example:"2026-01-31" doc:"Current Terms of Service version; a sign-up sends it as terms_version"`
		PrivacyVersion string     `json:"privacy_version" example:"2026-10-24" doc:"Current Privacy Policy version; a sign-up sends it as privacy_version"`
		MinSignupAge   int        `json:"min_signup_age" example:"16" doc:"Minimum age a sign-up must attest as min_age_attested"`
		GraceUntil     *time.Time `json:"grace_until" nullable:"true" doc:"While a grace window is open, the instant (UTC) the previous versions stop being accepted; null otherwise"`
		// omitempty rather than nullable: huma refuses nullable on a struct
		// field at registration, so the published contract for "no window" is
		// an absent field.
		Previous *LegalVersionsPrevious `json:"previous,omitempty" doc:"While a grace window is open, the versions also accepted until grace_until (either alone or both together); absent otherwise"`
	}
}

// LegalVersionsHandler answers GET /legal/versions: the Terms of Service and
// Privacy Policy versions a sign-up must send, the minimum age it must attest,
// and, while a version bump's grace window is open, the previous versions the
// backend also accepts and when they stop being accepted.
func LegalVersionsHandler(_ context.Context, _ *struct{}) (*LegalVersionsResponse, error) {
	return legalVersionsFor(legal.GraceWindow(time.Now())), nil
}

// legalVersionsFor builds the response from legal.GraceWindow's results.
func legalVersionsFor(previous legal.Versions, until time.Time, open bool) *LegalVersionsResponse {
	resp := &LegalVersionsResponse{CacheControl: legalVersionsCacheControl}
	resp.Body.TermsVersion = legal.CurrentTermsVersion
	resp.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	resp.Body.MinSignupAge = legal.MinSignupAge
	if open {
		until = until.UTC()
		resp.Body.GraceUntil = &until
		resp.Body.Previous = &LegalVersionsPrevious{TermsVersion: previous.Terms, PrivacyVersion: previous.Privacy}
	}
	return resp
}
