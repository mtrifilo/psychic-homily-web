/**
 * Shared form error utilities for TanStack Form + Zod validation.
 *
 * TanStack Form can produce error arrays with mixed types (strings, objects
 * with a `message` property, etc.) and may duplicate messages when both
 * `onChange` and `onSubmit` validators fire. These helpers normalise and
 * deduplicate those errors for display.
 */

/**
 * Safely extract a display string from a single TanStack Form validation error.
 */
export function getErrorMessage(err: unknown): string {
  if (typeof err === 'string') {
    return err
  }
  if (err && typeof err === 'object' && 'message' in err) {
    return String((err as { message: unknown }).message)
  }
  return String(err)
}

/**
 * Deduplicate an array of validation errors and join them into a single
 * comma-separated string suitable for rendering in a `<p role="alert">`.
 */
export function getUniqueErrors(errors: unknown[]): string {
  return [...new Set(errors.map(getErrorMessage))].join(', ')
}
