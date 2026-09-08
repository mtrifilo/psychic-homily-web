import { Suspense, cache } from 'react'
import type { Metadata } from 'next'
import dynamic from 'next/dynamic'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import * as Sentry from '@sentry/nextjs'
import { HydrationBoundary } from '@tanstack/react-query'
import type { SceneDetail } from '@/features/scenes'
import type { SceneCrewsResponse } from '@/features/scenes/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntities } from '@/lib/query-hydration'
import { hasText } from '@/features/scenes/scenePeriodApi'
import { fetchSceneWeek } from '@/features/scenes/sceneWeekApi'
import { fetchSceneSlice } from '@/features/scenes/sceneSliceApi'
import { buildSceneSliceJsonLd } from '@/features/scenes/sceneSliceJsonLd'
import { sceneDetailOgImages } from '@/features/scenes/sceneDetailShare'
// Deep-imported from the component FILE for the same reason SceneDetailView is:
// the `@/features/scenes/components` barrel is a `'use client'` barrel, and
// Turbopack does not tree-shake those per-export (PSY-1772). This one is a
// SERVER component, so it is rendered here and handed down as a slot rather
// than imported by SceneDetail.
import { SceneCalendar } from '@/features/scenes/components/SceneCalendar'

// Imported from the component FILE, never a `@/features/scenes` barrel — see
// the note in features/scenes/components/index.ts for why the barrel would undo
// this. `ssr: true` preserves the prefetchEntities + HydrationBoundary server
// render; the page's own <Suspense> below is the boundary this lazy resolves
// against.
const SceneDetailView = dynamic(
  () =>
    import('@/features/scenes/components/SceneDetail').then(m => ({
      default: m.SceneDetailView,
    })),
  { ssr: true },
)

interface ScenePageProps {
  params: Promise<{ slug: string }>
}

/**
 * The scene fields this route dereferences without a guard.
 *
 * `city` and `state` name the page in its title and its description; `slug`
 * reaches `SceneCalendar`, which builds the window links from it; `stats` is
 * read for a room count (`SceneCalendar`'s quiet-slice copy). Everything else
 * on the payload is already optional-safe.
 *
 * The PAYLOAD's `slug` is checked for text only, not for `looksLikeSlug`: every
 * link built from it goes through `sceneWindowHref`, which encodes. The route
 * PARAM is a different value, and `generateMetadata` encodes it where it builds
 * this page's canonical.
 */
const REQUIRED_SCENE_NAMES = ['city', 'state', 'slug'] as const

/**
 * Accept a 200 body only if it is actually a scene.
 *
 * A 200 is not proof of the right endpoint: a redirect, a CDN error page, or a
 * future API change can all answer 200 with something else, and this route's
 * whole job is to be the existence check the rest of the page trusts. A body
 * missing any field above renders a scene page that is not about a scene.
 *
 * Rejecting reaches the same `notFound()` a 404 does. Reported, naming the
 * offending field, because nothing else can see this: the response was a 200,
 * so no status check fires, and Next stores it for the whole revalidate
 * window. The body itself is not sent.
 */
function asScene(body: unknown, slug: string): SceneDetail | null {
  let rejected: string | undefined
  if (!body || typeof body !== 'object') {
    rejected = 'body'
  } else {
    const record = body as Record<string, unknown>
    rejected =
      REQUIRED_SCENE_NAMES.find(field => !hasText(record[field])) ??
      (typeof record.stats === 'object' && record.stats !== null ? undefined : 'stats')
  }
  if (rejected === undefined) return body as SceneDetail

  Sentry.captureMessage(`Scene page: rejected a payload on \`${rejected}\``, {
    level: 'error',
    tags: { service: 'scene-page' },
    extra: { slug, field: rejected },
  })
  return null
}

/**
 * Scenes are DERIVED from location data (verified venues + the artists/shows
 * at them), not a stored slug entity, so any string could otherwise be
 * title-cased into a real-looking "City, ST Music Scene" page (PSY-906).
 *
 * `GET /scenes/{slug}` is the authoritative existence check: the backend
 * resolves the slug against verified venues and returns 404 for an
 * unparseable slug OR a location below the scene threshold (the same guard
 * every sub-fetch the page renders — active artists, scene graph — already
 * enforces). Fetching it here, server-side, lets the route return a real
 * HTTP 404 (rendering the root `not-found.tsx`) instead of the soft-404 the
 * client `SceneDetailView` would paint at HTTP 200.
 *
 * Wrapped in `React.cache()` so `generateMetadata` and the page body share
 * ONE backend round-trip per request. The result also seeds the TanStack
 * Query cache via `prefetchEntities` below so the matching `useSceneDetail`
 * hook resolves from cache instead of refetching on first paint. Returns
 * null for non-2xx (404 expected for bogus slugs) so the page can call
 * `notFound()`.
 */
