import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  buildSceneDayMetadata,
  getSceneDay,
  SceneDayContent,
} from '@/features/scenes/sceneDayPage'

interface PageProps {
  params: Promise<{ slug: string }>
}

/**
 * `/scenes/{slug}/tonight` — the scene's current night.
 *
 * The URL people type, and its own canonical (see buildSceneDayMetadata): it is
 * the page that answers "live shows in {city} tonight" every night, so it is
 * the URL that query should rank, and the sitemap announces it with a daily
 * lastmod.
 *
 * "Tonight" is resolved by the BACKEND, in the scene's own timezone and against
 * its 6am night boundary — at 01:00 the answer is still the previous date, so
 * a reader at a show can find the show they are standing at. Nothing about that
 * answer may be computed here: the viewer's clock is the one clock that is
 * never the right one.
 *
 * Next resolves static segments before dynamic siblings, so this route wins
 * over `[period]` for the literal path `/scenes/{slug}/tonight`.
 */
export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { slug } = await params
  return buildSceneDayMetadata(slug)
}

export default async function SceneTonightPage({ params }: PageProps) {
  const { slug } = await params
  // notFound() must be called HERE, in the page component. Calling it from a
  // helper module rendered the not-found body but left the status at HTTP 200.
  //
  // `undefined` passed EXPLICITLY: React.cache keys on `arguments.length`, so
  // a one-argument call here and the two-argument one inside generateMetadata
  // land on different cache entries and the request pays for two fetches.
  const data = await getSceneDay(slug, undefined)
  if (!data) notFound()
  return <SceneDayContent data={data} isRollingRoute />
}
