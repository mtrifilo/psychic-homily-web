import { Suspense, cache } from 'react'
import { offerShowPrice } from '@/lib/utils/showPrice'
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
// From the policy module, not the feature barrel, for the same eviction reason
// the note above states: `showRails.ts` renders nothing.
import { drawsNightRail, venueRailShowsUrl } from '@/features/shows/showRails'
import { showEndpoints } from '@/features/shows/api'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { CURRENT_PERIOD_REVALIDATE } from '@/features/scenes/scenePeriodApi'
import type { ShowAlsoTonightResponse } from '@/features/shows/showRails'
import type { VenueShowsResponse } from '@/features/venues/types'
import type {
  ShowResponse,
  ShowTimelineResponse,
} from '@/features/shows/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicEventSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { formatShowDateLong } from '@/lib/utils/formatters'
import { getShowLifecycleState, hasShowStarted } from '@/lib/utils/showTiming'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntities } from '@/lib/query-hydration'

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
    const res = await fetch(`${API_BASE_URL}/shows/${encodeURIComponent(slug)}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return await res.json()
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

/**
 * The show's gig timeline, fetched here so both corridor modules are in the
 * server HTML rather than shifting the page down when they arrive after
 * hydration. They sit directly under the bill, above the fold.
 *
 * Failure is SILENT and returns null: these are two supplementary lines, and a
 * show page must not 500 because an archive query did. A null seed is skipped
 * by `prefetchEntities`, so the client hook then makes its own attempt.
 *
 * Addressed by SLUG, not by the id on the loaded show, so this can be started
 * before `getShow` has resolved rather than queued behind it. The endpoint takes
 * either spelling. The cache key it seeds is still the numeric id, which is
 * available by the time the seed is built.
 *
 * The slug is `encodeURIComponent`d because Next hands params through
 * DECODED: `%2F` and `%3F` arrive as live path and query delimiters, which
 * would otherwise let a caller re-point this server-side fetch at another
 * backend path and have its body echoed back inside the hydration state.
 *
 * Not `React.cache`d, unlike `getShow`: it has one caller. `generateMetadata`
 * has no use for it, and adding one would mean a second backend round trip on
 * every request for a title that already has everything it needs.
 */
async function getShowTimeline(
  slug: string,
): Promise<ShowTimelineResponse | null> {
  try {
    const res = await fetch(
      `${API_BASE_URL}/shows/${encodeURIComponent(slug)}/timeline`,
      { next: { revalidate: 3600 } },
    )
    // `return await`, not a bare `return`: a bare one hands the caller the
    // parse promise WITHOUT routing its rejection through the catch below, and
    // a 200 with a truncated body would then reject after this function has
    // already returned.
    if (res.ok) {
      return await res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Show timeline: API returned ${res.status}`, {
        level: 'warning',
        tags: { service: 'show-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'warning',
      tags: { service: 'show-page' },
      extra: { slug },
    })
  }
  return null
}

