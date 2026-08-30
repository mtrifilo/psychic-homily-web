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
/**
 * Event classes that must never flush a buffered replay.
 *
 * Replay treats ANY event without a `type` as an error event, `captureMessage`
 * included, so a `warning` is enough to convert a buffer-mode session into an
 * uploaded and then continuously-recorded one. That is the intended behaviour
 * for our own failures; it is not for an event whose trigger is a string an
 * untrusted contributor stored. Without this, whoever plants an affiliate tag
 * chooses which visitors get session-recorded, and spends replay quota (the
 * expensive kind) doing it.
 *
 * Keyed on `error_type`, which is the tag this app already uses to name an
 * event class.
 */
const NON_FLUSHING_ERROR_TYPES = new Set(['planted_affiliate_tag'])

export function attachReplay(): void {
  addIntegration(
    replayIntegration({
      // Mask all text for privacy
      maskAllText: true,
      // Block all media for privacy
      blockAllMedia: true,
      // Runs only for buffer-mode flush decisions; returning false leaves the
      // session buffering rather than uploading. Real errors are unaffected.
      beforeErrorSampling: event =>
        !NON_FLUSHING_ERROR_TYPES.has(String(event.tags?.error_type ?? '')),
    })
  )
}
