/**
 * Read by name alone, not `instanceof Error` or `instanceof DOMException`:
 * platform APIs reject with a `DOMException`, and not every engine makes
 * `DOMException` extend `Error`. Any thrown object carrying the name counts,
 * whatever its prototype chain.
 */
function hasErrorName(error: unknown, name: string): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    (error as { name?: unknown }).name === name
  )
}

/** Whether a thrown value is named AbortError. */
export function isAbortError(error: unknown): boolean {
  return hasErrorName(error, 'AbortError')
}

/** Whether a thrown value is named TimeoutError. */
export function isTimeoutError(error: unknown): boolean {
  return hasErrorName(error, 'TimeoutError')
}
