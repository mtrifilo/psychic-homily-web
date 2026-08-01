export const BUILD_TIME_API_FETCH_TIMEOUT_MS = 10_000

/**
 * The ceiling on a server-render API fetch.
 *
 * Two callers, both forwarding a per-call-site budget: `lib/seo/fetchSeoList.ts`
 * (see `app/artists/artistsMetadata.ts` for the one override and the reasoning
 * behind it) and `lib/ssr/fetchListPayload.ts`, which defaults to a SHORTER
 * budget because its seeds are read at request time — so the constant above is
 * no longer the effective ceiling everywhere. `app/sitemap.ts` deliberately
 * bypasses this helper and builds its own signal; its comment explains why.
 */
export function createBuildTimeApiSignal(timeoutMs: number): AbortSignal {
  return AbortSignal.timeout(timeoutMs)
}
