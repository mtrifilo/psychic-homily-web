import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import dynamic from 'next/dynamic'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import * as Sentry from '@sentry/nextjs'
import { HydrationBoundary } from '@tanstack/react-query'
import { connection } from 'next/server'
// From '@/features/shows/utils', not the '@/features/shows' barrel: this is the
// route's only VALUE import from the feature, and the barrel re-exports the
// whole client-component surface. Importing it through the barrel would keep
// that surface eagerly reachable from this route and quietly undo the eviction
// below. utils.ts has type-only imports of its own, so it costs nothing.
import { showTimingInput } from '@/features/shows/utils'
import type { ShowResponse } from '@/features/shows/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicEventSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import { getShowLifecycleState, hasShowStarted } from '@/lib/utils/showTiming'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'

// Imported from the component FILE, never `@/features/shows` — see the note in
// features/shows/components/index.ts for why the barrel would undo this.
// `ssr: true` preserves the prefetchEntity + HydrationBoundary server render;
// the page's own <Suspense> below is the boundary this lazy resolves against.
const ShowDetail = dynamic(
  () =>
    import('@/features/shows/components/ShowDetail').then(m => ({
      default: m.ShowDetail,
    })),
  { ssr: true },
)

interface ShowPageProps {
  params: Promise<{ slug: string }>
}

/**
 * Wrapped with `React.cache()` so `generateMetadata` and the page body
 * share ONE backend fetch per request instead of two. The result also
 * seeds the TanStack Query cache via `prefetchEntity` below, eliminating
 * the client-side refetch on first paint.
 */
const getShow = cache(async (slug: string): Promise<ShowResponse | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/shows/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    // Don't report 404s - they're expected for invalid slugs
    if (res.status >= 500) {
      Sentry.captureMessage(`Show page: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'show-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'show-page' },
      extra: { slug },
    })
  }
  return null
})

function formatShowDate(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    timeZone: resolveShowTimezone(state, timezone), // venue-local for SEO (PSY-986)
  })
}

export async function generateMetadata({ params }: ShowPageProps): Promise<Metadata> {
  const { slug } = await params
  const show = await getShow(slug)

  if (show) {
    const headliner = show.artists?.find(a => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
    const venueName = show.venues?.[0]?.name || 'TBA'
    const showDate = formatShowDate(show.event_date, show.venues?.[0]?.state, show.venues?.[0]?.timezone)
    const title = `${headliner} at ${venueName}`
    const generatedDesc = `${headliner} live at ${venueName} on ${showDate}`
    const description = show.description
      ? show.description.slice(0, 155) + (show.description.length > 155 ? '...' : '')
      : generatedDesc

    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/shows/${slug}`,
      },
      openGraph: {
        title,
        description,
        type: 'website',
        url: `/shows/${slug}`,
      },
      // The root layout already sets `twitter.card`, so the card type is not
      // what this fixes. It sets `twitter.images: ['/og-image.jpg']` too, and
      // that shadowed the per-show card: every show unfurled on X with the
      // generic site image. Declaring a route-level `twitter` object replaces
      // the root's wholesale, and because this one omits `images`, Next copies
      // the openGraph descriptor — this route's `opengraph-image` — across
      // instead. Verified by diffing the rendered tags with and without this
      // block. `images` must stay absent; setting a bare URL here would drop
      // the alt text and dimensions that come with the descriptor.
      twitter: {
        card: 'summary_large_image',
        title,
        description,
      },
    }
  }

  return {
    title: 'Show',
    description: 'View show details',
  }
}

/**
 * Reads the clock once, on the server, and hands the answer to the client
 * tree so the status stripe can say TONIGHT without ever consulting the
 * reader's own clock.
 *
 * Its own component because of `await connection()`: under `cacheComponents` a
 * prerender may not read the current time. (The JSON-LD offer gate below reads
 * it too, through `hasShowStarted`'s default `now`; that call sits in the page
 * body, which never executes during the fallback-shell prerender because it
 * awaits `params` first.) Isolating it costs nothing today, since this route has no
 * `generateStaticParams` and its whole body already arrives in the postponed
 * dynamic resume (measured, see the note in `next.config.ts`). It is the shape
 * that stays correct if per-slug prerendering is ever added, and it keeps the
 * one clock read in the page somewhere a reader will find it.
 *
 * `show` is passed down rather than refetched: `getShow` is `React.cache`d, so
 * a second call would be free, but two call sites is two chances for the
 * stripe to be judging a different payload than the page rendered.
 */
async function ShowDetailWithLifecycle({
  slug,
  show,
}: {
  slug: string
  show: ShowResponse
}) {
  await connection()
  return (
    <ShowDetail
      showId={slug}
      lifecycle={getShowLifecycleState(showTimingInput(show))}
    />
  )
}

function ShowLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ShowPage({ params }: ShowPageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  const showData = await getShow(slug)

  if (!showData) {
    notFound()
  }

  const dehydratedState = await prefetchEntity(
    queryKeys.shows.detail(slug),
    showData,
  )

  const headliner = showData.artists?.find(a => a.is_headliner)?.name || showData.artists?.[0]?.name || 'Live Music'
  const showName = showData.title || `${headliner} at ${showData.venues?.[0]?.name || 'TBA'}`

  return (
    <>
      <JsonLd data={generateMusicEventSchema({
        name: showData.title,
        date: showData.event_date,
        description: showData.description ?? undefined,
        is_cancelled: showData.is_cancelled,
        is_sold_out: showData.is_sold_out,
        venue: showData.venues?.[0] ? {
          name: showData.venues[0].name,
          slug: showData.venues[0].slug,
          address: showData.venues[0].address ?? undefined,
          city: showData.venues[0].city,
          state: showData.venues[0].state,
          timezone: showData.venues[0].timezone,
        } : undefined,
        artists: showData.artists?.map(a => ({
          name: a.name,
          slug: a.slug,
          is_headliner: a.is_headliner ?? undefined,
          // `ShowArtistSocials` is a struct of named optional fields; spread into a
          // plain object so it satisfies the schema helper's index-signature parameter
          // type without changing the cross-feature type.
          socials: { ...a.socials },
        })),
        price: showData.price ?? undefined,
        // See the builder for why an offer is dropped once the show has
        // happened. Deliberately NOT derived inside the builder: that would
        // make its output depend on the wall clock.
        //
        // The START INSTANT, not the venue-local day the share card is cached
        // against. This gates an `Offer`, which is a claim that a reader can
        // still buy a ticket, and doors close at a moment: stretching it to
        // local midnight would advertise one for a show already in progress.
        has_started: hasShowStarted(showData.event_date),
        // Names the vendor in `offers.seller`. The builder never emits this
        // URL — no free referrals into structured data.
        ticket_url: showData.ticket_url ?? undefined,
        image_url: showData.image_url,
        slug: showData.slug,
      })} />
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Shows', url: 'https://psychichomily.com/shows' },
        { name: showName, url: `https://psychichomily.com/shows/${slug}` },
      ])} />
      <HydrationBoundary state={dehydratedState}>
        <Suspense fallback={<ShowLoadingFallback />}>
          <ShowDetailWithLifecycle slug={slug} show={showData} />
        </Suspense>
      </HydrationBoundary>
    </>
  )
}
