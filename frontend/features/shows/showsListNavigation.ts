/**
 * URL shape for the `/shows` list's own navigation.
 *
 * Pure: no React, no router, no fetching, so the rules that decide what a page
 * link says can be tested without a rendered list.
 */

import { listPageHref } from '@/components/shared/paginationChrome'
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

/** {@link listPageHref} rooted at the shows list. */
export function showsPageHref(
  params: { toString: () => string },
  page: number,
  basePath: string = SHOWS_ROOT
): string {
  return listPageHref(params, page, basePath)
}
