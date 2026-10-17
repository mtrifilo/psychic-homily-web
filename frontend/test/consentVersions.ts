import type { QueryClient } from '@tanstack/react-query'
import type { ConsentVersions } from '@/features/auth/hooks/useLegalVersions'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION } from '@/lib/legal'
import { queryKeys } from '@/lib/queryClient'
import { createTestQueryClient } from './utils'

export const BUILD_CONSENT_VERSIONS: ConsentVersions = {
  termsVersion: CURRENT_TERMS_VERSION,
  privacyVersion: CURRENT_PRIVACY_VERSION,
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
