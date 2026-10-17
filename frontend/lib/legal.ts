// Version identifiers persisted at account creation for legal acceptance evidence.
// The backend accepts these exact values (backend/internal/legal/legal.go,
// pinned to this file by TestVersionsMatchFrontend), so a bump changes both.
export const CURRENT_TERMS_VERSION = '2026-01-31'
export const CURRENT_PRIVACY_VERSION = '2026-02-15'

// The pair the sign-up forms sent before the most recent bump, which the backend
// also accepts until its grace window closes (GET /legal/versions publishes the
// window). Empty while no window is open; pinned to the backend's
// PreviousTermsVersion and PreviousPrivacyVersion by the same test.
export const PREVIOUS_TERMS_VERSION = ''
export const PREVIOUS_PRIVACY_VERSION = ''

// Minimum age (years) a user must confirm at signup. Locked at 16 (PSY-1023) to
// match the /terms and /privacy minimum-age clauses. The backend re-checks this
// value, so the frontend confirmation is a UX gate, not the authoritative one.
export const MIN_SIGNUP_AGE = 16
