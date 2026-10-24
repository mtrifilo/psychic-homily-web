// The Terms of Service and Privacy Policy versions this build was released
// with, pinned to backend/internal/legal/legal.go by TestVersionsMatchFrontend,
// so a bump changes both. The sign-up forms do not send these: they send the
// versions GET /legal/versions publishes at submit time.
export const CURRENT_TERMS_VERSION = '2026-01-31'
export const CURRENT_PRIVACY_VERSION = '2026-10-24'

// The pair the sign-up forms sent before the most recent bump, which the backend
// also accepts until its grace window closes (GET /legal/versions publishes the
// window). Empty when no bump has opened a window; pinned to the backend's
// PreviousTermsVersion and PreviousPrivacyVersion by the same test.
export const PREVIOUS_TERMS_VERSION = '2026-01-31'
export const PREVIOUS_PRIVACY_VERSION = '2026-02-15'

// The minimum age, in years, an account may be created at, and the oldest age
// a date of birth may give before it counts as a typing slip. Pinned to
// backend/internal/legal (MinSignupAge, MaxPlausibleAge) by
// TestVersionsMatchFrontend. The backend computes the age and is the
// authoritative gate; these only shape what the forms say and send.
export const MIN_SIGNUP_AGE = 16
export const MAX_PLAUSIBLE_AGE = 150
