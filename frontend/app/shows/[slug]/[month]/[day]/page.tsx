import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  ShowsCalendarRoute,
  showsCalendarRouteMetadata,
  type ShowsCalendarSearchParams,
} from '@/features/shows/calendarPage'
import {
  applyWindowDays,
  parseDaySegments,
  type ShowsCalendarWindow,
} from '@/features/shows/showsCalendarRoute'

/**
 * `/shows/{yyyy}/{mm}/{dd}`, the upcoming list scoped to one venue-local day,
 * and `?days=N` of it scoped to the run of days beginning there.
 *
 * The first segment is named `[slug]` for the router's sake; see the month
 * route above it for why. It carries the year here.
 *
 * `parseDaySegments` refuses a day the calendar does not have (`2027/02/31`),
 * which a range check alone would wave through.
 */
interface ShowsDayRouteProps {
  params: Promise<{ slug: string; month: string; day: string }>
  searchParams: ShowsCalendarSearchParams
}

/**
 * The window this URL names, or `null` when it names none.
 *
 * Read by the head and by the body from the same two inputs, so the span in the
 * `<title>` is the span the page lists. A `?days=` outside the bound is a
 * not-found rather than the nearest run it would serve: a window names the days
 * it lists, and quietly answering with a different span is the kind of wrong a
 * reader cannot see.
 *
 * The head reads `searchParams` because a run is addressed by one: the `<title>`
 * has to name the span and the page has to carry `noindex`, and nothing but the
 * query says how long the run is. `?page=` is deliberately NOT read, which is
 * what keeps every page of a window on the window root's canonical.
 */
async function dayWindow(
  params: ShowsDayRouteProps['params'],
  searchParams: ShowsCalendarSearchParams
): Promise<ShowsCalendarWindow | null> {
  const [{ slug, month, day }, query] = await Promise.all([params, searchParams])
  const window = parseDaySegments(slug, month, day)
  return window === null ? null : applyWindowDays(window, query.days)
}

export async function generateMetadata({
  params,
  searchParams,
}: ShowsDayRouteProps): Promise<Metadata> {
  return showsCalendarRouteMetadata(await dayWindow(params, searchParams))
}

export default async function ShowsDayPage({
  params,
  searchParams,
}: ShowsDayRouteProps) {
  const window = await dayWindow(params, searchParams)
  if (window === null) {
    notFound()
  }

  return <ShowsCalendarRoute window={window} searchParams={searchParams} />
}
