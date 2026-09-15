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
 * READING `searchParams` HERE COSTS THIS ROUTE ITS PRERENDERED SHELL, which the
 * month route beside it keeps. It is the price of a run being addressed by a
 * query parameter at all: the head has to name the span and mark it noindex, and
 * nothing but the query says how long it is. `?page=` is deliberately not read —
 * every page of a window still canonicalizes to the window root.
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
