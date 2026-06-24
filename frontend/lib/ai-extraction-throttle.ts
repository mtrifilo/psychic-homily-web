// Shared per-user rate-limit gate for the AI extraction BFF routes
// (extract-show + extract-collection, PSY-855). Both routes call enforceThrottle
// BEFORE doing any (paid) Anthropic work; on a limit hit it returns the 429
// envelope the routes must send back WITHOUT calling Anthropic.
//
// The counter is Postgres-backed in the Go backend — not in-memory — so the
// limit holds across Vercel serverless instances (an in-memory counter would
// let a user multiply the limit by the number of warm instances). This helper
// just forwards the user's auth_token cookie to the backend throttle endpoint;
// the backend identifies the user from the JWT (no spoofable user_id), counts
// the attempt, and bypasses admins (PSY-345).

import { NextResponse } from 'next/server'
import * as Sentry from '@sentry/nextjs'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

interface ThrottleDecision {
  allowed: boolean
  retry_after_seconds: number
  limit: number
  window_seconds: number
}

// Shape the user-facing 429 body. Mirrors PSY-589's backend Huma 429 pattern:
// a human-readable `error` string plus a machine-readable `retry_after` (seconds).
interface ThrottleErrorBody {
  success: false
  error: string
  retry_after: number
}

/**
 * Build the "try again in N minutes" copy from the seconds-remaining. Rounds UP
 * to the next whole minute so the hint never undershoots the real cooldown; for
 * sub-minute waits it reads in seconds so a 30s wait doesn't round to a
 * misleading "1 minute".
 */
function formatRetryHint(retryAfterSeconds: number): string {
  const seconds = Math.max(1, Math.ceil(retryAfterSeconds))
  if (seconds < 60) {
    const unit = seconds === 1 ? 'second' : 'seconds'
    return `Rate limit exceeded. Try again in ${seconds} ${unit}.`
  }
  const minutes = Math.ceil(seconds / 60)
  const unit = minutes === 1 ? 'minute' : 'minutes'
  return `Rate limit exceeded. Try again in ${minutes} ${unit}.`
}

/**
 * Enforce the per-user AI-extraction rate limit. Returns `{ ok: true }` when the
 * attempt is allowed (the backend has already counted it), or `{ ok: false,
 * response }` carrying the exact response the route must return instead of
 * calling Anthropic:
 *   - 429 (limit hit): Retry-After header + the decided JSON body.
 *   - 503 (gate unavailable): fail CLOSED — if the counter is down we must not
 *     hand out a free pass that would drain the Anthropic budget.
 *
 * `authToken` is the caller's auth_token cookie value (already extracted by the
 * route's auth gate). The backend throttle endpoint is on the Protected group,
 * so the cookie identifies the user.
 */
export async function enforceThrottle(
  authToken: string,
  sentryService: string
): Promise<{ ok: true } | { ok: false; response: NextResponse }> {
  let res: Response
  try {
    res = await fetch(`${BACKEND_URL}/ai-extraction/throttle`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Cookie: `auth_token=${authToken}`,
      },
    })
  } catch (error) {
    // Backend unreachable — fail closed. A network blip on the gate must not
    // become an open door to the paid Anthropic API.
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: sentryService, error_type: 'throttle_unreachable' },
    })
    return {
      ok: false,
      response: NextResponse.json(
        { success: false, error: 'Rate limit check temporarily unavailable. Please try again.' },
        { status: 503 }
      ),
    }
  }

  if (!res.ok) {
    // Backend returned a non-2xx (e.g. 503 from the handler when the counter
    // write failed, or 401 if the cookie was rejected). Fail closed: don't call
    // Anthropic. Pass the status through so a 401 stays a 401.
    Sentry.captureMessage(`AI-extraction throttle gate returned ${res.status}`, {
      level: res.status >= 500 ? 'error' : 'warning',
      tags: { service: sentryService, error_type: 'throttle_gate_error' },
    })
    const status = res.status === 401 ? 401 : 503
    const message =
      status === 401
        ? 'Authentication required'
        : 'Rate limit check temporarily unavailable. Please try again.'
    return {
      ok: false,
      response: NextResponse.json({ success: false, error: message }, { status }),
    }
  }

  const decision = (await res.json().catch(() => null)) as ThrottleDecision | null
  if (!decision || typeof decision.allowed !== 'boolean') {
    // Malformed decision — fail closed.
    return {
      ok: false,
      response: NextResponse.json(
        { success: false, error: 'Rate limit check temporarily unavailable. Please try again.' },
        { status: 503 }
      ),
    }
  }

  if (decision.allowed) {
    return { ok: true }
  }

  const retryAfter = Math.max(1, Math.ceil(decision.retry_after_seconds || 1))
  const body: ThrottleErrorBody = {
    success: false,
    error: formatRetryHint(retryAfter),
    retry_after: retryAfter,
  }
  return {
    ok: false,
    response: NextResponse.json(body, {
      status: 429,
      headers: { 'Retry-After': String(retryAfter) },
    }),
  }
}
