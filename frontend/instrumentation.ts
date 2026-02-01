import * as Sentry from '@sentry/nextjs'

export async function register() {
  if (process.env.NEXT_RUNTIME === 'nodejs') {
    // Server-side (Node.js) initialization
    await import('./sentry.server.config')
  }

  if (process.env.NEXT_RUNTIME === 'edge') {
    // Edge runtime initialization
    await import('./sentry.edge.config')
  }
}

// Capture server-side request errors (API routes, server components, etc.)
export const onRequestError = Sentry.captureRequestError
