import type { QueryClient, QueryKey } from '@tanstack/react-query'

/**
 * Refetch every query under `queryKey` after a write, so the response reflects
 * the write.
 *
 * `invalidateQueries` alone does not guarantee that. For a query that has no
 * data yet and a fetch in flight, TanStack Query hands back the in-flight
 * promise instead of starting a new request, and that response may have been
 * produced before the write committed. Its success then clears the
 * invalidation, so nothing refetches until the query goes stale. Cancelling
 * first leaves no fetch to reuse, and a cancelled fetch's late response is
 * discarded.
 *
 * Use it for a write whose result a query may still be loading for the first
 * time, such as a list whose form renders before the list has loaded.
 */
export async function refetchAfterMutation(
  queryClient: QueryClient,
  queryKey: QueryKey
): Promise<void> {
  await queryClient.cancelQueries({ queryKey })
  await queryClient.invalidateQueries({ queryKey })
}
