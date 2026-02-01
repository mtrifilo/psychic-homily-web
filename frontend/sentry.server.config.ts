import * as Sentry from '@sentry/nextjs'

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,

  // Environment for filtering in Sentry dashboard
  environment: process.env.NODE_ENV,

  // Adjust tracing sample rate for production
  // Disable tracing in development to reduce noise
  tracesSampleRate: process.env.NODE_ENV === 'production' ? 0.1 : 0,

  // Debug mode off - too verbose
  debug: false,
})
