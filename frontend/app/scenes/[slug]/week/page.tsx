import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  buildSceneWeekMetadata,
  getSceneWeek,
  SceneWeekContent,
} from '@/features/scenes/sceneWeekPage'

interface PageProps {
  params: Promise<{ slug: string }>
}

/**
 * `/scenes/{slug}/week` — the scene's CURRENT week.
 *
 * This is the rolling URL: the one that gets posted, always fresh. Its
 * canonical tag points at the archived `{iso-week}` permalink (see
 * buildSceneWeekMetadata), because this URL's content changes every week and
 * indexing it would leave permanently stale snippets.
 *
 * Next.js resolves static segments before dynamic siblings, so this route wins
 * over `[iso-week]` for the literal path `/scenes/{slug}/week`.
 */
export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { slug } = await params
  return buildSceneWeekMetadata(slug)
}

export default async function SceneCurrentWeekPage({ params }: PageProps) {
  const { slug } = await params
  // notFound() must be called HERE, in the page component. Calling it from a
  // helper module rendered the not-found body but left the status at HTTP 200.
  const data = await getSceneWeek(slug)
  if (!data) notFound()
  return <SceneWeekContent data={data} />
}
