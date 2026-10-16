import type {
  InvalidateQueryFilters,
  QueryClient,
  QueryKey,
} from '@tanstack/react-query'

/** A query key prefix, or full filters for a target a key cannot describe. */
export type RefetchTarget = QueryKey | InvalidateQueryFilters

/**
 * Cancel every query each target matches, then invalidate them, so the next
 * data each one shows reflects the write. Active queries refetch now; inactive
 * and disabled ones are only marked stale, and any fetch they had in flight is
 * dropped.
 *
 * `invalidateQueries` alone does not guarantee that. For a query that has no
 * data yet and a fetch in flight, TanStack Query hands back the in-flight
 * promise instead of starting a new request, and that response may have been
 * produced before the write committed. Its success then clears the
 * invalidation, so nothing refetches until the query goes stale. Cancelling
 * first leaves no fetch to reuse, and a cancelled fetch's late response is
 * discarded.
 *
 * Every target is cancelled before any is invalidated. Pass each query once:
 * a key already covered by another target's prefix makes a member with data
 * refetch twice.
 *
 * The invalidation runs only after the cancel settles, a few microtasks after
 * the call, so it also marks stale anything the caller writes with
 * `setQueryData` under a target in the meantime; exclude such an entry with a
 * filters target. Neither step rejects: both swallow fetch errors. Callers
 * that do not need the refetch to finish `void` the returned promise, so the
 * mutation settles without waiting for it.
 */
export async function refetchAfterMutation(
  queryClient: QueryClient,
  ...targets: RefetchTarget[]
): Promise<void> {
  const filters = targets.map(toFilters)
  await Promise.all(filters.map(f => queryClient.cancelQueries(f)))
  await Promise.all(filters.map(f => queryClient.invalidateQueries(f)))
}

function toFilters(target: RefetchTarget): InvalidateQueryFilters {
  return isQueryKey(target) ? { queryKey: target } : target
}

function isQueryKey(target: RefetchTarget): target is QueryKey {
  return Array.isArray(target)
}
