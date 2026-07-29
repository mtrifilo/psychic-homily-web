export const BUILD_TIME_API_FETCH_TIMEOUT_MS = 10_000

/**
 * The ceiling on a server-render API fetch.
 *
 * The only caller is `lib/seo/fetchSeoList.ts`, which forwards a per-call-site
 * budget — see `app/artists/artistsMetadata.ts` for the one override and the
 * reasoning behind it. `app/sitemap.ts` deliberately bypasses this helper and
 * builds its own signal; its comment explains why.
 */
export function createBuildTimeApiSignal(timeoutMs: number): AbortSignal {
  return AbortSignal.timeout(timeoutMs)
}
