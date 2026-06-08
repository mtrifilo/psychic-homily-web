// Version identifiers persisted at account creation for legal acceptance evidence.
export const CURRENT_TERMS_VERSION = '2026-01-31'
export const CURRENT_PRIVACY_VERSION = '2026-02-15'

// Minimum age (years) a user must confirm at signup. Locked at 16 (PSY-1023) to
// match the /terms and /privacy minimum-age clauses. The backend re-checks this
// value, so the frontend confirmation is a UX gate, not the authoritative one.
export const MIN_SIGNUP_AGE = 16
