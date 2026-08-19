import type { Metadata } from 'next'
import { connection } from 'next/server'
import { notFound } from 'next/navigation'
import {
  SceneWindowContent,
  buildSceneWindowMetadata,
  getRequestNow,
  getSceneWindow,
} from '@/features/scenes/sceneWindowPage'

interface PageProps {
  params: Promise<{ slug: string }>
}

/**
 * `/scenes/{slug}/next-4-weeks` — the 28 nights ahead, in the scene's own
 * timezone.
 *
 * ROLLING from tonight, not anchored to the week's Monday. Four Monday-anchored
 * weeks would spend up to six of their days on nights that have already
 * happened, and a window whose label names a forward stretch of time while
 * listing a backward one is wrong in the way that looks authoritative.
 *
 * Composed from consecutive week payloads rather than `GET /scenes/{slug}/shows`
 * — that endpoint bounds at `event_date >= now()` as a UTC instant and a
 * date-only show is stored at UTC midnight, so tonight's date-only shows have
 * already dropped out of it (verified live, PSY-1849). Fixing that bound is its
 * own backend ticket; this page must not be built on it in the meantime.
 *
 * `await connection()` is load-bearing: the window is bounded against the
 * scene-local clock, and under `cacheComponents` reading a clock is only legal
 * at request time.
 */
export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  await connection()
  const { slug } = await params
  return buildSceneWindowMetadata(slug, 'next-4-weeks', getRequestNow())
}

export default async function SceneNext4WeeksPage({ params }: PageProps) {
  await connection()
  const { slug } = await params
  // notFound() must be called HERE, in the page component. Calling it from a
  // helper module rendered the not-found body but left the status at HTTP 200.
  const data = await getSceneWindow(slug, 'next-4-weeks', getRequestNow())
  if (!data) notFound()
  return <SceneWindowContent data={data} />
}
