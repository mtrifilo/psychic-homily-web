export const BUILD_TIME_API_FETCH_TIMEOUT_MS = 10_000

export function createBuildTimeApiSignal(): AbortSignal {
  return AbortSignal.timeout(BUILD_TIME_API_FETCH_TIMEOUT_MS)
}
