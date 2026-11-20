import { festivalEndpoints, festivalQueryKeys } from './api'

/**
 * The exact request `FestivalList` issues on its first render of a bare
 * `/festivals`, and the cache key that request lands on.
 *
 * `app/festivals/page.tsx` fetches the URL server-side and seeds the key, so
 * the festivals are in the server HTML. The two only work as a pair: a seeded
 * key the hook does not ask for leaves both render passes on the loader, so
 * `app/festivals/page.test.tsx` renders the real list over the seed.
 *
 * `useFestivals` keys an unfiltered request on `undefined` filters and sends no
 * query string. The endpoint is unpaginated, so this is every festival.
 *
 * No directive: the server page imports this module, and a value from a
 * `'use client'` module reaches a server module as a client reference rather
 * than as the value.
 */
export const FESTIVAL_LIST_FIRST_SCREEN_URL = festivalEndpoints.LIST

export const FESTIVAL_LIST_FIRST_SCREEN_KEY = festivalQueryKeys.list(undefined)

/**
 * Search params that move `FestivalList` off the first-screen key. Any of them
 * carrying a value skips the seed (see `isFirstScreenRequest`).
 *
 * Not self-maintaining: a URL param the list starts reading has to be added
 * here too, or the server keeps seeding a key the list no longer asks for.
 * The page test records every param the mounted list reads and fails on one
 * missing from this list.
 */
export const FESTIVAL_LIST_FIRST_SCREEN_DEFEATING_PARAMS = [
  'status',
  'year',
  'city',
  'tags',
  'tag_match',
] as const
