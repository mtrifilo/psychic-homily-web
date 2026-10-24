/**
 * Wraps a test's fetch stub so the read /auth's Create account tab makes for
 * the under-age refusal marker answers "no marker" without reaching it. The
 * stub then sees only the requests the test arranges answers for.
 */
export function answeringNoAgeRefusalMarker(
  fetchStub: (input: RequestInfo | URL, init?: RequestInit) => unknown
): (input: RequestInfo | URL, init?: RequestInit) => unknown {
  return (input, init) => {
    if (String(input).includes('/auth/oauth/pending-signup')) {
      return Promise.resolve(
        new Response(JSON.stringify({ status: 'expired' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      )
    }
    return fetchStub(input, init)
  }
}
