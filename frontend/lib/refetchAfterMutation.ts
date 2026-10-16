import type { QueryClient, QueryKey } from '@tanstack/react-query'

/**
 * Cancel every query under each of `queryKeys`, then invalidate them, so the
 * next data each one shows reflects the write. Active queries refetch now;
 * inactive and disabled ones are only marked stale, and any fetch they had in
 * flight is dropped.
 *
 * `invalidateQueries` alone does not guarantee that. For a query that has no
 * data yet and a fetch in flight, TanStack Query hands back the in-flight
 * promise instead of starting a new request, and that response may have been
 * produced before the write committed. Its success then clears the
 * invalidation, so nothing refetches until the query goes stale. Cancelling
 * first leaves no fetch to reuse, and a cancelled fetch's late response is
 * discarded.
 *
 * Every key is cancelled before any is invalidated, so with overlapping keys
 * (a family root and one of its members) no cancel can land on a refetch that
 * another key's invalidation has just started.
 *
 * Mutation hooks call it from `onSuccess` or `onSettled` with `void`, so the
 * mutation settles without waiting for the refetch, as a bare
 * `invalidateQueries` did. Neither step rejects: both swallow fetch errors.
 */
export async function refetchAfterMutation(
  queryClient: QueryClient,
  ...queryKeys: QueryKey[]
): Promise<void> {
  await Promise.all(
    queryKeys.map(queryKey => queryClient.cancelQueries({ queryKey }))
  )
  await Promise.all(
    queryKeys.map(queryKey => queryClient.invalidateQueries({ queryKey }))
  )
}
