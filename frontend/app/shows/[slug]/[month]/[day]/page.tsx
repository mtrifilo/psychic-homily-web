import { Suspense } from 'react'
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { ShowListSkeleton } from '@/features/shows'
import {
  ShowsCalendarContent,
  buildShowsCalendarMetadata,
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
  /**
   * Passed straight through to `ShowsCalendarContent` and awaited THERE, never
   * here. Awaiting it in this body would make the whole route dynamic and cost
   * it the prerendered shell. `generateMetadata` does not take it at all —
   * that is what keeps every `?page=` of a day on one canonical.
   */
  searchParams: ShowsCalendarSearchParams
}

export async function generateMetadata({
  params,
}: Pick<ShowsDayRouteProps, 'params'>): Promise<Metadata> {
  const { slug, month, day } = await params
  const window = parseDaySegments(slug, month, day)
  if (window === null) {
    return { title: 'Shows not found', robots: { index: false, follow: false } }
  }
  return buildShowsCalendarMetadata(window)
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

  return (
    <div className="w-full max-w-6xl mx-auto px-4 py-8 md:px-8">
      <Suspense fallback={<ShowListSkeleton />}>
        <ShowsCalendarContent window={window} searchParams={searchParams} />
      </Suspense>
    </div>
  )
}
