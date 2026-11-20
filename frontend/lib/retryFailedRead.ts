import { isAuthError } from '@/lib/errors'

/**
 * For a surface that stays open with nothing else to show until a read
 * succeeds: how often, and when, a failed read is tried again on its own.
 */
export const RETRY_FAILED_READ_MS = 10_000

/**
 * Whether a query's latest read failed in a way waiting can fix: any error
 * but a refused session, which will not change by waiting.
 */
export function retryableFailure(state: {
  status: string
  error: unknown
}): boolean {
  return state.status === 'error' && !isAuthError(state.error)
}

/** A `refetchInterval` that retries a read only while it is retryably failed. */
export function retryWhileFailed(query: {
  state: { status: string; error: unknown }
}): number | false {
  return retryableFailure(query.state) ? RETRY_FAILED_READ_MS : false
}
