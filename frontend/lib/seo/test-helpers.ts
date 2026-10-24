/**
 * Shared fetch-Response stubs for page-level `generateMetadata` tests.
 *
 * The slug pages (shows/venues/artists/collections) read their entity via
 * `fetch` and branch on `res.ok` / `res.status`. These return the minimal
 * shape those code paths touch — `ok`, `status`, and a `json()` resolver —
 * so a single `fetchMock.mockResolvedValueOnce(...)` drives the metadata
 * branch under test.
 */

export function okResponse(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as unknown as Response
}

export function errorResponse(status: number): Response {
  return { ok: false, status, json: async () => ({}) } as unknown as Response
}

/**
 * A full `Response` carrying `body` as JSON, for reads that go through
 * `fetchListPayload`: it weighs the body against the Data Cache budget and so
 * reads it with `text()`, which `okResponse` does not offer. A body can be read
 * once, so build a fresh one per request.
 */
export function jsonBodyResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}
