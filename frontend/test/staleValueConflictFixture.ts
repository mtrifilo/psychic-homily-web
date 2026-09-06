import { STALE_VALUE_CODE } from '@/features/contributions/staleValueConflict'
import type { ApiError } from '@/lib/api'

/**
 * The rejection `apiRequest` produces from a stale-value 409: `details` is the
 * response's `errors` array, and each entry's `value` carries the code and the
 * entity's current value per named field.
 *
 * One builder for every suite that exercises the re-seed. The shape is
 * transcribed from the Go mapper by hand, with no generator behind it, so this
 * catches no server-side rename; what it buys is that a rename is one edit here
 * rather than one per suite.
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
      value: { code: STALE_VALUE_CODE, current_values: currentValues },
    },
  ]
  return error
}
