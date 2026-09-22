'use client'

import { useCallback, useMemo } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
// Concrete module path, not the `@/features/auth` barrel: that barrel is
// mocked wholesale by sibling suites and pulling it in here would ship the
// whole auth surface into the home chunk. See sharedChunkBarrelGuard.test.ts.
import { useProfile } from '@/features/auth/hooks/useAuth'
import {
  resolveHomeLayout,
  type HomeLayoutDocument,
  type ResolvedHomeSection,
} from '../sections'

/** The subset of the profile payload these hooks read and write. */
interface ProfileWithHomeLayout {
  user?: { home_layout?: HomeLayoutDocument | null } | null
}

/** Shared key so a write can tell whether another write is still in flight
 *  before it lets the profile refetch. */
const HOME_LAYOUT_MUTATION_KEY = ['auth', 'home-layout'] as const

interface HomeLayoutResponse {
  success: boolean
  message: string
  home_layout?: HomeLayoutDocument | null
}

/**
 * This viewer's home layout, merged with the registry.
 *
 * `fallback` is the document the SERVER read for this request. It is what the
 * first paint renders from, so a viewer with a custom order never sees the
 * shipped order reflow into theirs after hydration. Once the profile query
 * holds a payload (it is hydrated from the same request) that payload wins,
 * which is also how an optimistic write reaches the page.
 */
export function useHomeLayout(
  fallback?: HomeLayoutDocument | null
): ResolvedHomeSection[] {
  const { data } = useProfile()
  const profile = data as ProfileWithHomeLayout | undefined
  // A payload that NAMES a viewer is authoritative, absent field included: the
  // backend omits `home_layout` for a viewer who has none, and reading that as
  // "no answer" would fall back to the server document and undo a reset.
  const stored = profile?.user ? (profile.user.home_layout ?? null) : undefined
  return useMemo(
    () => resolveHomeLayout(stored === undefined ? fallback : stored),
    [stored, fallback]
  )
}

/**
 * Persist a layout, optimistically.
 *
 * The page reads its order from the profile cache, so writing the new document
 * there IS the immediate apply the owner asked for; the PUT only confirms it.
 * A failure restores the exact snapshot taken before the write, so the rows
 * snap back to what the server still holds.
 */
export function useUpdateHomeLayout() {
  const queryClient = useQueryClient()

  return useMutation<
    HomeLayoutResponse,
    Error,
    HomeLayoutDocument,
    { previous: unknown }
  >({
    mutationKey: HOME_LAYOUT_MUTATION_KEY,
    mutationFn: (document: HomeLayoutDocument) =>
      apiRequest<HomeLayoutResponse>(API_ENDPOINTS.AUTH.HOME_LAYOUT, {
        method: 'PUT',
        body: JSON.stringify(document),
      }),
    onMutate: async (document: HomeLayoutDocument) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.auth.profile })
      const previous = queryClient.getQueryData(queryKeys.auth.profile)
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        withHomeLayout(old, document)
      )
      return { previous }
    },
    onError: (_error, _document, context) => {
      if (context) {
        queryClient.setQueryData(queryKeys.auth.profile, context.previous)
      }
    },
    onSettled: () => {
      settleProfile(queryClient)
    },
  })
}

/** Clear the stored document so the shipped order and visibility come back. */
export function useResetHomeLayout() {
  const queryClient = useQueryClient()

  return useMutation<HomeLayoutResponse, Error, void, { previous: unknown }>({
    mutationKey: HOME_LAYOUT_MUTATION_KEY,
    mutationFn: () =>
      apiRequest<HomeLayoutResponse>(API_ENDPOINTS.AUTH.HOME_LAYOUT, {
        method: 'DELETE',
      }),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: queryKeys.auth.profile })
      const previous = queryClient.getQueryData(queryKeys.auth.profile)
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        withHomeLayout(old, null)
      )
      return { previous }
    },
    onError: (_error, _variables, context) => {
      if (context) {
        queryClient.setQueryData(queryKeys.auth.profile, context.previous)
      }
    },
    onSettled: () => {
      settleProfile(queryClient)
    },
  })
}

/**
 * Refetch the profile only once the last write has landed.
 *
 * Every ▲ click fires its own PUT, and an invalidation from the first one
 * resolving mid-run would repaint the page with a document two clicks stale.
 * The settling write counts itself, so "1" means "no other write pending".
 */
function settleProfile(queryClient: ReturnType<typeof useQueryClient>): void {
  if (queryClient.isMutating({ mutationKey: HOME_LAYOUT_MUTATION_KEY }) > 1) {
    return
  }
  queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
}

/** Write `home_layout` into a cached profile payload without disturbing the
 *  rest of it. A cache entry that names no user is left alone: there is no
 *  viewer to hold a layout. */
function withHomeLayout(
  cached: unknown,
  document: HomeLayoutDocument | null
): unknown {
  const profile = cached as ProfileWithHomeLayout | undefined
  if (!profile?.user) return cached
  return { ...profile, user: { ...profile.user, home_layout: document } }
}

/**
 * The one write surface the popover and the settings card share: takes the
 * list the viewer just produced and persists it.
 */
export function usePersistHomeLayout() {
  const update = useUpdateHomeLayout()
  const reset = useResetHomeLayout()

  const persist = useCallback(
    (document: HomeLayoutDocument) => {
      update.mutate(document)
    },
    [update]
  )

  return {
    persist,
    reset: reset.mutate,
    isResetting: reset.isPending,
    /** One inline line covers both writes: the viewer made one gesture and
     *  wants to know whether it stuck. */
    hasError: update.isError || reset.isError,
  }
}
