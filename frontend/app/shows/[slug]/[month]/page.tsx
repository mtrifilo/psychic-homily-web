import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  ShowsCalendarRoute,
  showsCalendarRouteMetadata,
  type ShowsCalendarSearchParams,
} from '@/features/shows/calendarPage'
import { parseMonthSegments } from '@/features/shows/showsCalendarRoute'

/**
 * `/shows/{yyyy}/{mm}` — the upcoming list scoped to one venue-local month.
 *
 * THE FIRST SEGMENT IS NAMED `[slug]` BECAUSE THE ROUTER REQUIRES IT. Next
 * refuses two different dynamic names at one position, and `app/shows/[slug]`
 * is the show detail route, so a month's year has to travel under that name.
 * It is destructured to a window immediately and never used as a slug; the same
 * shape `app/charts/[module]` carries for the chart archives.
 *
 * A year segment that is also a real show slug cannot happen: a slug is one
 * segment and these URLs are two, so `/shows/2026` still resolves to the show
 * detail route and `/shows/2026/11` to this one. They do not compete.
 *
 * This file parses segments and nothing else. Every read, and the page shell
 * around it, lives in `features/shows/calendarPage` with the day route's.
 */
interface ShowsMonthRouteProps {
  params: Promise<{ slug: string; month: string }>
  /**
   * Passed straight through and awaited under the boundary, never here.
   * Awaiting it in this body would make the whole route dynamic and cost it the
   * prerendered shell. `generateMetadata` does not take it at all — that is what
   * keeps every `?page=` of a month on one canonical.
   */
  searchParams: ShowsCalendarSearchParams
}

export async function generateMetadata({
  params,
}: Pick<ShowsMonthRouteProps, 'params'>): Promise<Metadata> {
  const { slug, month } = await params
  return showsCalendarRouteMetadata(parseMonthSegments(slug, month))
}

export default async function ShowsMonthPage({
  params,
  searchParams,
}: ShowsMonthRouteProps) {
  const { slug, month } = await params
  const window = parseMonthSegments(slug, month)
  if (window === null) {
    notFound()
  }

  return <ShowsCalendarRoute window={window} searchParams={searchParams} />
}
