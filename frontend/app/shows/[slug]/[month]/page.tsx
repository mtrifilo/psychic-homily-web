import { Suspense } from 'react'
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { ShowListSkeleton } from '@/features/shows'
import {
  ShowsCalendarContent,
  buildShowsCalendarMetadata,
  type ShowsCalendarSearchParams,
} from '@/features/shows/calendarPage'
import { parseMonthSegments } from '@/features/shows/showsCalendarRoute'

/**
 * `/shows/{yyyy}/{mm}` — the upcoming list scoped to one venue-local month.
 *
 * THE FIRST SEGMENT IS NAMED `[slug]` BECAUSE THE ROUTER REQUIRES IT. Next
 * refuses two different dynamic names at one position, and `app/shows/[slug]`
 * is the show detail route, so a month's year has to travel under that name.
 * It is destructured to `year` immediately and never used as a slug; the same
 * shape `app/charts/[module]` carries for the chart archives.
 *
 * A year segment that is also a real show slug cannot happen: a slug is one
 * segment and these URLs are two, so `/shows/2026` still resolves to the show
 * detail route and `/shows/2026/11` to this one. They do not compete.
 */
interface ShowsMonthRouteProps {
  params: Promise<{ slug: string; month: string }>
  /**
   * Passed straight through to `ShowsCalendarContent` and awaited THERE, never
   * here. Awaiting it in this body would make the whole route dynamic and cost
   * it the prerendered shell. `generateMetadata` does not take it at all —
   * that is what keeps every `?page=` of a month on one canonical.
   */
  searchParams: ShowsCalendarSearchParams
}

export async function generateMetadata({
  params,
}: Pick<ShowsMonthRouteProps, 'params'>): Promise<Metadata> {
  const { slug, month } = await params
  const window = parseMonthSegments(slug, month)
  if (window === null) {
    return { title: 'Shows not found', robots: { index: false, follow: false } }
  }
  return buildShowsCalendarMetadata(window)
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

  return (
    // The list's own skeleton rather than `null`: this body returns as soon as
    // `params` resolves, so this boundary owns the whole wait, and the skeleton
    // is the shape the rows arrive in.
    <div className="w-full max-w-6xl mx-auto px-4 py-8 md:px-8">
      <Suspense fallback={<ShowListSkeleton />}>
        <ShowsCalendarContent window={window} searchParams={searchParams} />
      </Suspense>
    </div>
  )
}
