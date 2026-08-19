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
 * `/scenes/{slug}/this-weekend` — Friday, Saturday and Sunday night in the
 * scene's own timezone.
 *
 * A rolling URL, like `/week` and `/tonight`: the one that gets posted on a
 * Thursday, always fresh. Unlike `/week` it has no dated permalink to
 * canonicalise to, so it is its own canonical — see `buildSceneWindowMetadata`.
 *
 * Next.js resolves static segments before dynamic siblings, so this route wins
 * over `[period]` for the literal path. The proxy has to know the segment too:
 * anything under `/scenes/{slug}/` that it does not recognise is hard-404ed
 * before Next ever sees it (see the scenes branch in `proxy.ts`).
 *
 * `await connection()` is load-bearing. The window is bounded against the
 * scene-local clock, and under `cacheComponents` reading a clock is only legal
 * at request time — without it the route would prerender one weekend and serve
 * it forever.
 */
export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  await connection()
  const { slug } = await params
  return buildSceneWindowMetadata(slug, 'this-weekend', getRequestNow())
}

export default async function SceneThisWeekendPage({ params }: PageProps) {
  await connection()
  const { slug } = await params
  // notFound() must be called HERE, in the page component. Calling it from a
  // helper module rendered the not-found body but left the status at HTTP 200.
  const data = await getSceneWindow(slug, 'this-weekend', getRequestNow())
  if (!data) notFound()
  return <SceneWindowContent data={data} />
}
