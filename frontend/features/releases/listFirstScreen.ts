import { releaseEndpoints, releaseQueryKeys } from './api'

/** The page size `ReleaseList` requests, and pages through with `offset`. */
export const RELEASE_LIST_PAGE_LIMIT = 50

/** The order a bare `/releases` lists in; `?sort=` never spells it. */
export const RELEASE_LIST_DEFAULT_SORT = 'newest'

/**
 * The exact request `ReleaseList` issues on its first render of a bare
 * `/releases`, and the cache key that request lands on.
 *
 * `app/releases/page.tsx` fetches the URL server-side and seeds the key, so the
 * first page of releases is in the server HTML. The two only work as a pair: a
 * seeded key the hook does not ask for leaves both render passes on the loader,
 * which looks like nothing at all, so `app/releases/page.test.tsx` renders the
 * real list over the seed.
 *
 * `offset` and `tagMatch` are in the KEY but not the URL: `useReleases` keys on
 * every option it is handed, and omits a zero offset and the default tag
 * semantics from the request.
 *
 * No directive: the server page and the client list both import this module,
 * and a value from a `'use client'` module reaches a server module as a client
 * reference rather than as the value.
 */
export const RELEASE_LIST_FIRST_SCREEN_URL = `${releaseEndpoints.LIST}?sort=${RELEASE_LIST_DEFAULT_SORT}&limit=${RELEASE_LIST_PAGE_LIMIT}`

export const RELEASE_LIST_FIRST_SCREEN_KEY = releaseQueryKeys.list({
  releaseType: undefined,
  year: undefined,
  search: undefined,
  sort: RELEASE_LIST_DEFAULT_SORT,
  labelId: undefined,
  tags: undefined,
  tagMatch: 'all',
  limit: RELEASE_LIST_PAGE_LIMIT,
  offset: 0,
})

/**
 * Search params that move `ReleaseList` off the first-screen key. Any of them
 * carrying a value skips the seed (see `isFirstScreenRequest`).
 */
export const RELEASE_LIST_FIRST_SCREEN_DEFEATING_PARAMS = [
  'page',
  'type',
  'year',
  'search',
  'sort',
  'label_id',
  'tags',
  'tag_match',
] as const
