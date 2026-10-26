import { API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { DEFAULT_TAG_SORT, TAG_CATEGORIES, TAG_SORT_OPTIONS } from './types'
import type { TagSortOption } from './types'

/** The page size `TagBrowse` requests, and pages through with `offset`. */
export const TAG_BROWSE_PAGE_SIZE = 50

/** The `limit` of each per-category count read: only `total` is used. */
export const TAG_CATEGORY_COUNT_LIMIT = 1

/** The backend `sort` value for a `TagBrowse` sort option. */
export function tagSortToBackend(sort: TagSortOption): string {
  return TAG_SORT_OPTIONS.find(o => o.value === sort)?.backend ?? 'usage'
}

const DEFAULT_BACKEND_SORT = tagSortToBackend(DEFAULT_TAG_SORT)

/** One request `TagBrowse` issues on a bare `/tags`, and the key it lands on. */
export interface TagBrowseFirstScreenRead {
  url: string
  queryKey: readonly unknown[]
}

/**
 * The exact requests `TagBrowse` issues on its first render of a bare `/tags`,
 * and the cache keys they land on: the first page of the directory, then one
 * count per category for the facet chips.
 *
 * `app/tags/page.tsx` fetches the URLs server-side and seeds the keys, so the
 * directory and its chip counts are in the server HTML. The counts are part of
 * the first screen: without them the chips render as zero and disabled over a
 * table that is full. The pairs only work when they match what the hooks ask
 * for, so `app/tags/page.test.tsx` renders the real directory over the seed.
 *
 * Each key is the params object `TagBrowse` hands `useTags`; the hook keys on
 * it as given and omits undefined params and a zero offset from the request.
 *
 * No directive: the server page and the client directory both import this
 * module, and a value from a `'use client'` module reaches a server module as a
 * client reference rather than as the value.
 */
export const TAG_BROWSE_FIRST_SCREEN_READS: readonly TagBrowseFirstScreenRead[] = [
  {
    url: `${API_ENDPOINTS.TAGS.LIST}?sort=${DEFAULT_BACKEND_SORT}&limit=${TAG_BROWSE_PAGE_SIZE}`,
    queryKey: queryKeys.tags.list({
      category: undefined,
      search: undefined,
      limit: TAG_BROWSE_PAGE_SIZE,
      offset: 0,
      sort: DEFAULT_BACKEND_SORT,
    }),
  },
  ...TAG_CATEGORIES.map(category => ({
    url: `${API_ENDPOINTS.TAGS.LIST}?category=${category}&limit=${TAG_CATEGORY_COUNT_LIMIT}`,
    queryKey: queryKeys.tags.list({
      category,
      search: undefined,
      limit: TAG_CATEGORY_COUNT_LIMIT,
    }),
  })),
]

/**
 * Search params that move `TagBrowse` off the first-screen keys. Category,
 * search and page are component state and always start at their defaults.
 *
 * Not self-maintaining: a URL param the list starts reading has to be added
 * here too, or the server keeps seeding a key the list no longer asks for.
 * The page test records every param the mounted list reads and fails on one
 * missing from this list.
 */
export const TAG_BROWSE_FIRST_SCREEN_DEFEATING_PARAMS = ['sort'] as const
