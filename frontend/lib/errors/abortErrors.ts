/**
 * Read by name alone, not `instanceof Error` or `instanceof DOMException`:
 * fetch, body reads and `navigator.share` reject with a `DOMException`, and
 * `DOMException extends Error` is a recent WebIDL guarantee that older engines
 * do not honour. Any thrown object carrying the name counts,
 * whatever its prototype chain.
 */
function hasErrorName(error: unknown, name: string): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    (error as { name?: unknown }).name === name
  )
}

/**
 * Whether a thrown value is the rejection an aborted operation gives: fetch
 * or a body read once its signal aborts with no reason, or a dismissed share
 * sheet.
 */
export function isAbortError(error: unknown): boolean {
  return hasErrorName(error, 'AbortError')
}

/**
 * Whether a thrown value is the rejection a timed-out operation gives, such as
 * fetch under an `AbortSignal.timeout` that fired.
 */
export function isTimeoutError(error: unknown): boolean {
  return hasErrorName(error, 'TimeoutError')
}
