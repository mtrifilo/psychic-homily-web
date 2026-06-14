import * as Sentry from '@sentry/nextjs'

const isProduction = process.env.NODE_ENV === 'production'

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,

  // Environment for filtering in Sentry dashboard
  environment: process.env.NODE_ENV,

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
// is 0 in dev, so there's nothing to capture and no reason to fetch it).
// lazyLoadIntegration fetches the replay bundle from the Sentry CDN at runtime,
// so it never enters our eager client bundle. Tradeoff: replay attaches a beat
// late, so an error in the very first moments of a session may lack a replay.
if (isProduction && typeof window !== 'undefined') {
  const attachReplay = () => {
    Sentry.lazyLoadIntegration('replayIntegration')
      .then(replayIntegration => {
        Sentry.addIntegration(
          replayIntegration({
            // Mask all text for privacy
            maskAllText: true,
            // Block all media for privacy
            blockAllMedia: true,
          })
        )
      })
      .catch(() => {
        // Replay is best-effort; a CDN/load failure must not break the app.
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
