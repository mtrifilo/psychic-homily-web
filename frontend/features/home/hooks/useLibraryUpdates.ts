'use client'

import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { components } from '@/types/api'
import {
  LIBRARY_UPDATES_PATH,
  LIBRARY_UPDATES_QUERY_KEY,
} from '@/features/notifications/libraryUpdates'
import {
  describeLibraryUpdate,
  type UpdateRowCopy,
} from '@/features/notifications/updateRowCopy'

type LibraryUpdatesResponse =
  components['schemas']['GetNotificationsResponseBody']

export interface LibraryUpdate {
  id: number
  unread: boolean
  copy: UpdateRowCopy
}

/**
 * The viewer's newest followed-entity updates for the signed-in home, or
 * `undefined` until the read has answered (or when it failed).
 *
 * Recency is measured from `dataUpdatedAt`, the instant the rows were last
 * read, not from the clock: a server-prefetched entry hydrates with the same
 * `dataUpdatedAt`, so the server render and the hydrating client print the
 * same "2d ago". It advances on every refetch (a mount past `staleTime`, a
 * window focus, any mark-read), and holds still between them.
 *
 * Rows of kinds this surface cannot render are dropped here rather than in
 * the component, so "no updates" means the same thing to the empty rule as it
 * does to the list.
 *
 * The key is shared with the server prefetch (`lib/auth-hydration.ts`) and
 * sits under the notification-log prefix, so marking a row read anywhere (here,
 * the bell, the inbox) refreshes it. No poll of its own: it refreshes on focus
 * and on every mark-read.
 */
export function useLibraryUpdates(): LibraryUpdate[] | undefined {
  const { isAuthenticated } = useAuthContext()
  const { data, dataUpdatedAt } = useQuery({
    queryKey: LIBRARY_UPDATES_QUERY_KEY,
    queryFn: () =>
      apiRequest<LibraryUpdatesResponse>(`${API_BASE_URL}${LIBRARY_UPDATES_PATH}`),
    enabled: isAuthenticated,
    staleTime: 60 * 1000,
    refetchOnWindowFocus: true,
  })

  return useMemo(() => {
    if (!data) return undefined
    const readAt = new Date(dataUpdatedAt)
    const updates: LibraryUpdate[] = []
    for (const entry of data.notifications ?? []) {
      const copy = describeLibraryUpdate(entry, readAt)
      if (copy) updates.push({ id: entry.id, unread: entry.read_at == null, copy })
    }
    return updates
  }, [data, dataUpdatedAt])
}
