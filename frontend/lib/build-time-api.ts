export const BUILD_TIME_API_FETCH_TIMEOUT_MS = 10_000

/**
 * The ceiling on a server-render API fetch. Pass a wider budget only with the
 * reason written down at the call site — see `app/sitemap.ts` and
 * `app/artists/artistsMetadata.ts` for the two that do.
 */
export function createBuildTimeApiSignal(
  timeoutMs: number = BUILD_TIME_API_FETCH_TIMEOUT_MS
): AbortSignal {
  return AbortSignal.timeout(timeoutMs)
}