export async function generateMetadata({ params }: ShowPageProps): Promise<Metadata> {
  const { slug } = await params
  const show = await getShow(slug)

  if (show) {
    const headliner = show.artists?.find(a => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
    const venueName = show.venues?.[0]?.name || 'TBA'
    // The shared long-form formatter, not a local `toLocaleDateString`: the
    // description states the same day the page's header, stripe and share card
    // state, including the `~` when that day is read on a guessed zone.
    const showDate = formatShowDateLong(
      show.event_date,
      show.venues?.[0]?.state,
      show.venues?.[0]?.timezone
    )
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
  rails,
}: {
  slug: string
  show: ShowResponse
  rails: Promise<
    [ShowAlsoTonightResponse | null, VenueShowsResponse | null]
  >
}) {
  await connection()
  // The SAME instant the lifecycle is judged against, handed down so the
  // discovery rails order the night by a clock the client cannot disagree with.
  // Read here, after `connection()`, because a prerender may not read the time.
  const renderedAt = new Date()
  const lifecycle = getShowLifecycleState(showTimingInput(show), renderedAt)

  // The rails were STARTED in the page body and are only awaited here, which is
  // the whole point of taking promises rather than URLs: everything above the
  // fold on this page — the bill, the stripe, the venue module — renders out of
  // this component, so a rail read that began here would hold all of it behind
  // a request for a module that sits below the embeds. Started up there, they
  // overlap the show and timeline reads and cost no serial time at all.
  //
  // The night's rail is dropped rather than un-asked on a past show: an archive
  // page must not offer a reader other shows they equally cannot attend, and by
  // the time the lifecycle is known the request is already in flight. It is a
  // per-slug cached read, so the archive cost is one query an hour, not one per
  // view.
  const [alsoTonight, venueShows] = await rails
  const nightRail = drawsNightRail(lifecycle) ? alsoTonight : null

  return (
    <ShowDetail
      showId={slug}
      lifecycle={lifecycle}
      renderedAt={renderedAt.toISOString()}
      initialAlsoTonight={nightRail ?? undefined}
      initialVenueShows={venueShows ?? undefined}
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

  // Started BEFORE the show is awaited, so the two backend reads overlap
  // instead of costing two serial round trips on every Data Cache miss. It
  // resolves to null rather than rejecting, so leaving it in flight through the
  // `notFound()` below cannot raise an unhandled rejection.
  //
  // The accepted cost is on the path that does NOT render: a bogus slug spends
  // two backend requests instead of one, neither cacheable, since the Data
  // Cache drops non-2xx. That is the cheaper side of the trade, because the
  // rendering path is the common one and the 404 path is a fast backend miss.
  const timelinePromise = getShowTimeline(slug)
  const showData = await getShow(slug)

  if (!showData) {
    notFound()
  }

  // Both keys through ONE client: `getQueryClient()` mints a fresh one per
  // call, so two `prefetchEntity` calls would produce two dehydrated states and
  // the boundary below can only carry one.
  //
  // The timeline is keyed on the numeric id while the detail is keyed on the
  // slug the route was addressed by. That is not drift: `useShowTimeline` asks
  // by id because a component holding a ShowResponse has one, and the endpoint
  // accepts either spelling.
  // Both discovery rails, STARTED here and awaited inside
  // `ShowDetailWithLifecycle`, so they overlap the timeline read below instead
  // of adding a round trip in front of the whole page. Their rows go into the
  // served HTML rather than arriving after hydration (PSY-1967): the provenance
  // byline, RevisionHistory and CommentThread all sit under these rails, and
  // `/shows/{slug}#comment-123` scrolls to its comment as soon as the comment
  // query resolves, so rows inserted after that scroll take the targeted
  // comment out from under the reader.
  //
  // Started UNGATED, because the gate needs a clock and this body must not read
  // one — `ShowDetailWithLifecycle` owns that, behind `connection()`. A past
  // show therefore asks for a night rail it will not draw; the read is cached
  // per slug, so that costs one backend query an hour rather than one per view.
  //
  // Through `fetchListPayload`, the same helper every other server-seeded list
  // on the site reads. Two of the things it brings are load-bearing here: a
  // request budget, so a slow backend cannot hold the seed open indefinitely,
  // and a shape guard, because a 200 that lost its `shows` array would
  // otherwise be seeded as data and a rail renders an absent list as "nothing
  // else on tonight" — a confident wrong answer where null is silence.
  //
  // A SHORT cache window, against that helper's hour-long default, which its
  // own doc asks a period-scoped caller to shorten: the night rail's body
  // carries `is_tonight` and every row's sold-out and cancelled state, all
  // computed at request time, and a show mutation revalidates only that show's
  // own page — never the neighbouring pages whose rails list it. 15 minutes is
  // the window the scene pages already hold a live period to.
  //
  // Neither read rejects: `fetchListPayload` degrades to null, and a null seed
  // simply leaves that rail to the client query it already had.
  const railsPromise = Promise.all([
    fetchListPayload<ShowAlsoTonightResponse>({
      // Encoded for the reason `getShowTimeline` states: Next hands params
      // through DECODED, so an unencoded `%2F` would re-point this server-side
      // fetch at another backend path.
      url: showEndpoints.ALSO_TONIGHT(encodeURIComponent(slug)),
      collection: 'shows',
      service: 'show-also-tonight',
      revalidateSeconds: CURRENT_PERIOD_REVALIDATE,
    }),
    // `venues[0].id` is the id the show page itself addresses this venue by.
    // Checked rather than trusted: it arrives from `res.json()` through a type
    // assertion, and it is interpolated into a server-side URL path.
    Number.isSafeInteger(showData.venues?.[0]?.id)
      ? fetchListPayload<VenueShowsResponse>({
          url: venueRailShowsUrl(showData.venues[0].id),
          collection: 'shows',
          service: 'show-venue-rail',
          revalidateSeconds: CURRENT_PERIOD_REVALIDATE,
        })
      : null,
  ])

  const dehydratedState = await prefetchEntities([
    { queryKey: queryKeys.shows.detail(slug), data: showData },
    {
      queryKey: queryKeys.shows.timeline(showData.id),
      data: await timelinePromise,
    },
  ])

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
        // Falls back to the door price when there is no advance price
        // (PSY-1864) — the same "whichever single price we know" rule the
        // ticket line applies. Without it a door-only show emits NO Offer at
        // all, because the builder gates the whole block on a price, so a show
        // with a perfectly well-known $15 door would drop out of search-result
        // pricing entirely. This changes only which VALUE feeds the offer; the
        // posture is untouched (price + seller, still no url).
        price: offerShowPrice(showData),
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
          <ShowDetailWithLifecycle
            slug={slug}
            show={showData}
            rails={railsPromise}
          />
        </Suspense>
      </HydrationBoundary>
    </>
  )
}
