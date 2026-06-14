import { addIntegration, replayIntegration } from '@sentry/nextjs'

// Self-hosted, lazily-attached Sentry Session Replay (PSY-1091). This module is
// dynamic-import()ed from instrumentation-client.ts after interactivity, so the
// statically-imported replayIntegration (and @sentry-internal/replay, ~45KB)
// lands in a lazy chunk instead of the eager client bundle — it was the top
// non-framework scripting cost on /explore's TTI.
//
// NOTE: not the Sentry CDN `lazyLoadIntegration` path — the app's CSP
// (next.config.ts `script-src`) does not allow browser.sentry-cdn.com, and a
// CDN script is also adblock-prone. A self-hosted dynamic import keeps replay
// under `'self'` with no CSP change and no runtime CDN dependency.
//
// Sample rates (replaysSessionSampleRate / replaysOnErrorSampleRate) come from
// the Sentry.init options in instrumentation-client.ts: replay reads them from
// client.getOptions() at addIntegration time (verified @sentry/nextjs 10.38 via
// loadReplayOptionsFromClient). Re-verify on Sentry major bumps.
export function attachReplay(): void {
  addIntegration(
    replayIntegration({
      // Mask all text for privacy
      maskAllText: true,
      // Block all media for privacy
      blockAllMedia: true,
    })
  )
}
