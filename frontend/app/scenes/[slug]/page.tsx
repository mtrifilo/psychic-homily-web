import { Suspense, cache } from 'react'
import type { Metadata } from 'next'
import dynamic from 'next/dynamic'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import * as Sentry from '@sentry/nextjs'
import { HydrationBoundary } from '@tanstack/react-query'
import type { SceneDetail } from '@/features/scenes'
import { JsonLd } from '@/components/seo/JsonLd'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'
import { fetchSceneWeek } from '@/features/scenes/sceneWeekApi'
import { fetchSceneSlice } from '@/features/scenes/sceneSliceApi'
import { buildSceneWeekJsonLd } from '@/features/scenes/sceneWeekJsonLd'
import { sceneDetailOgImages } from '@/features/scenes/sceneDetailShare'
// Deep-imported from the component FILE for the same reason SceneDetailView is:
// the `@/features/scenes/components` barrel is a `'use client'` barrel, and
// Turbopack does not tree-shake those per-export (PSY-1772). This one is a
// SERVER component, so it is rendered here and handed down as a slot rather
// than imported by SceneDetail.
import { SceneCalendar } from '@/features/scenes/components/SceneCalendar'

// Imported from the component FILE, never a `@/features/scenes` barrel — see
// the note in features/scenes/components/index.ts for why the barrel would undo
// this. `ssr: true` preserves the prefetchEntity + HydrationBoundary server
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
 * Query cache via `prefetchEntity` below so the matching `useSceneDetail`
 * hook resolves from cache instead of refetching on first paint. Returns
 * null for non-2xx (404 expected for bogus slugs) so the page can call
 * `notFound()`.
 */
const getScene = cache(async (slug: string): Promise<SceneDetail | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/scenes/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
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
 * Current week for this scene, cached so `generateMetadata` and the JSON-LD
 * injection share one trip. Fetched through `sceneWeekApi` rather than
 * `sceneWeekPage` so this route does not pull the week view (or `next/og`)
 * into its graph. `undefined` week = the backend's current week, in the
 * scene's own timezone.
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

  return {
    title,
    description,
    alternates: {
      canonical: `https://psychichomily.com/scenes/${slug}`,
    },
    openGraph: {
      title: `${title} | Psychic Homily`,
      description,
      url: `/scenes/${slug}`,
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

  // CONCURRENT, because none of the three needs another's answer. Awaited in
  // sequence they would stack the week fetch in front of the slice's own chain;
  // this leaves that chain as the only thing on the critical path.
  //
  // That chain is THREE calls, not two, and the extra one is not ours: the
  // next-day leg is fetched with a KEY, so `fetchScenePeriod` runs its
  // two-phase freshness probe, and a future date is never `is_past_day`, so the
  // fall-through fires every time. See the follow-up noted on the PR — the fix
  // belongs in that shared caching layer, which `/next-4-weeks` already pays
  // five times over.
  //
  //  - `prefetchEntity`: `cache()` above guarantees the scene fetch already
  //    happened, so this is a no-op cache write seeding the entry
  //    `useSceneDetail` picks up.
  //  - the WEEK feeds the structured data only, and that is now a KNOWN
  //    MISMATCH left deliberately in place. `buildSceneWeekJsonLd` emits an
  //    ItemList plus MusicEvent[] for seven days while the page visibly renders
  //    two, and because the week is Monday-anchored the slice's second day is
  //    outside it every Sunday — so the markup can both over- and under-state
  //    what a reader sees. Structured data is supposed to describe visible
  //    content, so this wants re-scoping to the slice; doing it here would mean
  //    reopening the scene-SEO decisions documented in `sceneDayPage.tsx`
  //    (canonical-to-week, the sitemap families), which this ticket has no
  //    mandate to change on a guess. Raised on the PR for its own ticket.
  //  - the SLICE is what the page actually renders.
  const [dehydratedState, week, slice] = await Promise.all([
    prefetchEntity(queryKeys.scenes.detail(slug), scene),
    getSceneWeek(slug),
    getSceneSlice(slug),
  ])

  const jsonLd = week ? buildSceneWeekJsonLd(week) : null

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