const getScene = cache(async (slug: string): Promise<SceneDetail | null> => {
  try {
    // The slug is attacker-controlled: Next decodes route params before this
    // runs, so `phoenix-az?x` or `phoenix-az/../artists` would truncate or walk
    // this path and send the request to a DIFFERENT endpoint, one that can
    // answer 200 with a shape this page then renders as a scene. The period
    // fetches encode for the same reason.
    const res = await fetch(
      `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}`,
      { next: { revalidate: 3600 } }
    )
    if (res.ok) {
      return asScene(await res.json(), slug)
    }
    // Don't report 404s — they're the expected response for invalid /
    // below-threshold slugs (the whole point of this check).
    if (res.status >= 500) {
      Sentry.captureMessage(`Scene page: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'scene-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'scene-page' },
      extra: { slug },
    })
  }
  return null
})

/**
 * Current week for this scene, read by `generateMetadata` alone: the OG card
 * this route advertises is the week card, and its archived permalink carries
 * the week key. Nothing the page BODY renders or describes comes from it.
 *
 * Fetched through `sceneWeekApi` rather than `sceneWeekPage` so this route does
 * not pull the week view (or `next/og`) into its graph. `undefined` week = the
 * backend's current week, in the scene's own timezone. `cache()` bounds this to
 * one trip per request however many callers it grows.
 */
const getSceneWeek = cache((slug: string) =>
  fetchSceneWeek(slug, undefined, 'scene-week')
)

/**
 * The root's calendar slice: tonight and the next full day (PSY-1850).
 *
 * Replaces a 28-day / 61-row CLIENT fetch. Server-side, so the rows arrive as
 * HTML in the first response instead of after hydration, and the reader
 * downloads no calendar JSON at all.
 *
 * The sequencing and the empty-`next_date` trap live in `sceneSliceApi` rather
 * than here, so a second consumer cannot re-derive them; this wrapper only adds
 * the per-request dedupe its two neighbours above already have. `cache()` is
 * currently redundant — the page body is the only caller — and kept for the
 * caller this page will plausibly grow: `generateMetadata` can now state a real
 * tonight count, which the old forward window could not supply (PSY-1807).
 */
const getSceneSlice = cache((slug: string) => fetchSceneSlice(slug))

/**
 * The crew tags booking in this scene, for the header's chip row.
 *
 * Read HERE rather than left to the row's own client query: the row sits
 * inside the header, so a client fetch inserts it under content that has
 * already painted and pushes the calendar down. Seeded, the chips are in the
 * first HTML and nothing moves.
 *
 * Ten minutes, which is the SAME window `useSceneCrews` holds the seeded entry
 * for. The two compose: `prefetchEntities` stamps the seed "fetched now", so a
 * reader's worst case is this window plus that one, and a longer window here
 * would make the hook's promise about staleness untrue.
 *
 * Called with the CANONICAL scene slug, not the requested one. A member-city
 * URL resolves to its metro (`/scenes/tempe-az` renders Phoenix), and the row
 * subscribes under the slug the scene payload carries — seeding the requested
 * spelling would leave the entry orphaned and the row unseeded on exactly
 * those URLs.
 *
 * Returns null on any failure, which `prefetchEntities` SKIPS — the row then
 * falls through to its own client fetch and its own hide-on-error rule rather
 * than hydrating into a permanent empty.
 */
const getSceneCrews = cache(
  async (slug: string): Promise<SceneCrewsResponse | null> => {
    try {
      // Encoded, the same rule `sceneDayApi` and `sceneWeekApi` follow: a slug
      // carrying `?`, `#` or `/` would otherwise splice a query string or an
      // extra segment onto the URL and send this request somewhere else.
      const res = await fetch(
        `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/crews`,
        { next: { revalidate: 600 } }
      )
      // `await` is load-bearing: `return res.json()` adopts the promise after
      // the try block exits, so a malformed body would reject past this catch
      // and 500 the whole scene page instead of dropping one row.
      if (res.ok) {
        return await res.json()
      }
      if (res.status >= 500) {
        Sentry.captureMessage(`Scene crews: API returned ${res.status}`, {
          level: 'error',
          tags: { service: 'scene-page' },
          extra: { slug, status: res.status },
        })
      }
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'scene-page' },
        extra: { slug },
      })
    }
    return null
  }
)

