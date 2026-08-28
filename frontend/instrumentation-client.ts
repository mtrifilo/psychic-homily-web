import * as Sentry from '@sentry/nextjs'
import { stripUrls, toTelemetryPath } from './lib/rate-limit-telemetry'

const isProduction = process.env.NODE_ENV === 'production'

/**
 * Strip query strings from the SDK's automatic HTTP breadcrumbs.
 *
 * `breadcrumbsIntegration` is on by default and records the FULL url of every
 * fetch and XHR in the tab, which then rides along on every event we send. On
 * this app that trail routinely contains feed tokens (`/feeds/phcal_...`),
 * verification and magic-link tokens, and user-typed search and filter terms,
 * none of which we have any reason to ship to a third party.
 *
 * `toTelemetryPath` is reused rather than reimplemented so there is ONE
 * definition of "safe to send" (PSY-1912). It drops the query and fragment
 * outright and reduces ids and token-shaped segments to placeholders.
 *
 * The cost is real and accepted: a breadcrumb no longer shows which page of a
 * paginated call was requested. The endpoint family, method, and status code
 * all survive, which is what an incident actually gets debugged from.
 *
 * MESSAGES get the same rule (PSY-1568) — see the note in the body. That half
 * only replaces URLs; it deliberately does not shorten anything, so scrubbing
 * cannot quietly truncate unrelated console output.
 */
function scrubBreadcrumbUrls(
  breadcrumb: Sentry.Breadcrumb
): Sentry.Breadcrumb {
  // Console breadcrumbs carry their URL inside the MESSAGE, not in `data.url`
  // — `consoleIntegration` records `String(arg)` for each argument. So a
  // third-party error logged to the console (MapLibre's AJAXError embeds the
  // full tile request URL in its message) shipped its URL verbatim while the
  // careful shaping below sat next to it doing nothing. Same rule, applied to
  // the other place a URL can hide.
  const scrubbed: Sentry.Breadcrumb =
    typeof breadcrumb.message === 'string'
      ? { ...breadcrumb, message: stripUrls(breadcrumb.message) }
      : breadcrumb

  // Both scrubs apply, never one or the other: a breadcrumb is free to carry a
  // message AND a url, and an early return on the first would silently leave
  // the second unscrubbed.
  const url = scrubbed.data?.url
  if (typeof url !== 'string') return scrubbed
  return {
    ...scrubbed,
    data: { ...scrubbed.data, url: toTelemetryPath(url) },
  }
}

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,

  // Environment for filtering in Sentry dashboard
  environment: process.env.NODE_ENV,

  beforeBreadcrumb: scrubBreadcrumbUrls,

  // Adjust tracing sample rate for production
  // Disable tracing in development to reduce noise
  tracesSampleRate: isProduction ? 0.1 : 0,

  // Debug mode off - too verbose
  debug: false,

  // Session Replay sampling (read by the lazily-attached replay integration
  // below). Production only; 0 in dev.
  replaysSessionSampleRate: isProduction ? 0.1 : 0,
  replaysOnErrorSampleRate: isProduction ? 1.0 : 0,

  // replayIntegration is intentionally NOT eager here (PSY-1091): statically
  // including it pulls @sentry-internal/replay (~45KB) + its init into every
  // route's eager bundle — it was the top non-framework scripting cost on
  // /explore's TTI. It is attached lazily after interactivity below.
})

// Attach Session Replay after the page is interactive, production only (sampling
// is 0 in dev, so there's nothing to capture). The dynamic import lands replay
// in a self-hosted lazy chunk (off the eager critical path) — see
// ./instrumentation-replay for why this is a local import, not the Sentry CDN
// lazyLoadIntegration path. Tradeoff: replay attaches a beat late, so an error
// in the very first moments of a session may lack a replay.
if (isProduction && typeof window !== 'undefined') {
  const attachReplay = () => {
    void import('./instrumentation-replay')
      .then(m => m.attachReplay())
      .catch(() => {
        // Replay is best-effort; a load failure must not break the app.
      })
  }

  if ('requestIdleCallback' in window) {
    window.requestIdleCallback(attachReplay, { timeout: 5000 })
  } else {
    setTimeout(attachReplay, 2000)
  }
}

// Export for router instrumentation (Next.js 15+)
export const onRouterTransitionStart = Sentry.captureRouterTransitionStart
