import { cache } from 'react'
import type { Metadata } from 'next'
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateItemListSchema } from '@/lib/seo/jsonld'
import { SceneWeekView } from './components/SceneWeekView'
import { countShows, formatWeekRange, showDisplayTitle, type SceneWeekResponse } from './sceneWeek'

const SITE = 'https://psychichomily.com'

/**
 * Fetch one scene-week.
 *
 * `week` is an ISO week key, or omitted for the scene's CURRENT week. Current
 * is resolved SERVER-side by the backend, in the scene's own timezone — a
 * reader in Berlin and a reader in Chicago must see the same Chicago week, so
 * the client must not compute it.
 *
 * Wrapped in `React.cache` so `generateMetadata` and the page body share one
 * round-trip per request, matching the existing scene-page pattern (PSY-906).
 */
export const getSceneWeek = cache(
  async (slug: string, week?: string): Promise<SceneWeekResponse | null> => {
    const path = week
      ? `${API_BASE_URL}/scenes/${slug}/week/${week}`
      : `${API_BASE_URL}/scenes/${slug}/week`
    try {
      // An archived week is immutable once past; the rolling week is not. The
      // caller picks the revalidate window.
      const res = await fetch(path, { next: { revalidate: week ? 86400 : 900 } })
      if (res.ok) return res.json()
      // 404 is the expected answer for an unknown slug, a below-threshold
      // scene, or a week key that does not exist (e.g. 2025-W53) — not an error
      // worth reporting.
      if (res.status >= 500) {
        Sentry.captureMessage(`Scene week: API returned ${res.status}`, {
          level: 'error',
          tags: { service: 'scene-week' },
          extra: { slug, week, status: res.status },
        })
      }
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'scene-week' },
        extra: { slug, week },
      })
    }
    return null
  }
)

export async function buildSceneWeekMetadata(
  slug: string,
  week?: string
): Promise<Metadata> {
  const data = await getSceneWeek(slug, week)
  if (!data) {
    return { title: 'Week not found', robots: { index: false, follow: false } }
  }

  const total = countShows(data)
  const range = formatWeekRange(data.start_date, data.end_date)
  const title = `${data.scene_name} shows — ${range}`
  const description =
    total > 0
      ? `${total} ${total === 1 ? 'show' : 'shows'} at the ${data.city} rooms we track, ${range}.`
      : `No shows at the ${data.city} rooms we track, ${range}.`

  // The archived week is the canonical URL even when reached via the rolling
  // /week route: the rolling URL's content changes weekly, so pointing search
  // engines at it would make every indexed snippet go stale.
  const canonical = `${SITE}/scenes/${data.slug}/${data.iso_week}`

  return {
    title,
    description,
    alternates: { canonical },
    openGraph: { title, description, url: canonical, type: 'website' },
    twitter: { card: 'summary_large_image', title, description },
  }
}

/**
 * Render a week that has ALREADY been fetched.
 *
 * Deliberately does not fetch or call `notFound()` itself: `notFound()` must be
 * invoked from the page component so Next.js sets a real 404 status. Calling it
 * from a helper in another module rendered the not-found BODY but left the
 * response at HTTP 200 — which search engines and link unfurlers read as a
 * valid page. Verified against the existing scene page, which 404s correctly
 * with the decision in the page body (PSY-906).
 */
export function SceneWeekContent({ data }: { data: SceneWeekResponse }) {
  const shows = (data.days ?? []).flatMap(d => d.shows ?? [])
  const itemList = generateItemListSchema({
    name: `${data.scene_name} shows, ${formatWeekRange(data.start_date, data.end_date)}`,
    listItems: shows.map(s => ({
      url: s.slug ? `${SITE}/shows/${s.slug}` : `${SITE}/shows/${s.id}`,
      name: showDisplayTitle(s),
    })),
  })

  return (
    <>
      {shows.length > 0 && <JsonLd data={itemList} />}
      <SceneWeekView week={data} />
    </>
  )
}
