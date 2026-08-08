import type { Metadata } from 'next'
import { connection } from 'next/server'

import {
  GraphWeekContent,
  GraphWeekUnbuilt,
  buildGraphWeekMetadata,
  getGraphWeek,
} from '@/features/graph/graphWeekPage'

export async function generateMetadata(): Promise<Metadata> {
  return buildGraphWeekMetadata()
}

/**
 * `/graph/this-week` — the share URL for the weekly growth card.
 *
 * IT ANSWERS 200 EVEN WITH NO SNAPSHOT, and that is a decision reached by
 * measurement rather than the family's usual reflex.
 *
 * The reflex is `notFound()`, which is what the scene-week pages do. It cannot
 * work here. Under `cacheComponents` this route's path is a LITERAL, so Next
 * prerenders a fallback shell for it and flushes that shell with HTTP 200
 * before the dynamic body resolves — after which `notFound()` still renders the
 * not-found body but can no longer set the status. Measured against a
 * production build with the backend answering 503: this route gave 200 with a
 * 404 body while the sibling `/scenes/{slug}/week` — a dynamic segment, so
 * there is no shell to prerender — correctly gave 404. `await connection()`
 * makes the body dynamic but does not move the status.
 *
 * So the choice was between a 404 body under a 200 status, which is the worst
 * of both, and telling the truth. The truth is an EMPTY STATE: unlike a bad
 * scene slug, this URL is permanent and always meaningful — only the data is
 * temporarily absent, and it arrives with the next nightly build. The card
 * already answers this state with the family's branded fallback, so the page
 * and the image now tell one story. `generateMetadata` holds the line that
 * actually matters for a thin page: `noindex, nofollow`, and no `og:image`.
 */
export default async function GraphThisWeekPage() {
  // Keeps the BODY out of the prerender, so what a visitor gets is never a
  // snapshot of whatever the build machine could reach. A route's render mode
  // is otherwise conditional on whether its build-time fetch succeeded, which
  // for an endpoint that 503s until a nightly job has run is not something to
  // leave to whichever build happens to run.
  await connection()

  const view = await getGraphWeek()
  if (!view) return <GraphWeekUnbuilt />
  return <GraphWeekContent view={view} />
}