export async function generateMetadata({
  params,
}: ScenePageProps): Promise<Metadata> {
  const { slug } = await params
  const scene = await getScene(slug)

  // Resolve the title from the real scene record rather than title-casing the
  // slug — a nonexistent scene must NOT emit a fabricated "City, ST Music
  // Scene" title (PSY-906). The page body calls `notFound()` for the missing
  // case, so return an explicit not-found title to avoid flashing a generic
  // one before the not-found page mounts.
  if (!scene) {
    return {
      title: 'Scene not found',
      description: 'The music scene you are looking for does not exist.',
    }
  }

  const title = `${scene.city}, ${scene.state} Music Scene`
  // No "scene pulse" clause: that module is gone (PSY-1783 kill set), and a
  // description is both the `<meta name="description">` and the `og:description`,
  // so a stale one advertises a module the page no longer has to a crawler and
  // to anyone who shares the link.
  const generatedDescription = `Upcoming shows, venues and local artists in the ${scene.city}, ${scene.state} music scene.`

  // An authored tagline (PSY-1848) is the scene's own words, so it outranks
  // the generated sentence for `<meta name="description">` / `og:description`
  // — the tagline was specified to double as exactly this. Trimmed-empty
  // counts as absent, matching the page body, so a blank value falls back
  // rather than unfurling as an empty description.
  const description = scene.tagline?.trim() || generatedDescription

  // Both this rolling URL and `/week` advertise the ARCHIVED card. Next would
  // otherwise inject this route's own file-convention image, and that URL is a
  // constant — it carries a hash of the route source, not of the week.
  // Facebook, Discord and Slack cache an unfurled image against its URL for
  // far longer than any header we set, so the rolling URL would keep showing
  // whichever week that scraper happened to see first. The archived URL
  // carries the week, so a new week is a new image.
  //
  // Setting `images` explicitly suppresses the file convention, so the
  // dimensions and alt that convention would have supplied are given here.
  // `twitter.images` is deliberately absent: Next copies the openGraph
  // descriptor across when Twitter has none, so omitting it inherits the alt
  // and dimensions. Setting a bare URL string there would silently drop them.
  //
  // The alt stays the GENERATED sentence even when a tagline exists: alt text
  // describes the card image (this scene's week of shows) for someone who
  // cannot see it, and a four-word authored headline does not do that job.
  const week = await getSceneWeek(slug)
  const ogImages =
    week?.slug && week.iso_week
      ? sceneDetailOgImages(week.slug, week.iso_week, generatedDescription)
      : undefined

  // Encoded, because both of these interpolate the ROUTE PARAM, which Next has
  // already decoded: a `%2F` in the address arrives here as a `/` and would
  // otherwise offer crawlers a two-segment URL as this page's identity.
  const path = `/scenes/${encodeURIComponent(slug)}`

  return {
    title,
    description,
    alternates: {
      canonical: `https://psychichomily.com${path}`,
    },
    openGraph: {
      title: `${title} | Psychic Homily`,
      description,
      url: path,
      type: 'website',
      ...(ogImages ? { images: ogImages } : {}),
    },
    twitter: { card: 'summary_large_image', title, description },
  }
}

function SceneLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ScenePage({ params }: ScenePageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  // Server-side existence check: a slug that doesn't resolve to a qualifying
  // scene must return HTTP 404 so the route renders `not-found.tsx`. Without
  // this, `SceneDetailView` renders a friendly "Scene not found" message at
  // HTTP 200 — a soft-404 that poisons SEO, monitoring, and crawlers.
  const scene = await getScene(slug)
  if (!scene) {
    notFound()
  }

  // CONCURRENT, because neither needs the other's answer. The page waits for
  // both: the slice is a serial chain (`sceneSliceApi` states why it is
  // serial, `sceneSlice` how many calls deep it runs) and the crews row is one
  // request. The slice is listed FIRST so its chain STARTS first: its head
  // request goes out before the crews read, and the route suite pins that.
  //
  // `scene.slug` is the CANONICAL spelling, which is what the crews row keys
  // on; the requested slug can be a member city of the same metro. It carries
  // text or `asScene` would have rejected the payload above.
  const [slice, crews] = await Promise.all([
    getSceneSlice(slug),
    getSceneCrews(scene.slug),
  ])

  // Both seeds are anonymous reads whose payload does not vary by viewer,
  // which is the condition `prefetchEntities` states for stamping them fetched
  // now rather than revalidating on the first commit. The scene seed costs no
  // request: `cache()` guarantees the fetch above already happened, so it only
  // seeds the entry `useSceneDetail` picks up.
  const dehydratedState = await prefetchEntities([
    { queryKey: queryKeys.scenes.detail(slug), data: scene },
    { queryKey: queryKeys.scenes.crews(scene.slug), data: crews },
  ])

  // ONE slice payload feeds both the structured data and the rows
  // `SceneCalendar` draws. The calendar neither caps nor filters those rows, so
  // the two list the same shows.
  const jsonLd = slice ? buildSceneSliceJsonLd(slice) : null

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        {jsonLd && (
          <>
            <JsonLd data={jsonLd.breadcrumb} />
            {jsonLd.itemList && <JsonLd data={jsonLd.itemList} />}
            {jsonLd.events.length > 0 && <JsonLd data={jsonLd.events} />}
          </>
        )}
        <HydrationBoundary state={dehydratedState}>
          <Suspense fallback={<SceneLoadingFallback />}>
            <SceneDetailView
              slug={slug}
              timeZone={slice?.timezone}
              calendarSlot={<SceneCalendar scene={scene} slice={slice} />}
            />
          </Suspense>
        </HydrationBoundary>
      </main>
    </div>
  )
}
