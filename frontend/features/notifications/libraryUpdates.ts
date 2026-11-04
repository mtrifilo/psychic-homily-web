/**
 * The read behind the signed-in home's "From your library" module: the
 * viewer's newest inbox rows about the artists and venues they follow, from the
 * same GET /me/notifications the bell and the inbox read, narrowed by `types`.
 *
 * ISOMORPHIC ON PURPOSE: no `'use client'`, no client-only imports. The server
 * prefetch in `lib/auth-hydration.ts` and the client hook must build the SAME
 * key and URL, and a value imported from a `'use client'` module arrives in a
 * server module as a client reference rather than the value.
 */

import { NOTIFICATION_LOG_KEY } from './queryKeys'
import {
  NOTIFICATION_ENTITY_ARTIST_RELEASE_DIGEST,
  NOTIFICATION_ENTITY_ARTIST_SHOW_ALERT,
  NOTIFICATION_ENTITY_VENUE_SHOW_ALERT,
} from './types'

/** Rows the module paints (owner decision, 2026-11-03). */
export const LIBRARY_UPDATES_LIMIT = 5

/**
 * The discriminators the module asks for. Each must be one the backend's
 * `types` vocabulary accepts (an unknown one is a 422 for the whole read) and
 * one `describeLibraryUpdate` can render.
 */
export const LIBRARY_UPDATE_ENTITY_TYPES = [
  NOTIFICATION_ENTITY_ARTIST_SHOW_ALERT,
  NOTIFICATION_ENTITY_VENUE_SHOW_ALERT,
  NOTIFICATION_ENTITY_ARTIST_RELEASE_DIGEST,
] as const

const LIBRARY_UPDATE_TYPES_PARAM = LIBRARY_UPDATE_ENTITY_TYPES.join(',')

/** Path and query, relative to the API base. */
export const LIBRARY_UPDATES_PATH = `/me/notifications?${new URLSearchParams({
  limit: String(LIBRARY_UPDATES_LIMIT),
  offset: '0',
  types: LIBRARY_UPDATE_TYPES_PARAM,
})}`

/** Under the notification-log prefix, so marking a row read anywhere (here,
 *  the bell, the inbox) refreshes this entry's unread dots. */
export const LIBRARY_UPDATES_QUERY_KEY = [
  ...NOTIFICATION_LOG_KEY,
  { limit: LIBRARY_UPDATES_LIMIT, offset: 0, types: LIBRARY_UPDATE_TYPES_PARAM },
] as const
