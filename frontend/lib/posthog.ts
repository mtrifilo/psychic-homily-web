import type { PostHog } from 'posthog-js'

// PostHog is loaded LAZILY and only after analytics consent (PSY-1091). The
// 55KB posthog-js bundle + its init were sitting in every route's eager client
// chunk even though capturing is gated on consent — a top /explore TTI cost,
// and pure dead weight for the no-consent path (every first-time visitor, and
// the Lighthouse gate, which never consents). Now `posthog-js` is dynamic-
// import()ed only when `enableAnalytics()` runs, so it never enters the eager
// bundle; the no-consent path pays nothing.

let instance: PostHog | null = null
let loadPromise: Promise<PostHog | null> | null = null
// Current consent intent. Tracked separately from posthog's own opt-in state so
// a consent withdrawal DURING the async load aborts the pending opt-in (privacy
// race — PSY-1091 adversarial review).
let desired = false
// Landing pageview is captured once per page load by enableAnalytics, because
// the PostHogPageView effect fires before posthog has lazy-loaded (no-op then).
let landingCaptured = false

// Idempotent: dynamic-import + init posthog-js once. Returns null on the server
// or when no key is configured.
function loadPostHog(): Promise<PostHog | null> {
  if (loadPromise) return loadPromise
  const key = process.env.NEXT_PUBLIC_POSTHOG_KEY
  if (typeof window === 'undefined' || !key) return Promise.resolve(null)

  loadPromise = import('posthog-js').then(({ default: posthog }) => {
    posthog.init(key, {
      api_host:
        process.env.NEXT_PUBLIC_POSTHOG_HOST || 'https://app.posthog.com',
      capture_pageview: false, // Manual for SPA
      capture_pageleave: true,
      opt_out_capturing_by_default: true, // GDPR: no tracking until consent
      persistence: 'localStorage',
      session_recording: { maskAllInputs: true },
    })
    instance = posthog
    return posthog
  })
  return loadPromise
}

// Consent granted → load posthog-js (if needed), opt in, start recording.
// Idempotent: safe to call on every consent-sync render.
export async function enableAnalytics(): Promise<void> {
  desired = true
  const ph = await loadPostHog()
  // No key/SSR, or consent was withdrawn while the import was in flight — do
  // NOT opt in (privacy: never capture against the current consent intent).
  if (!ph || !desired) return
  // Only opt in / start recording when not already opted in, so a returning
  // consented visitor doesn't re-fire opt-in + a synthetic $opt_in event on
  // every page load (PSY-728 parity — `optedIn` would reset per page load).
  if (!ph.has_opted_in_capturing()) {
    ph.opt_in_capturing()
    ph.startSessionRecording()
  }
  // Recover the landing pageview the PostHogPageView effect missed because
  // posthog hadn't lazy-loaded yet on first paint. Once per page load.
  if (!landingCaptured) {
    ph.capture('$pageview', { $current_url: window.location.href })
    landingCaptured = true
  }
}

// Consent withdrawn → opt out / stop / reset. Also clears the intent flag so an
// in-flight enableAnalytics() aborts its opt-in when it resolves. No-op if
// posthog never loaded or is already opted out.
export function disableAnalytics(): void {
  desired = false
  if (!instance || !instance.has_opted_in_capturing()) return
  instance.opt_out_capturing()
  instance.stopSessionRecording()
  instance.reset()
}

// Capture a pageview — only if posthog is already loaded (i.e. consented).
export function capturePageview(url: string): void {
  instance?.capture('$pageview', { $current_url: url })
}

// Identify an authenticated user — only if posthog is already loaded.
export function identifyUser(
  id: string,
  props: { email: string; is_admin?: boolean }
): void {
  instance?.identify(id, props)
}

// Reset identity (logout) — only if posthog is already loaded.
export function resetAnalytics(): void {
  instance?.reset()
}
