import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  buildSceneWeekMetadata,
  getSceneWeek,
  SceneWeekContent,
} from '@/features/scenes/sceneWeekPage'
import { looksLikeISOWeek } from '@/features/scenes/sceneWeek'

interface PageProps {
  params: Promise<{ slug: string; 'iso-week': string }>
}

/**
 * `/scenes/{slug}/{iso-week}` — a specific ISO week. The stable permalink, and
 * the canonical URL for both routes.
 *
 * This segment is dynamic, so it also catches any unmatched child path under a
 * scene. The shape check rejects obvious non-week segments locally; anything
 * week-shaped goes to the backend, which owns the calendar maths and is the
 * only thing that can say whether e.g. `2025-W53` exists (it does not — 2025
 * has 52 weeks).
 */
export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { slug, 'iso-week': week } = await params
  if (!looksLikeISOWeek(week)) {
    return { title: 'Week not found', robots: { index: false, follow: false } }
  }
  return buildSceneWeekMetadata(slug, week)
}

export default async function SceneArchivedWeekPage({ params }: PageProps) {
  const { slug, 'iso-week': week } = await params
  // notFound() must be called HERE, in the page component. Calling it from a
  // helper module rendered the not-found body but left the status at HTTP 200.
  if (!looksLikeISOWeek(week)) notFound()
  const data = await getSceneWeek(slug, week)
  if (!data) notFound()
  return <SceneWeekContent data={data} />
}
