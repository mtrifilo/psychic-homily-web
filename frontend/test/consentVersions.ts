import type { QueryClient } from '@tanstack/react-query'
import { http } from 'msw'
import type { ConsentVersions } from '@/features/auth/hooks/useLegalVersions'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION } from '@/lib/legal'
import { queryKeys } from '@/lib/queryClient'
import { server } from './mocks/server'
import { TEST_API_BASE } from './mocks/handlers'
import { createTestQueryClient } from './utils'

export const BUILD_CONSENT_VERSIONS: ConsentVersions = {
  termsVersion: CURRENT_TERMS_VERSION,
  privacyVersion: CURRENT_PRIVACY_VERSION,
}

/** GET /legal/versions naming this build's versions. */
export const BUILD_VERSIONS_BODY = {
  terms_version: CURRENT_TERMS_VERSION,
  privacy_version: CURRENT_PRIVACY_VERSION,
}

/**
 * GET /legal/versions naming versions no build names: a backend that bumped
 * them ahead of this frontend.
 */
export const NEWER_VERSIONS_BODY = {
  terms_version: '2031-04-01',
  privacy_version: '2031-04-02',
}

/**
 * A query client that already holds the published consent versions, so a
 * sign-up reads them without a request. For suites that stub `fetch` and
 * count or order its calls. The entry never goes stale or away, whatever the
 * test client's gcTime.
 */
export function clientWithConsentVersions(
  versions: ConsentVersions = BUILD_CONSENT_VERSIONS,
  queryClient: QueryClient = createTestQueryClient()
): QueryClient {
  queryClient.setQueryDefaults(queryKeys.legal.versions, {
    gcTime: Infinity,
    staleTime: Infinity,
  })
  queryClient.setQueryData(queryKeys.legal.versions, versions)
  return queryClient
}

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

/** A GET /legal/versions answer publishing these versions, no window open. */
export function publishedVersions(versions: {
  terms_version: string
  privacy_version: string
}): Response {
  return jsonResponse({ ...versions, min_signup_age: 16, grace_until: null })
}

/** A GET /legal/versions answer that carries no versions. */
export function versionsUnavailable(): Response {
  return jsonResponse({ detail: 'unavailable' }, 503)
}

/**
 * Answers each GET /legal/versions through MSW with the next answer in turn,
 * the last one repeating, and counts the reads.
 */
export function publishLegalVersionsInTurn(
  ...answers: Array<() => Response | Promise<Response>>
): { reads: () => number } {
  let reads = 0
  server.use(
    http.get(`${TEST_API_BASE}/legal/versions`, () => {
      const answer = answers[Math.min(reads, answers.length - 1)]
      reads += 1
      return answer()
    })
  )
  return { reads: () => reads }
}
