import posthog from 'posthog-js'

let isInitialized = false

export function initPostHog(): void {
  if (typeof window === 'undefined' || isInitialized) return

  const key = process.env.NEXT_PUBLIC_POSTHOG_KEY
  if (!key) return

  posthog.init(key, {
    api_host: process.env.NEXT_PUBLIC_POSTHOG_HOST || 'https://app.posthog.com',
    capture_pageview: false, // Manual for SPA
    capture_pageleave: true,
    opt_out_capturing_by_default: true, // GDPR: no tracking until consent
    persistence: 'localStorage',
    session_recording: { maskAllInputs: true },
  })

  isInitialized = true
}

export function optInPostHog(): void {
  if (!isInitialized) initPostHog()
  posthog.opt_in_capturing()
  posthog.startSessionRecording()
}

export function optOutPostHog(): void {
  posthog.opt_out_capturing()
  posthog.stopSessionRecording()
  posthog.reset()
}

export { posthog }
