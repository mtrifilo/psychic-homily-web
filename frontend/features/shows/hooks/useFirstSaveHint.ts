'use client'

import { useCallback } from 'react'
import {
  useMutation,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import {
  readProfilePreference,
  readProfileViewerId,
  settleProfilePreference,
  withProfilePreference,
} from '@/features/auth/hooks/profilePreferenceCache'
import type { components } from '@/types/api'
import type { SavedShowsListResponse } from '../types'

type DismissFirstSaveHintResponse =
  components['schemas']['DismissFirstSaveHintResponseBody']

const DISMISSED_AT = 'first_save_hint_dismissed_at'

/**
 * Whether the viewer the cached profile names has NOT dismissed the hint. A
 * cache entry that names no viewer answers false: there is nobody to show it
 * to, and an unknown answer must not render a once-ever message.
 */
function isHintUndismissed(cached: unknown): boolean {
  return (
    readProfileViewerId(cached) != null &&
    readProfilePreference<string>(cached, DISMISSED_AT) == null
  )
}

/**
 * Viewers whose first-save question is already answered for the lifetime of a
 * query client (one browser session): the hint opened for them, or a save
 * found them past their first. Keyed per client so a new client, including
 * each test's, starts empty.
 */
const answeredViewers = new WeakMap<QueryClient, Set<unknown>>()

/**
 * Called after a save SUCCEEDS: whether that save should open the first-save
 * hint. True only when the viewer has not dismissed the hint, has not had the
 * question answered this session, and the save took them from zero saved
 * shows to exactly one.
 *
 * The flag is read from the profile cache at call time rather than through a
 * subscription, so the dozens of Save controls on a list page do not each
 * re-render on every profile change.
 *
 * Cost: while the flag is unset, the FIRST save of each session reads the
 * saved-show count once. That includes every account that had saves before
 * the flag existed and every viewer who never dismissed the hint, since their
 * flag stays unset.
 * Any answer of one or more ends the question for the session, so the read
 * never repeats per save, and the hint cannot reopen in the same session.
 */
export async function shouldOpenFirstSaveHint(
  queryClient: QueryClient,
  isStillCurrent: () => boolean = () => true
): Promise<boolean> {
  const cached = queryClient.getQueryData(queryKeys.auth.profile)
  const viewerId = readProfileViewerId(cached)
  if (!isHintUndismissed(cached)) return false
  const answered = answeredViewers.get(queryClient) ?? new Set<unknown>()
  answeredViewers.set(queryClient, answered)
  if (answered.has(viewerId)) return false

  const params = new URLSearchParams({ limit: '1', offset: '0' })
  const page = await apiRequest<SavedShowsListResponse>(
    `${API_ENDPOINTS.SAVED_SHOWS.LIST}?${params.toString()}`,
    { method: 'GET' }
  )
  // An answer to a question that no longer stands (the viewer unsaved or
  // saved again while the read was in flight) settles nothing: the newer
  // click asks for itself. Zero likewise means the save had not landed in
  // the count yet, so the question stays open.
  if (!isStillCurrent()) return false
  if (page.total >= 1) answered.add(viewerId)
  return page.total === 1
}

/** `shouldOpenFirstSaveHint` bound to this tree's query client. */
export function useShouldOpenFirstSaveHint(): (
  isStillCurrent: () => boolean
) => Promise<boolean> {
  const queryClient = useQueryClient()
  return useCallback(
    (isStillCurrent: () => boolean) =>
      shouldOpenFirstSaveHint(queryClient, isStillCurrent),
    [queryClient]
  )
}

/**
 * Stamp the hint dismissed on the account, optimistically: the profile cache
 * carries the flag at once, so no other Save control can open the hint again
 * while the request is in flight. Only null versus set is ever read, so the
 * server's stored time is not reconciled into the cache.
 *
 * A failed write restores the flag and is not surfaced. The hint has already
 * closed at the viewer's request, and the only cost of the failure is that the
 * hint may open again at a later first save.
 */
export function useDismissFirstSaveHint() {
  const queryClient = useQueryClient()

  return useMutation<
    DismissFirstSaveHintResponse,
    Error,
    void,
    { viewerId: unknown; previous: string | null }
  >({
    mutationFn: () =>
      apiRequest<DismissFirstSaveHintResponse>(
        API_ENDPOINTS.AUTH.FIRST_SAVE_HINT,
        { method: 'PUT' }
      ),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: queryKeys.auth.profile })
      const cached = queryClient.getQueryData(queryKeys.auth.profile)
      const context = {
        viewerId: readProfileViewerId(cached),
        previous: readProfilePreference<string>(cached, DISMISSED_AT),
      }
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        withProfilePreference(
          old,
          DISMISSED_AT,
          context.previous ?? new Date().toISOString()
        )
      )
      return context
    },
    onError: (_error, _vars, context) => {
      if (!context) return
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        settleProfilePreference(
          old,
          context.viewerId,
          DISMISSED_AT,
          context.previous
        )
      )
    },
  })
}
