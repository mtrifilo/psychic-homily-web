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

interface IdentityProps {
  email: string
  is_admin?: boolean
}

// Identity the app asked for while posthog was still loading. The signed-in
// viewer is known at first client render (the profile is hydrated from the
// server), which is reliably EARLIER than the consent read + dynamic import
// that produce `instance` — so without this the identify call would land on a
// null instance and never be retried. Replayed by enableAnalytics once the
// instance exists, same recovery shape as `landingCaptured`.
let pendingIdentity: { id: string; props: IdentityProps } | null = null

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
  // Replay the identify call that arrived before the instance existed.
  if (pendingIdentity) {
    ph.identify(pendingIdentity.id, pendingIdentity.props)
    pendingIdentity = null
  }
}

// Consent withdrawn → opt out / stop / reset. Also clears the intent flag so an
// in-flight enableAnalytics() aborts its opt-in when it resolves. No-op if
// posthog never loaded or is already opted out.
export function disableAnalytics(): void {
  desired = false
  // Drop any queued identify: consent is gone, so it must never be replayed by
  // a later enableAnalytics(). Re-granting consent re-fires the identify effect
  // and re-queues it.
  pendingIdentity = null
  if (!instance || !instance.has_opted_in_capturing()) return
  instance.opt_out_capturing()
  instance.stopSessionRecording()
  instance.reset()
}

// Capture a pageview — only if posthog is already loaded (i.e. consented).
export function capturePageview(url: string): void {
  instance?.capture('$pageview', { $current_url: url })
}

// Identify an authenticated user. Queued when posthog hasn't loaded yet, so
// the identity survives the lazy import (see `pendingIdentity`).
export function identifyUser(id: string, props: IdentityProps): void {
  if (!instance) {
    // The queue is a browser-only buffer. Module state is shared across
    // requests in the server runtime, so queueing there would park one
    // viewer's email where the next request could read it — and it could
    // never be flushed anyway, since posthog only ever loads in the browser.
    if (typeof window === 'undefined') return
    pendingIdentity = { id, props }
    return
  }
  instance.identify(id, props)
}

// Reset identity (logout). Also drops any queued identify so a sign-out that
// races the lazy import can't be undone by a replay.
export function resetAnalytics(): void {
  pendingIdentity = null
  instance?.reset()
}
