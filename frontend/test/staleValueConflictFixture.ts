import { STALE_VALUE_CODE } from '@/features/contributions/staleValueConflict'
import type { ApiError } from '@/lib/api'

/**
 * The rejection `apiRequest` produces from a stale-value 409: `details` is the
 * response's `errors` array, and each entry's `value` carries the code and the
 * entity's current value per named field.
 *
 * One builder for every suite that exercises the re-seed, so the wire shape is
 * spelled once. Two copies would both stay green after a rename on the server
 * while asserting a shape it no longer sends, which is the drift these suites
 * exist to catch.
 */
export function stubStaleValueConflict(
  currentValues: Record<string, unknown>,
  message = 'This field has changed since you loaded the form.'
): ApiError {
  const error: ApiError = new Error(message)
  error.status = 409
  error.details = [
    {
      message,
      location: 'body.changes',
      value: { code: STALE_VALUE_CODE, current_values: currentValues },
    },
  ]
  return error
}
