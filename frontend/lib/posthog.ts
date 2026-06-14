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
let optedIn = false

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
  if (optedIn) return
  const ph = await loadPostHog()
  if (!ph) return
  ph.opt_in_capturing()
  ph.startSessionRecording()
  optedIn = true
}

// Consent withdrawn → opt out / stop / reset. No-op if posthog never loaded
// (the common case: a visitor who never consented).
export function disableAnalytics(): void {
  if (!instance || !optedIn) return
  instance.opt_out_capturing()
  instance.stopSessionRecording()
  instance.reset()
  optedIn = false
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
