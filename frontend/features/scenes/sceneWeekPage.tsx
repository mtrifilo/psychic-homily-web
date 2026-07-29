import { cache } from 'react'
import type { Metadata } from 'next'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateItemListSchema } from '@/lib/seo/jsonld'
import { SceneWeekView } from './components/SceneWeekView'
import { fetchSceneWeek } from './sceneWeekApi'
import { countShows, formatWeekRange, showDisplayTitle, type SceneWeekResponse } from './sceneWeek'

const SITE = 'https://psychichomily.com'

/**
 * Fetch one scene-week for the page.
 *
 * Wrapped in `React.cache` so `generateMetadata` and the page body share one
 * round-trip per request, matching the existing scene-page pattern (PSY-906).
 * The wrapper stays here rather than in `sceneWeekApi` because `React.cache` is
 * server-component-only — the share card, which renders on the edge, calls the
 * underlying fetch directly.
 */
export const getSceneWeek = cache(
  (slug: string, week?: string): Promise<SceneWeekResponse | null> =>
    fetchSceneWeek(slug, week, 'scene-week')
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
