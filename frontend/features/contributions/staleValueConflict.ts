import { isConflictError } from '@/lib/api'
import type { ApiError } from '@/lib/api'

/**
 * The error code the suggest-edit and approve paths answer with when a recorded
 * previous value no longer describes the entity. It rides inside
 * `errors[].value` rather than at the top level because huma's error model has
 * no field for a code.
 */
export const STALE_VALUE_CODE = 'PENDING_EDIT_STALE_VALUE'

/**
 * The entity's current value for `field` on a stale-value 409, or undefined for
 * any other rejection.
 *
 * A 409 is also how a duplicate queued edit is reported, and that one carries no
 * values, so the code has to be checked rather than the status alone.
 *
 * The value returned is the server's derivation of what THIS reader observes,
 * which is the same value a successful submission would have stored. That is
 * what makes it the right thing to re-seed a form with.
 */
export function staleFieldCurrentValue(
  error: unknown,
  field: string
): string | undefined {
  if (!isConflictError(error)) return undefined

  const details = (error as ApiError).details
  if (!Array.isArray(details)) return undefined

  for (const detail of details) {
    const value = (detail as { value?: unknown } | null)?.value
    if (!isRecord(value) || value.code !== STALE_VALUE_CODE) continue
    const currentValues = value.current_values
    if (!isRecord(currentValues)) continue
    const current = currentValues[field]
    if (typeof current === 'string') return current
  }
  return undefined
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
