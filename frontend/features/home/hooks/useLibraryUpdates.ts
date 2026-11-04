'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { components } from '@/types/api'
import {
  LIBRARY_UPDATES_LIMIT,
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
 * Rows of kinds this surface cannot render are dropped here rather than in the
 * component, so "no rows" means the same thing to the empty rule as it does
 * to the list. Module scope keeps the `select` reference stable.
 */
function selectLibraryUpdates(data: LibraryUpdatesResponse): LibraryUpdate[] {
  const updates: LibraryUpdate[] = []
  for (const entry of data.notifications ?? []) {
    const copy = describeLibraryUpdate(entry)
    if (copy) updates.push({ id: entry.id, unread: entry.read_at == null, copy })
  }
  return updates.slice(0, LIBRARY_UPDATES_LIMIT)
}

/**
 * The viewer's newest followed-entity updates for the signed-in home.
 *
 * The key is shared with the server prefetch (`lib/auth-hydration.ts`), so a
 * prefetched first paint is fresh on mount. It sits under the notification-log
 * prefix, so marking a row read anywhere (here, the bell, the inbox) refreshes
 * it. No poll of its own: the bell already polls its own entry, and this one
 * refreshes on focus and on every mark-read.
 */
export function useLibraryUpdates() {
  const { isAuthenticated } = useAuthContext()
  return useQuery({
    queryKey: LIBRARY_UPDATES_QUERY_KEY,
    queryFn: () =>
      apiRequest<LibraryUpdatesResponse>(`${API_BASE_URL}${LIBRARY_UPDATES_PATH}`),
    select: selectLibraryUpdates,
    enabled: isAuthenticated,
    staleTime: 60 * 1000,
    refetchOnWindowFocus: true,
  })
}
