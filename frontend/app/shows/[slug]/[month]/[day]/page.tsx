import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  ShowsCalendarRoute,
  showsCalendarRouteMetadata,
  type ShowsCalendarSearchParams,
} from '@/features/shows/calendarPage'
import { parseDaySegments } from '@/features/shows/showsCalendarRoute'

/**
 * `/shows/{yyyy}/{mm}/{dd}` — the upcoming list scoped to one venue-local day.
 *
 * The first segment is named `[slug]` for the router's sake; see the month
 * route above it for why. It carries the year here.
 *
 * `parseDaySegments` refuses a day the calendar does not have (`2027/02/31`),
 * which a range check alone would wave through.
 */
interface ShowsDayRouteProps {
  params: Promise<{ slug: string; month: string; day: string }>
  /** Awaited under the boundary, never here. See the month route. */
  searchParams: ShowsCalendarSearchParams
}

export async function generateMetadata({
  params,
}: Pick<ShowsDayRouteProps, 'params'>): Promise<Metadata> {
  const { slug, month, day } = await params
  return showsCalendarRouteMetadata(parseDaySegments(slug, month, day))
}

export default async function ShowsDayPage({
  params,
  searchParams,
}: ShowsDayRouteProps) {
  const { slug, month, day } = await params
  const window = parseDaySegments(slug, month, day)
  if (window === null) {
    notFound()
  }

  return <ShowsCalendarRoute window={window} searchParams={searchParams} />
}
