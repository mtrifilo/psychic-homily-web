/**
 * The artist-shaped face of the shared show archive (PSY-1754).
 *
 * Two things, mirroring `features/venues/showArchive.ts` (PSY-1842). The first
 * is where a row's TIMEZONE comes from — the one thing that genuinely differs
 * between the entities: a venue archive lists ONE venue's shows so the zone is
 * the venue's for every row, an artist archive lists shows ACROSS venues so each
 * row is placed in its own venue's zone. The second is the archive's URL SPACE.
 *
 * The derivations themselves live in `@/features/shows/showArchive` and the
 * rendering in `PastShowsArchive`, both shared with the venue archive.
 */

import type { ShowZone } from '@/features/shows/showArchive'
import type { ArtistShow } from './types'

/**
 * Where one of an artist's shows is read on the calendar: its own venue's
 * resolved IANA zone, falling back to the venue's state until the timezone
 * backfill reaches it (PSY-985/986).
 *
 * A row with no venue at all resolves to NEITHER, and `resolveShowTimezone`
 * then hands back its silent America/Phoenix default — the same zone for every
 * reader on earth, not the reader's own. That default is wrong by up to most of
 * a day for a show outside the US, which is why `isShowTimezoneResolved` exists;
 * this surface does not consult it yet, and neither does the venue archive.
 * Gating the clock column on it is a follow-up for both.
 */
export function artistShowZone(show: Pick<ArtistShow, 'venue'>): ShowZone {
  return { state: show.venue?.state, timezone: show.venue?.timezone }
}

/**
 * The fragment the artist page's past-shows section is addressed by.
 *
 * Declared here rather than in the component because this module owns the artist
 * archive's URL SPACE, and {@link artistArchiveHref} has to append it. The
 * component re-exports it as `ARTIST_PAST_SHOWS_ANCHOR` for the markup that
 * carries the id — the same arrangement the venue archive uses.
 */
export const ARTIST_PAST_SHOWS_FRAGMENT = 'artist-past-shows'

/**
 * THE address of one view of an artist's past-show archive.
 *
 * The space it defines, in full:
 *
 *   every year, page 1   /artists/{slug}#artist-past-shows
 *   every year, page N   /artists/{slug}?page=N#artist-past-shows
 *   one year,   page 1   /artists/{slug}?year=Y#artist-past-shows
 *   one year,   page N   /artists/{slug}?year=Y&page=N#artist-past-shows
 *
 * THE YEAR IS A QUERY PARAM, not a path segment, and that is the one place this
 * deliberately differs from `venueArchiveHref`. A venue's year archive is its own
 * crawlable document (`/venues/{slug}/shows/{year}`), so a `?year=` form there
 * would put the same rows at two addresses with no canonical relationship — the
 * duplicate-content shape PSY-1756 removed. The artist archive has no per-year
 * route, so there is no second address for this form to duplicate. When one is
 * built, this is the function that has to move first.
 *
 * IT BUILDS FROM THE PARAMS ALREADY ON THE URL rather than from an empty set,
 * which `venueArchiveHref` does not have to do. The venue page is the only
 * query-param writer on its route; the artist page is not — the connections graph
 * pushes `?center=<slug>` onto the same URL and leaves it there when its dialog
 * closes, and IT already preserves `year`/`page`. Building from scratch here
 * would make that courtesy one-way: paging the archive would silently drop the
 * reader's graph center, and a shared link would lose it too. Page 1 and "all
 * years" are still bare, so there is one canonical address per view — that rule
 * is about OUR two params, and anything else on the URL belongs to another owner
 * and is carried through untouched.
 *
 * THE SLUG FALLS BACK TO THE ID, which resolves on the same route. `slug` is
 * nullable in the DB and the API sends "" for a missing one — and `GenerateSlug`
 * returns "" for any name with no [a-z0-9] characters at all, so a band called
 * `!!!` or `少年ナイフ` reaches this page slugless. An unguarded
 * `/artists/${''}` is `/artists/`, which is not a 404 but the artists INDEX:
 * every year link, every page link and both "back to the first page" links would
 * silently eject the reader from the archive PSY-1754 exists to make navigable.
 * Handled HERE rather than at the caller so no future caller can forget it.
 */
export function artistArchiveHref({
  artistSlug,
  artistId,
  currentParams,
  year,
  page,
}: {
  artistSlug: string
  artistId: number
  /** Everything already on the URL, so params this archive does not own survive. */
  currentParams: URLSearchParams
  year: number | null
  page: number
}): string {
  const params = new URLSearchParams(currentParams)
  if (year === null) params.delete('year')
  else params.set('year', String(year))
  if (page > 1) params.set('page', String(page))
  else params.delete('page')

  const query = params.toString()
  const basePath = `/artists/${artistSlug || artistId}`
  return `${basePath}${query ? `?${query}` : ''}#${ARTIST_PAST_SHOWS_FRAGMENT}`
}
