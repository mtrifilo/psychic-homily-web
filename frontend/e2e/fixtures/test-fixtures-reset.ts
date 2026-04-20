import { type APIRequestContext, request as playwrightRequest } from '@playwright/test'
import * as path from 'path'

/**
 * PSY-432 — E2E helper for the admin-only `/admin/test-fixtures/reset`
 * endpoint. See backend/internal/api/handlers/test_fixtures.go.
 *
 * The endpoint is only registered when the backend boots with
 * `ENABLE_TEST_FIXTURES=1` + `ENVIRONMENT` in {test,ci,development}. Global
 * setup (global-setup.ts) sets both before spawning the backend.
 */

const AUTH_DIR = path.resolve(__dirname, '../.auth')

// PSY-432: we talk to the backend directly on :8080 rather than through the
// frontend Next.js proxy (/api → :8080). The proxy strips non-auth headers
// (app/api/[...path]/route.ts:17-30), so a custom `X-Test-Fixtures` header
// would be dropped before reaching the backend. The cookie we stored for
// `domain=localhost` still works against :8080.
const BACKEND_BASE_URL = 'http://localhost:8080'

/**
 * DEFAULT_RESET_SCOPES is the canonical set of tables a worker teardown
 * should clear for its seeded user. Kept broad on purpose — tests want a
 * clean slate, and `user_bookmarks` covers five mutating flows at once
 * (saves, favorite venues, follows, going, interested).
 */
export const DEFAULT_RESET_SCOPES = [
  'user_bookmarks',
  'collection_items',
  'collection_subscribers',
  'collections',
  'pending_shows',
] as const

export type ResetScope = (typeof DEFAULT_RESET_SCOPES)[number]

export type ResetResponse = {
  deleted: Record<string, number>
}

/**
 * resetTestFixtures calls POST /admin/test-fixtures/reset using the admin
 * storage state. Use from a worker-scoped fixture teardown — see
 * fixtures/auth.ts.
 *
 * Why APIRequestContext instead of `page.request`: worker teardown runs
 * after the browser/page is closed, so we need a standalone request
 * context bound to the admin auth cookie jar.
 */
export async function resetTestFixtures(
  userId: number,
  scopes: readonly string[] = DEFAULT_RESET_SCOPES,
): Promise<ResetResponse> {
  const ctx: APIRequestContext = await playwrightRequest.newContext({
    baseURL: BACKEND_BASE_URL,
    storageState: path.join(AUTH_DIR, 'admin.json'),
  })
  try {
    const resp = await ctx.post('/admin/test-fixtures/reset', {
      headers: { 'X-Test-Fixtures': '1' },
      data: { user_id: userId, tables: scopes },
    })
    if (!resp.ok()) {
      const bodyText = await resp.text().catch(() => '<unreadable body>')
      throw new Error(
        `test-fixtures reset failed: HTTP ${resp.status()} ${bodyText}`,
      )
    }
    return (await resp.json()) as ResetResponse
  } finally {
    await ctx.dispose()
  }
}

/**
 * lookupWorkerUserId fetches the numeric user ID for the given auth storage
 * state file by calling GET /auth/profile. Cached per call; the caller is
 * expected to cache across tests (see fixtures/auth.ts).
 */
export async function lookupWorkerUserId(authFile: string): Promise<number> {
  const ctx: APIRequestContext = await playwrightRequest.newContext({
    baseURL: BACKEND_BASE_URL,
    storageState: path.join(AUTH_DIR, authFile),
  })
  try {
    const resp = await ctx.get('/auth/profile')
    if (!resp.ok()) {
      throw new Error(
        `profile lookup failed: HTTP ${resp.status()} for ${authFile}`,
      )
    }
    const profile = (await resp.json()) as { id?: number; user?: { id?: number } }
    // The backend may return `{id}` or `{user: {id}}` depending on version;
    // accept both to be robust to wrapper changes.
    const id = profile.id ?? profile.user?.id
    if (!id || typeof id !== 'number') {
      throw new Error(
        `profile lookup returned no id field: ${JSON.stringify(profile).slice(0, 200)}`,
      )
    }
    return id
  } finally {
    await ctx.dispose()
  }
}
