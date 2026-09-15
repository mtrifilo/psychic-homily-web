/**
 * URL shape for the `/shows` list's own navigation.
 *
 * Pure: no React, no router, no fetching, so the rules that decide what a page
 * link says can be tested without a rendered list.
 */

import { SHOWS_ROOT } from './showsCalendarRoute'

/**
 * Rows per page on `/shows`.
 *
 * Stated rather than inherited. The backend's `default:"50"` is the same
 * number, but the pager's arithmetic (which page a row ordinal falls on, which
 * months a page covers) has to agree with the limit the request actually
 * carried, and a default living in another repo layer could move without
 * anything here noticing. The request sends this explicitly for the same
 * reason.
 */
export const SHOWS_PAGE_SIZE = 50

/**
 * The href for a 1-based page of the list rooted at `basePath`, built FROM the
 * params already on screen.
 *
 * Every key but `page` is carried through untouched. The list shares its query
 * string with the city filter, the tag filter, and whatever a campaign link
 * brought along, and a pager that minted a fresh `URLSearchParams` would
 * silently drop all of it on every page click.
 *
 * Page 1 writes NO `page`, so the first page of any filter state has exactly
 * one address and the root is reachable by paging back.
 *
 * `basePath` is what makes `?page=` mean the same thing inside a month as it
 * does on the root: the page axis is a query in both places, and only the list
 * it pages through changes.
 */
export function showsPageHref(
  params: URLSearchParams | { toString: () => string },
  page: number,
  basePath: string = SHOWS_ROOT
): string {
  const next = new URLSearchParams(params.toString())
  if (page > 1) next.set('page', String(page))
  else next.delete('page')
  const query = next.toString()
  return query ? `${basePath}?${query}` : basePath
}
