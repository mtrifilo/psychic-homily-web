import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  buildSceneWeekMetadata,
  getSceneWeek,
  SceneWeekContent,
} from '@/features/scenes/sceneWeekPage'
import {
  buildSceneDayMetadata,
  getSceneDay,
  SceneDayContent,
} from '@/features/scenes/sceneDayPage'
import { looksLikeISOWeek } from '@/features/scenes/sceneWeek'
import { looksLikeCalendarDate } from '@/features/scenes/sceneDay'

interface PageProps {
  params: Promise<{ slug: string; period: string }>
}

/**
 * `/scenes/{slug}/{period}` — one stretch of a scene's calendar, addressed by
 * the key that names it: an ISO week (`2026-W31`) or a single calendar date
 * (`2026-07-31`). Both are stable permalinks. The WEEK key is the canonical URL
 * for both rolling siblings, `/week` and `/tonight`; the date key stands as its
 * own canonical. Why the day key is not announced in any sitemap: see the note
 * on SITEMAP_FAMILIES in app/sitemap-shards.ts.
 *
 * The two share ONE dynamic segment because Next allows only one per level, not
 * because they are one page: a `[iso-week]` route alongside a `[date]` route is
 * a build error. The shape checks below are what separate them, and they cannot
 * both match — a week key carries a `W` where a date carries a month.
 *
 * This segment is dynamic, so it also catches any unmatched child path under a
 * scene. The shape checks reject obvious junk locally; anything week- or
 * date-shaped goes to the backend, which owns the calendar maths and the
 * scene's timezone and is the only thing that can say whether `2025-W53` or
 * `2026-02-30` exists (neither does).
 */
export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { slug, period } = await params
  if (looksLikeISOWeek(period)) return buildSceneWeekMetadata(slug, period)
  if (looksLikeCalendarDate(period)) return buildSceneDayMetadata(slug, period)
  return { title: 'Not found', robots: { index: false, follow: false } }
}

export default async function ScenePeriodPage({ params }: PageProps) {
  const { slug, period } = await params

  // notFound() must be called HERE, in the page component. Calling it from a
  // helper module rendered the not-found body but left the status at HTTP 200.
  if (looksLikeISOWeek(period)) {
    const week = await getSceneWeek(slug, period)
    if (!week) notFound()
    return <SceneWeekContent data={week} />
  }

  if (looksLikeCalendarDate(period)) {
    const day = await getSceneDay(slug, period)
    if (!day) notFound()
    return <SceneDayContent data={day} />
  }

  notFound()
}
