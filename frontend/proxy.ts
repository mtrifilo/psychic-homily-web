import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { API_BASE_URL } from '@/lib/api-base'
import {
  isNumericShowSegment,
  showSlugRedirectPath,
} from '@/lib/seo/showCanonical'

/**
 * Slug-existence proxy — real HTTP 404 for unknown entity slug pages (PSY-897).
 *
 * Why this file exists
 * --------------------
 * Under Next 16's `cacheComponents: true` (PPR, see `next.config.ts`), entity
 * slug pages that call `notFound()` render the global `app/not-found.tsx` UI
 * but commit **HTTP 200**, not 404. The root cause is the root layout
 * (`app/layout.tsx`): every page's `{children}` is wrapped in a `<Suspense>`
 * around the async, cookie-reading `<AuthHydrator>`. That boundary flushes the
 * static shell — committing the 200 status header — before any page's
 * `await getShow()` → `notFound()` resolves. Next's own docs describe this
 * exactly ("a `200` status code will be returned [...] the status code of the
 * response cannot be updated [after streaming starts]") and prescribe the fix:
 *
 *   "If you need a 404 status [...] ensure the resource exists before the
 *    response body is streamed [...] run this check in `proxy` to rewrite
 *    missing slugs to a not-found route, or produce a 404 response. Keep proxy
 *    checks fast, and avoid fetching full content there."
 *   — Next.js v16 loading.js "Status Codes" docs
 *
 * Proxy runs BEFORE the route renders/streams, so a 404 produced here sets the
 * status header before the streaming trap can commit a 200. This is Option C
 * from the PSY-897 architecture spike (Options A and B were rejected — A is
 * impossible because the trapping Suspense is the root-layout ancestor of every
 * page, and B regresses the PSY-797/PSY-841 PPR/ISR architecture).
 *
 * Scope: Phase 1 proved the approach on SHOWS. Phase 2 extends it to the five
 * uniform-shape entities (venues, artists, releases, labels, festivals) plus
 * `tags`. Phase 3 adds `scenes` (PSY-906) — a derived city/state aggregation,
 * not a stored entity, whose `GET /scenes/<slug>` already 404s for an
 * unresolvable slug or a location below the scene threshold. Adding an entity
 * is a single `ENTITY_CHECKS` entry + a single `config.matcher` source.
 * Collections (auth-gated) and blog/dj-sets (local MDX) remain out of scope and
 * have distinct semantics.
 *
 * How the 404 is produced
 * ------------------------
 * We `rewrite` to a synthetic path that matches NO route. Next's routing layer
 * returns HTTP 404 for an unmatched path and renders `app/not-found.tsx` — the
 * SAME mechanism that already correctly 404s for `/this-route-does-not-exist`
 * (a no-route-match 404 is resolved before any render/stream, so it is NOT
 * subject to the streaming-commits-200 trap that breaks page-level
 * `notFound()`). We also pass `{ status: 404 }` to `rewrite` as belt-and-
 * suspenders for Next versions that honor a rewrite status override. The
 * synthetic target lives OUTSIDE this proxy's matcher (which intercepts only
 * the enumerated entity prefixes), so the rewrite cannot re-trigger the
 * proxy — no loop.
 */

/**
 * Path Next rewrites unknown slugs to. Deliberately matches no route segment
 * AND is not matched by `config.matcher` below (matcher only intercepts
 * `/shows/...`), guaranteeing no rewrite loop. The leading-underscore segment
 * also signals "framework-internal, not a user route".
 */
const NOT_FOUND_REWRITE_PATH = '/_psy-not-found'

/**
 * Shape of an ISO-8601 week segment (`2026-W31`) under `/scenes/<slug>/`.
 *
 * Shape only — it deliberately does not judge whether the week is real. The
 * backend decides that; this just separates "might be a week" from "definitely
 * junk" so the latter 404s without a round-trip.
 */
const ISO_WEEK_SEGMENT = /^\d{4}-W\d{2}$/i

/**
 * Shape of a calendar-date segment (`2026-07-31`) under `/scenes/<slug>/`.
 *
 * Shape only, for the same reason as the week: `2026-02-30` is well-formed and
 * impossible, and the backend — which owns the calendar maths and the scene's
 * timezone — decides that. This just separates "might be a day" from
 * "definitely junk" so the latter 404s without a round-trip.
 */
const CALENDAR_DATE_SEGMENT = /^\d{4}-\d{2}-\d{2}$/

/**
 * The years a scene period may name. MUST stay in lockstep with the feature
 * module's `looksLikeISOWeek` / `looksLikeCalendarDate`, which apply the same
 * bound (`proxy.scenes.test.ts` asserts they agree; the proxy keeps its own
 * copy rather than importing `features/`, matching the charts branch).
 *
 * Not decoration. The page 404s a key outside this window, and a `notFound()`
 * that the proxy waved through commits a 404 BODY at HTTP 200 — the exact
 * soft-404 this whole branch exists to prevent. The backend would happily serve
 * `1998-W12`, so without the bound here that URL renders a not-found page and
 * tells every crawler it succeeded.
 */
const FIRST_TRACKED_YEAR = 2015

function periodYearInRange(segment: string): boolean {
  const year = Number(segment.slice(0, 4))
  return year >= FIRST_TRACKED_YEAR && year <= new Date().getUTCFullYear() + 1
}

/**
 * Whether a date segment names a day that actually exists.
 *
 * `2026-02-30` is well-formed and impossible. Deciding that here — rather than
 * asking the backend — is what lets the dated route use the cheap scene probe:
 * a Gregorian date's validity needs no database, no timezone and no scene, so
 * the round-trip below reaches the same verdict the backend's own parse does,
 * for free. `Date.UTC` normalizes an out-of-range date exactly as Go's parser
 * does, so comparing the components back is the whole check.
 */
function isRealCalendarDate(segment: string): boolean {
  const [year, month, day] = segment.split('-').map(Number)
  const parsed = new Date(Date.UTC(year, month - 1, day))
  return (
    parsed.getUTCFullYear() === year &&
    parsed.getUTCMonth() === month - 1 &&
    parsed.getUTCDate() === day
  )
}

/**
 * Per-entity existence check. The function returns the backend HEAD probe URL
 * whose 404 response means "slug does not exist". The probe uses direct backend
 * existence queries instead of duplicating each page's full `GET /<type>/<slug>`
 * fetch and its response hydration. Adding an entity is one entry here plus one
 * `config.matcher` source.
 *
 * The backend probe centralizes the historical non-uniform cases: tags still
 * resolve by tag ID/slug without loading `/tags/<slug>/detail`, and scenes still
 * resolve derived city/state slugs against the same qualifying venue threshold
 * without loading the full computed scene detail. Collections (auth-gated) and
 * blog/dj-sets (local MDX, no backend existence endpoint) are deliberately
 * absent — they have distinct semantics.
 */
const ENTITY_CHECKS: Record<string, (slug: string) => string> = {
  shows: slug =>
    `${API_BASE_URL}/entities/shows/${encodeURIComponent(slug)}/exists`,
  venues: slug =>
    `${API_BASE_URL}/entities/venues/${encodeURIComponent(slug)}/exists`,
  artists: slug =>
    `${API_BASE_URL}/entities/artists/${encodeURIComponent(slug)}/exists`,
  releases: slug =>
    `${API_BASE_URL}/entities/releases/${encodeURIComponent(slug)}/exists`,
  labels: slug =>
    `${API_BASE_URL}/entities/labels/${encodeURIComponent(slug)}/exists`,
  festivals: slug =>
    `${API_BASE_URL}/entities/festivals/${encodeURIComponent(slug)}/exists`,
  tags: slug =>
    `${API_BASE_URL}/entities/tags/${encodeURIComponent(slug)}/exists`,
  scenes: slug =>
    `${API_BASE_URL}/entities/scenes/${encodeURIComponent(slug)}/exists`,
}

/**
 * Static (non-`[slug]`) routes that live UNDER a proxied entity prefix. These
 * are real Next routes — e.g. `app/shows/submit/page.tsx` (the show-submission
 * form) and `app/shows/saved/page.tsx` (a server redirect to `/library`) — NOT
 * entity slugs. The `config.matcher`'s `/shows/:path*` source intercepts them,
 * and the `/<entity>/<slug>` shape guard below would otherwise existence-check
 * `GET ${API_BASE_URL}/shows/submit` → backend 404 → rewrite the real page to a
 * 404 (PSY-913, regressed by PSY-897's shows phase). Excluding these segments
 * lets the genuine page render.
 *
 * Today only `shows` has static sub-routes; the other six proxied prefixes
 * (venues/artists/releases/labels/festivals/tags) have only `[slug]`. When
 * adding a NEW static route under ANY proxied prefix, add its segment here.
 */
const RESERVED_SEGMENTS: Record<string, ReadonlySet<string>> = {
  shows: new Set(['submit', 'saved']),
}

/**
 * Shape of the date segments under `/shows/`: `/shows/2026/11` and
 * `/shows/2026/11/14`.
 *
 * Fixed width, and EXPORTED so a test can compare this branch's verdict against
 * the route grammar's on every input rather than assert each against a literal.
 * The copy is deliberate: this file must not import `features/`, the same
 * constraint the scenes, charts and venue-year branches work under.
 */
export const SHOWS_CALENDAR_MONTH_SEGMENT = /^(0[1-9]|1[0-2])$/
export const SHOWS_CALENDAR_DAY_SEGMENT = /^(0[1-9]|[12]\d|3[01])$/

/**
 * The years a shows window may name. MUST stay in lockstep with the route
 * grammar's own bound; the test that pins the segment shapes pins these too.
 *
 * Not decoration, and the bound does more than the shape. Four digits alone
 * admits `0026`, whose page emits a canonical the router cannot serve, and
 * `0000`, which the backend reads as no window at all. It is also the crawl
 * bound: the page 404s a month with no shows, and a `notFound()` the proxy
 * waved through commits a 404 BODY at HTTP 200.
 */
export const SHOWS_CALENDAR_MIN_YEAR = 2000
export const SHOWS_CALENDAR_MAX_YEAR = 2100

export function isAddressableShowsYear(segment: string): boolean {
  if (!/^\d{4}$/.test(segment)) return false
  const year = Number(segment)
  return year >= SHOWS_CALENDAR_MIN_YEAR && year <= SHOWS_CALENDAR_MAX_YEAR
}

/**
 * Sub-routes under `/shows/<slug>/` that are NOT date segments.
 *
 * `opengraph-image` is a file-convention route on the show detail page. It
 * reaches the same four-segment shape the month route does, and without this it
 * would be 404ed as a malformed month. The two cannot collide in the router
 * either, a static segment outranks a dynamic one, so this list is what keeps
 * the proxy agreeing with the routing layer.
 */
const SHOWS_SLUG_SUBROUTES: ReadonlySet<string> = new Set(['opengraph-image'])

/**
 * Whether three date segments name a day the calendar actually has.
 *
 * The same rule `isRealCalendarDate` applies to a scene permalink, reached
 * through the one implementation rather than a second copy: a day's validity is
 * Gregorian arithmetic, which needs no database, no timezone and no round trip.
 *
 * Exported alongside the segment shapes so `proxy.shows-calendar.test.ts` can
 * compare this verdict against the route grammar's own on every input, rather
 * than re-deriving it and testing the re-derivation.
 */
export function isRealShowsCalendarDay(
  year: string,
  month: string,
  day: string
): boolean {
  return isRealCalendarDate(`${year}-${month}-${day}`)
}

/**
 * Fixed allowlist for `/charts/[module]` drill-downs plus numeric-year
 * archive first segments (PSY-1422). Unlike entity slug pages there is no
 * backend existence probe — unknown modules are rewritten here so
 * `notFound()` in the page does not soft-404 under cacheComponents.
 * Keep module slugs in lockstep with `CHART_MODULE_SLUGS` in
 * features/charts/moduleConfig (asserted by proxy.charts.test.ts). Inlined
 * here so proxy stays free of features/ imports.
 */
const CHART_MODULE_SEGMENTS = new Set([
  'most-active-artists',
  'on-the-radio',
  'most-anticipated',
  'busiest-venues',
  'new-releases',
  'openers-to-watch',
])

/**
 * Static charts routes that are NOT module drill-downs — pass through so the
 * App Router can resolve `app/charts/<seg>/page.tsx` (PSY-1411 link target /
 * PSY-1501 archive page). Without this, `/charts/featured` is rewritten to a
 * hard 404 before Next sees the static segment.
 */
const CHART_STATIC_SEGMENTS = new Set(['featured'])

function isChartArchiveYearSegment(segment: string): boolean {
  return /^[0-9]{4}$/.test(segment)
}

function isChartArchiveQuarterSegment(segment: string): boolean {
  return /^q[1-4]$/.test(segment)
}

/**
 * The static segment between a venue slug and its year archive:
 * `/venues/<slug>/shows/<year>` (PSY-1756).
 */
const VENUE_ARCHIVE_SEGMENT = 'shows'

/** Shape of the year segment. Shape only — membership is a data question. */
const VENUE_ARCHIVE_YEAR_SEGMENT = /^\d{4}$/

/**
 * The year window the archive route accepts. MUST stay in lockstep with
 * `ARCHIVE_YEAR_RANGE` in features/venues/showArchive; the proxy keeps its own
 * copy rather than importing `features/`, matching the scenes and charts
 * branches, and EXPORTS it so proxy.venue-years.test.ts can assert the two
 * copies are EQUAL rather than assert each against a literal.
 *
 * Not decoration, for exactly the reason FIRST_TRACKED_YEAR is not: the page
 * 404s a year outside this window, and a `notFound()` the proxy waved through
 * commits a 404 BODY at HTTP 200 — the soft-404 this whole file exists to
 * prevent. The upper bound is enforced by VENUE_ARCHIVE_YEAR_SEGMENT (four
 * digits cannot exceed 9999) rather than by a comparison that could never fire;
 * it is named here so the lockstep assertion has both ends to compare.
 */
export const VENUE_ARCHIVE_MIN_YEAR = 1900
export const VENUE_ARCHIVE_MAX_YEAR = 9999

/**
 * Returns the global 404 rewrite response (status 404 + render
 * `app/not-found.tsx` via the unmatched synthetic path).
 */
function notFoundResponse(request: NextRequest): NextResponse {
  return NextResponse.rewrite(new URL(NOT_FOUND_REWRITE_PATH, request.url), {
    status: 404,
  })
}

export async function proxy(request: NextRequest): Promise<NextResponse> {
  const { pathname } = request.nextUrl

  // pathname is `/<entity>/<slug>` (matcher guarantees one of the enumerated
  // entity prefixes). Split into ["", "<entity>", "<slug>", ...optional
  // sub-segments].
  const segments = pathname.split('/')
  const entityType = segments[1]
  const slug = segments[2]

  // Charts: module drill-downs + calendar archives (`/charts/2026`,
  // `/charts/2026/q2`) + static editorial routes (`/charts/featured`). Bare
  // `/charts` passes through; unknown shapes get a real 404 (page `notFound()`
  // alone soft-404s under cacheComponents).
  if (entityType === 'charts') {
    if (!slug) {
      return NextResponse.next()
    }
    if (segments.length === 3) {
      if (
        CHART_MODULE_SEGMENTS.has(slug) ||
        CHART_STATIC_SEGMENTS.has(slug) ||
        isChartArchiveYearSegment(slug)
      ) {
        return NextResponse.next()
      }
      return notFoundResponse(request)
    }
    if (
      segments.length === 4 &&
      isChartArchiveYearSegment(slug) &&
      isChartArchiveQuarterSegment(segments[3])
    ) {
      return NextResponse.next()
    }
    return notFoundResponse(request)
  }

  // Shows: the month and day list routes sit one level BELOW the show detail
  // shape: `/shows/2026/11` and `/shows/2026/11/14`. The generic
  // check further down only handles the 3-segment detail shape, so without this
  // branch a malformed date streams a 200 shell before the route's own
  // `notFound()` resolves, and every junk segment under `/shows/` becomes a
  // soft-404 (the PSY-897 arc; the scene period routes hit the same trap).
  //
  // SHAPE FIRST, which is the half a crawler can walk for free and the larger
  // one: every four-digit year crossed with every two-character segment. It
  // needs no backend at all, so it is settled before anything is asked of one.
  //
  // MEMBERSHIP is then asked of the addressable SPAN rather than of the window
  // itself. A month inside the span with no shows is a quiet page at 200 and a
  // month outside it is a 404, which is why an empty window is never enough to
  // decide: the Tonight and This weekend chips resolve to today's own dates, and
  // a 404 there dead-ends the row that sent the reader.
  if (
    entityType === 'shows' &&
    slug &&
    (segments.length === 4 || segments.length === 5)
  ) {
    if (segments.length === 4 && SHOWS_SLUG_SUBROUTES.has(segments[3])) {
      return NextResponse.next()
    }
    if (
      !isAddressableShowsYear(slug) ||
      !SHOWS_CALENDAR_MONTH_SEGMENT.test(segments[3])
    ) {
      return notFoundResponse(request)
    }
    if (segments.length === 5) {
      const day = segments[4]
      if (
        !SHOWS_CALENDAR_DAY_SEGMENT.test(day) ||
        !isRealShowsCalendarDay(slug, segments[3], day)
      ) {
        return notFoundResponse(request)
      }
    }
    return showsCalendarWindowCheck(request, Number(slug), Number(segments[3]))
  }

  // Scenes: the weekly and nightly city pages sit one level BELOW the scene
  // detail — `/scenes/<slug>/week` and `/scenes/<slug>/tonight` (rolling), plus
  // `/scenes/<slug>/2026-W31` and `/scenes/<slug>/2026-07-31` (permalinks). The
  // generic check below only handles the 3-segment detail shape, so without
  // this these stream a 200 shell before `notFound()` resolves and every bad
  // key becomes a soft-404 (PSY-897 arc).
  //
  // The backend is the authority on whether a period EXISTS: `2025-W53` is
  // well-formed but unreal (2025 has 52 weeks) and `2026-02-30` is well-formed
  // and impossible, and only the backend owns that calendar maths plus the
  // scene's timezone. Re-deriving it here would drift.
  //
  // Every backend path below is registered with `huma.Head` as well as
  // `huma.Get`. Without the HEAD registration the router answers 405, this
  // check fails OPEN, and a nonexistent key soft-404s all over again.
  if (entityType === 'scenes' && segments.length === 4 && slug) {
    const sub = segments[3]
    const scene = encodeURIComponent(slug)
    if (sub === 'week') {
      return existenceCheck(request, `${API_BASE_URL}/scenes/${scene}/week`, {
        // Shipped route; see failedProbeResponse's note on the `false`s.
        requireApiAuthoredNotFound: false,
      })
    }
    // Both DAY routes probe the cheap scene-existence endpoint rather than the
    // day endpoint itself, because everything else that could make them 404 is
    // decided right here for free: `/tonight` has no key at all, and a dated
    // key's validity is pure Gregorian arithmetic needing no database, no
    // timezone and no scene. Probing `/day` would rebuild the entire night —
    // venue count, timezone, the shows join, tracked venues, and on a quiet
    // night a six-week look-ahead — and discard the body, on EVERY request,
    // uncached, in addition to the page's own fetch.
    //
    // Two caveats this buys, both accepted deliberately:
    //
    // 1. `sceneExists` shares the >= 2-verified-venues threshold with the day
    //    endpoint but reaches it by its own slug resolution. That is the same
    //    bargain every `/scenes/{slug}` page already makes through
    //    ENTITY_CHECKS, so it is the established shape here rather than a new
    //    risk — but if the two resolvers ever diverge, these routes soft-404.
    //    `not-found.spec.ts` asserts the rendered page is the SCENE, not merely
    //    that something rendered, which is what would catch it.
    //
    // 2. DEPLOY THE BACKEND FIRST. Because the probe no longer touches the day
    //    endpoint, a frontend that goes live ahead of its backend would pass
    //    these requests through to a page whose own fetch 404s, and that
    //    `notFound()` arrives after the shell has streamed — a 404 body at
    //    HTTP 200 for the length of the skew window. The `/week` routes are
    //    immune only because they probe their own endpoint. This is transient
    //    and self-healing where the cost it replaces — rebuilding the whole
    //    night, uncached, on every request over thousands of dated keys per
    //    scene — was permanent and reachable by anyone.
    if (sub === 'tonight') {
      return existenceCheck(request, ENTITY_CHECKS.scenes(slug), {
        requireApiAuthoredNotFound: false,
      })
    }
    // The multi-day rolling windows (PSY-1849). Same reasoning as `/tonight`:
    // neither carries a key, so the ONLY thing that can 404 them is the scene
    // itself, and the cheap existence endpoint answers that without rebuilding
    // a window the page is about to fetch anyway. `/next-4-weeks` composes five
    // week payloads, so probing its own data here would cost five extra backend
    // queries per request and discard all of them.
    //
    // Caveat 2 on `/tonight` applies to these as well: DEPLOY THE BACKEND
    // FIRST. Because the probe does not touch the window's own data, a frontend
    // live ahead of its backend passes these through to a page whose fetch
    // 404s, and that `notFound()` lands after the shell has streamed.
    if (sub === 'this-weekend' || sub === 'next-4-weeks') {
      return existenceCheck(request, ENTITY_CHECKS.scenes(slug), {
        requireApiAuthoredNotFound: false,
      })
    }
    // File-convention OG card on the rolling detail URL. Without this branch
    // `opengraph-image` is a junk period and 404s before Next sees the route
    // (PSY-1785). Probe scene existence, same as `/tonight`: the card's own
    // fetch then draws the current week. The PAGE does not advertise this URL
    // (unfurl caches key on it forever); it stays addressable on its own.
    if (sub === 'opengraph-image') {
      return existenceCheck(request, ENTITY_CHECKS.scenes(slug), {
        requireApiAuthoredNotFound: false,
      })
    }
    if (CALENDAR_DATE_SEGMENT.test(sub)) {
      if (!periodYearInRange(sub) || !isRealCalendarDate(sub)) {
        return notFoundResponse(request)
      }
      return existenceCheck(request, ENTITY_CHECKS.scenes(slug), {
        requireApiAuthoredNotFound: false,
      })
    }
    // The WEEK key still goes to its own endpoint: `2025-W53` is well-formed
    // and unreal, and unlike a calendar date that verdict is ISO-8601 week
    // arithmetic the backend owns and this file deliberately does not copy.
    if (ISO_WEEK_SEGMENT.test(sub) && periodYearInRange(sub)) {
      return existenceCheck(
        request,
        `${API_BASE_URL}/scenes/${scene}/week/${encodeURIComponent(sub)}`,
        { requireApiAuthoredNotFound: false }
      )
    }
    // Not a servable period (`/scenes/chicago-il/garbage`, `/scenes/chicago-il/
    // 1998-W12`): no route can serve it, so 404 without spending a backend
    // round-trip. The out-of-range case matters as much as the junk one — the
    // backend WOULD serve 1998-W12, and waving it through means the page's
    // own `notFound()` commits a 404 body at HTTP 200.
    return notFoundResponse(request)
  }

  // The retired `?year=` form (PSY-1756). PSY-1753 shipped it days earlier as a
  // deliberately shareable address, so those links exist in the wild — and with
  // the read removed they would otherwise resolve silently to the UNFILTERED
  // archive, showing a reader every year while their URL says one.
  //
  // Handled here rather than in next.config's `redirects()` because that layer
  // carries unmatched query params through to the destination, landing the
  // reader on `/shows/2025?year=2025` — a second address for a page whose whole
  // premise is having exactly one. Built from `venueArchiveHref`'s shape, and
  // only for a year the archive would accept: anything else falls through and
  // the venue page renders as it does today, ignoring the param.
  if (
    entityType === 'venues' &&
    segments.length === 3 &&
    slug &&
    !RESERVED_SEGMENTS[entityType]?.has(slug)
  ) {
    const legacyYear = request.nextUrl.searchParams.get('year')
    if (
      legacyYear &&
      VENUE_ARCHIVE_YEAR_SEGMENT.test(legacyYear) &&
      Number(legacyYear) >= VENUE_ARCHIVE_MIN_YEAR
    ) {
      const target = new URL(
        `/venues/${slug}/${VENUE_ARCHIVE_SEGMENT}/${legacyYear}`,
        request.url
      )
      // The page number is a different axis and survives the move unchanged.
      const page = request.nextUrl.searchParams.get('page')
      if (page) target.searchParams.set('page', page)
      return NextResponse.redirect(target, 308)
    }
  }

  // Venue year archives: `/venues/<slug>/shows/<year>` (PSY-1756). One level
  // BELOW the entity-detail shape the generic check handles, so like the scene
  // periods it needs its own branch or every bad year soft-404s.
  //
  // The URL space here is unbounded by construction — 8,100 in-range years for
  // every venue in the catalogue — and only a handful of them are documents. So
  // this branch is what stands between a crawler and an arbitrarily large field
  // of 200-with-a-not-found-body.
  //
  // Membership is asked of the year's own existence endpoint (PSY-1770). It is
  // built on the SAME predicate the page renders from and the `venue_years`
  // sitemap family is projected from — `venueShowsBaseQuery(venue, "past",
  // year)` — so the three still cannot drift: a year in the sitemap resolves,
  // and a year that resolves is in the strip.
  if (
    entityType === 'venues' &&
    segments.length === 5 &&
    slug &&
    segments[3] === VENUE_ARCHIVE_SEGMENT
  ) {
    const year = segments[4]
    // Shape and range are settled HERE, with no round trip: a segment that is
    // not four in-range digits can never be an archive, whatever the database
    // says, and this is the half of the space a crawler can walk for free.
    if (
      !VENUE_ARCHIVE_YEAR_SEGMENT.test(year) ||
      Number(year) < VENUE_ARCHIVE_MIN_YEAR
    ) {
      return notFoundResponse(request)
    }
    return venueArchiveYearCheck(request, slug, Number(year))
  }

  const buildCheckUrl = ENTITY_CHECKS[entityType]

  // Not a mapped entity, or a sub-route like `/<entity>/<slug>/edit`, or the
  // bare `/<entity>` list (no slug): leave untouched. Only the exact
  // `/<entity>/<slug>` detail shape is existence-checked.
  if (!buildCheckUrl || !slug || segments.length !== 3) {
    return NextResponse.next()
  }

  // `/<entity>/<segment>` where <segment> is a real static route (e.g.
  // `/shows/submit`, `/shows/saved`), not an entity slug — never existence-check
  // it, or we'd 404 a genuine page (PSY-913).
  if (RESERVED_SEGMENTS[entityType]?.has(slug)) {
    return NextResponse.next()
  }

  // A year-shaped id is left to the existence probe: `/shows/{year}` is the
  // parent shape of the calendar routes, and a cached permanent redirect there
  // would bind that address to one show.
  if (
    entityType === 'shows' &&
    isNumericShowSegment(slug) &&
    !isAddressableShowsYear(slug)
  ) {
    return numericShowRedirect(request, slug)
  }

  return existenceCheck(request, buildCheckUrl(slug), {
    requireApiAuthoredNotFound: false,
  })
}

/**
 * How long an existence probe may take before the proxy gives up and lets the
 * page render. Generous relative to the endpoints it calls (all indexed
 * lookups) and far below any platform function timeout, so the budget that
 * binds is this one rather than the invocation's.
 */
const EXISTENCE_CHECK_TIMEOUT_MS = 2_500

/**
 * The content type the API stamps on every error it MEANS.
 *
 * It is how a 404 that answers the question ("no such venue", "no shows that
 * year") is told apart from a 404 that means the server never heard of the
 * path — huma writes `application/problem+json` on its errors, while chi's
 * default handler for an unregistered route writes `text/plain`. Measured on
 * this build against the real router:
 *
 *   /venues/{slug}/shows/2020/exists      404 application/problem+json
 *   /entities/venues/{slug}/exists        404 application/problem+json
 *   /venues/{slug}/shows/2020/no-such     404 text/plain; charset=utf-8
 *
 * Load-bearing at DEPLOY time, which is the only time it fires. A frontend that
 * goes live ahead of its backend probes paths the running API does not carry
 * yet; without this the bare 404 reads as "missing" and the proxy hard-404s
 * every URL behind that probe — for the venue year archives, a whole
 * sitemap-announced family, for the length of the skew window. With it, an
 * unrouted probe falls through to the same fail-OPEN path as a 5xx: the page
 * renders, and the worst case is the soft-404 the scene branches already
 * tolerate, which is transient and recoverable where a hard 404 is not.
 */
const API_ERROR_CONTENT_TYPE = 'application/problem+json'

/**
 * The ONE fail-open policy, for a probe that did not answer 2xx: the response
 * the proxy sends instead, or null when the probe succeeded and the caller
 * decides.
 *
 * Shared by `existenceCheck` and `numericShowRedirect`, the two callers that
 * probe one address, so the rules below have one copy.
 *
 * The fail-open rules are the load-bearing part: a backend 404 is the only
 * "missing", and anything else (5xx, 403, 429, opaqueredirect) lets the page
 * render. Producing a 404 on a transient blip would mask a real outage as
 * "not found".
 *
 * `requireApiAuthoredNotFound` narrows what counts as that 404 — see
 * API_ERROR_CONTENT_TYPE — and is REQUIRED rather than optional on purpose.
 * Every caller has to state which reading it wants, because the two fail in
 * opposite directions during a deploy skew and the safe answer depends on how
 * old the route is. A default would be a trap: the next probe added to this file
 * would be written by copying a neighbour, and would silently inherit whichever
 * reading that neighbour happened to need.
 *
 * `true` is the answer for a NEW route, and the answer to reach for when unsure.
 * The callers that pass `false` probe shipped routes, kept as they are because
 * turning the guard on for them changes 404 semantics site-wide — worth doing,
 * but as its own change with its own verification across every entity type, not
 * as a side effect of a performance ticket.
 */
function failedProbeResponse(
  request: NextRequest,
  res: Response,
  options: { requireApiAuthoredNotFound: boolean }
): NextResponse | null {
  // 404 from the backend = the thing genuinely does not exist → real 404.
  //
  // Unless the caller asked for the stricter reading, in which case the
  // content type has to agree that the API AUTHORED this 404 — "not found"
  // and "I have never heard of this path" arrive as the same status and must
  // not mean the same thing. See API_ERROR_CONTENT_TYPE.
  if (res.status === 404) {
    if (!options.requireApiAuthoredNotFound) {
      return notFoundResponse(request)
    }
    const contentType = res.headers.get('content-type') ?? ''
    if (contentType.includes(API_ERROR_CONTENT_TYPE)) {
      return notFoundResponse(request)
    }
    return NextResponse.next()
  }

  // Any other non-ok (5xx, 403, 429, opaqueredirect, …): fail OPEN — let the
  // page render and apply its own handling (each page's server fetch reports
  // 5xx to Sentry, renders its own not-found on null, etc.).
  if (!res.ok) {
    return NextResponse.next()
  }

  return null
}

/**
 * Probe a backend URL and turn the result into a pass-through or a real 404,
 * under `failedProbeResponse`'s policy. A network error fails open too.
 *
 * HEAD, and the STATUS is the whole answer. PSY-1756 briefly widened this with a
 * `verdict` callback, for the one probe that had to read a body because its
 * endpoint answered 200 for any venue that existed; PSY-1770 gave that probe a
 * status-bearing endpoint of its own and the callback went with it. A new caller
 * that finds itself wanting a body should get its endpoint an honest status
 * instead — the backend can answer the question in one indexed row, and a body
 * this function parses is a body every OTHER caller pays to receive.
 *
 * Two callers read a body anyway, each for a reason a status cannot carry.
 * `readShowsCalendarRange` asks about the URL SPACE rather than about a URL: one
 * span bounds every dated shows window, so it is read once per instance and
 * compared in memory, where a status-bearing probe would be a backend call per
 * dated URL a crawler walks. `numericShowRedirect` needs the show's slug, a
 * value rather than a yes or no, and no status-bearing endpoint returns it.
 * Any other probe that answers about ONE address belongs in this function.
 */
async function existenceCheck(
  request: NextRequest,
  url: string,
  options: { requireApiAuthoredNotFound: boolean }
): Promise<NextResponse> {
  try {
    const res = await fetch(url, {
      method: 'HEAD',
      // `next: { revalidate }` has NO effect inside proxy (per Next docs), so
      // we don't set it. `redirect: 'manual'` keeps the check cheap and avoids
      // following any backend redirect chain. A 2xx means the slug resolves;
      // only a backend 404 is treated as "missing".
      redirect: 'manual',
      // Without this a slow-but-alive backend pins one middleware invocation
      // per request until the platform's own timeout — across EVERY proxied
      // entity prefix, not just the one being probed. An abort lands in the
      // catch below, which is the fail-open path, so the page still renders.
      signal: AbortSignal.timeout(EXISTENCE_CHECK_TIMEOUT_MS),
    })

    // Backend reachable and the slug resolves when nothing failed.
    return failedProbeResponse(request, res, options) ?? NextResponse.next()
  } catch {
    // Network error reaching the backend: fail OPEN. The proxy must never take
    // a route down when the check itself fails.
    return NextResponse.next()
  }
}

/**
 * How long a browser may reuse a numeric-id redirect before asking again.
 *
 * Bounded because a show's slug is not permanent: the backend can rewrite one
 * in place, and a 308 sent without a lifetime is cached by browsers
 * indefinitely, which would pin the id to a slug that no longer resolves. The
 * status stays permanent, which is what tells a crawler the slug is canonical.
 */
const NUMERIC_SHOW_REDIRECT_CACHE_CONTROL = 'private, max-age=3600'

/**
 * A show addressed by its numeric id, permanently redirected to its slug URL,
 * so one show is never indexed at two addresses.
 *
 * Here and not in the page, because only the proxy can set the status: the page
 * body resolves after the root layout's Suspense boundary has committed a 200,
 * so a `permanentRedirect()` there arrives as a client-side meta refresh.
 *
 * A GET of the show rather than the HEAD existence probe, because the answer
 * needed is the slug and only the detail read carries it. That is the full
 * content read the Next guidance quoted at the top of this file advises
 * against, uncached, so it runs only for a numeric-id request. It stands in for
 * the probe on that path, so the request still costs one backend call here.
 *
 * `failedProbeResponse` decides every non-2xx answer, with the same 404 reading
 * the shows existence probe uses, and a network error fails open. A show with
 * no addressable slug renders at the numeric URL. Query parameters survive the
 * redirect.
 */
async function numericShowRedirect(
  request: NextRequest,
  id: string
): Promise<NextResponse> {
  try {
    const res = await fetch(`${API_BASE_URL}/shows/${encodeURIComponent(id)}`, {
      redirect: 'manual',
      signal: AbortSignal.timeout(EXISTENCE_CHECK_TIMEOUT_MS),
    })
    const failed = failedProbeResponse(request, res, {
      // `GET /shows/{id}` is a shipped route, read the way the shows existence
      // probe it stands in for is read.
      requireApiAuthoredNotFound: false,
    })
    if (failed) {
      // A GET carries an error body; releasing it frees the connection now.
      await res.body?.cancel().catch(() => {})
      return failed
    }
    const body: unknown = await res.json()
    const loadedSlug =
      typeof body === 'object' && body !== null
        ? (body as { slug?: unknown }).slug
        : undefined
    const target = showSlugRedirectPath(
      id,
      typeof loadedSlug === 'string' ? loadedSlug : null
    )
    if (!target) {
      return NextResponse.next()
    }
    const url = new URL(target, request.url)
    url.search = request.nextUrl.search
    const redirect = NextResponse.redirect(url, 308)
    redirect.headers.set('Cache-Control', NUMERIC_SHOW_REDIRECT_CACHE_CONTROL)
    return redirect
  } catch {
    return NextResponse.next()
  }
}

/**
 * Whether a venue actually has past shows in `year`.
 *
 * A status-bearing HEAD probe, like every other branch in this file (PSY-1770).
 * The backend answers 404 for a venue that does not exist AND for a year with no
 * archived shows, which is the whole question — so nothing here reads a body,
 * and the fail-open rules in `existenceCheck` apply unchanged.
 *
 * It replaces a GET of the venue-shows LIST scoped to the year, which had to
 * read `total` out of the body because that endpoint answers 200 for any venue
 * that exists. That form was already scoped rather than aggregating the venue's
 * whole history — the shape the ticket describes was fixed during PSY-1756 — but
 * its handler still ran a COUNT, plucked a page of ids, and hydrated the row
 * with its bills and artists, then shipped a JSON body back over a link that
 * exists to carry one bit. The backend's probe is
 * `venueShowsBaseQuery(venue, "past", year)` with LIMIT 1 — the same builder the
 * page's own rows come from, and composing the same venue-local year fragments
 * the `venue_years` sitemap family does. That the three AGREE is enforced by
 * tests rather than by construction; see the note on HasPastShowsInYear for
 * which ones and what breaks them. It is cheaper, not free — see the cost note
 * on the handler, which is honest about the slug resolution it still pays for.
 *
 * THE FRONTEND WILL GO LIVE FIRST, and the guard is what makes that survivable
 * rather than an instruction to avoid it. A release here is ONE fast-forward
 * push of `production` that Railway and Vercel react to in parallel; there is no
 * backend step to sequence ahead of a frontend step, and Next finishes building
 * well before Go builds, migrates and passes a healthcheck. So on any release
 * that adds a probed endpoint, this branch WILL spend a window calling a route
 * the running API does not carry.
 *
 * That window is survivable because chi answers an unknown path with a
 * `text/plain` 404 while the API stamps `application/problem+json` on the ones
 * it authors — see `requireApiAuthoredNotFound`. The skew therefore costs
 * soft-404s on empty years, which are transient and recoverable, instead of hard
 * 404s across a sitemap-announced family, which are neither.
 *
 * Do NOT replace this with "deploy the backend first". That sentence has been
 * written twice already in this file (see the scene `/tonight` branch) and names
 * a step the release process does not have.
 */
function venueArchiveYearCheck(
  request: NextRequest,
  slug: string,
  year: number
): Promise<NextResponse> {
  return existenceCheck(
    request,
    `${API_BASE_URL}/venues/${encodeURIComponent(slug)}/shows/${year}/exists`,
    { requireApiAuthoredNotFound: true }
  )
}

/**
 * The addressable month span, reduced to the only thing this file asks of it:
 * two comparable months.
 */
interface ShowsCalendarRange {
  first: number
  last: number
}

/** The span endpoint. Constant: it takes no parameters and no viewer. */
const SHOWS_CALENDAR_RANGE_URL = `${API_BASE_URL}/shows/calendar/range`

/**
 * How long a span is reused.
 *
 * `next: { revalidate }` has no effect inside proxy, so the cache is this
 * module's own and lives for the life of the instance. That is what keeps the
 * probe off the per-request path: one call per instance per window, against a
 * backend whose anonymous budget is shared by every reader behind one address,
 * rather than one per dated URL a crawler walks.
 *
 * The endpoint publishes the same number as its `max-age`, and nothing here
 * reads that header: the two are set to agree by hand, and a change to either
 * one is a change to how long a month approved beyond the last edge keeps
 * 404ing.
 */
const SHOWS_CALENDAR_RANGE_TTL_MS = 300_000

/**
 * How long an UNANSWERED probe is remembered.
 *
 * Shorter than a good answer, because the page is rendering unbounded windows
 * until it clears, and longer than zero, because a backend that is down or
 * rate-limiting must not be asked again by every request that arrives while it
 * is. Both directions are failure handling, not caching.
 */
const SHOWS_CALENDAR_RANGE_UNKNOWN_TTL_MS = 30_000

/**
 * The cached span, held as the PROMISE rather than the resolved value.
 *
 * One entry is both the cache and the single-flight guard: a request arriving
 * while the probe is open finds the entry and awaits the same promise, so a
 * cold instance taking a burst of dated URLs sends one probe rather than one
 * per request. The entry is stamped with the unknown window first and restamped
 * when the probe settles, which is safe because `fetchShowsCalendarRange`
 * resolves for every outcome and rejects for none.
 */
let showsCalendarRangeCache: {
  range: Promise<ShowsCalendarRange | null>
  expiresAt: number
} | null = null

/** Two months on one axis, so December cannot compare as later than January. */
function showsCalendarMonthOrdinal(year: number, month: number): number {
  return year * 12 + month
}

/**
 * The span a response body names, or `null` when the body is not one.
 *
 * Every field is checked before it is compared. A body that has drifted is an
 * unanswered probe rather than a span of `NaN`, which would compare false
 * against everything and 404 the whole family.
 */
/**
 * One edge of the span as a comparable month, or `null` when it is not one.
 *
 * The YEAR is bounded by the same window the route grammar accepts, not merely
 * checked for being a number. An edge of year 0 is well-typed, orders correctly
 * against its sibling, and would place the whole span before every addressable
 * month: the proxy would then hard-404 every dated shows URL on the site from a
 * body it accepted. Out-of-range is an unanswered probe, which fails open.
 */
function edgeOrdinal(edge: unknown): number | null {
  if (typeof edge !== 'object' || edge === null) return null
  const { year, month } = edge as { year?: unknown; month?: unknown }
  if (typeof year !== 'number' || !Number.isInteger(year)) return null
  if (year < SHOWS_CALENDAR_MIN_YEAR || year > SHOWS_CALENDAR_MAX_YEAR) return null
  if (typeof month !== 'number' || !Number.isInteger(month)) return null
  if (month < 1 || month > 12) return null
  return showsCalendarMonthOrdinal(year, month)
}

function parseShowsCalendarRange(body: unknown): ShowsCalendarRange | null {
  if (typeof body !== 'object' || body === null) return null
  const { first_month: first, last_month: last } = body as Record<string, unknown>

  const firstOrdinal = edgeOrdinal(first)
  const lastOrdinal = edgeOrdinal(last)
  if (firstOrdinal === null || lastOrdinal === null) return null
  // An inverted span would 404 every month including the current one.
  if (lastOrdinal < firstOrdinal) return null

  // A span that does not hold TODAY is refused whatever its shape, and this is
  // the check that bounds the blast radius of every fault upstream of here: a
  // clock adrift on the API host, an API_BASE_URL pointing at the wrong
  // environment, an origin answering with someone else's data. Each of those
  // arrives as a well-ordered span that happens to sit elsewhere on the
  // calendar, and acting on one would hard-404 every dated shows URL at once,
  // the chips included. Refused, it is an unanswered probe, which fails open.
  //
  // It cannot reject an honest span: the backend's own edges are computed from
  // its clock as a band around now, so an honest answer always contains the
  // current month on any clock within a day of this one.
  const now = new Date()
  const currentOrdinal = showsCalendarMonthOrdinal(
    now.getUTCFullYear(),
    now.getUTCMonth() + 1
  )
  if (currentOrdinal < firstOrdinal || currentOrdinal > lastOrdinal) return null

  return { first: firstOrdinal, last: lastOrdinal }
}

/**
 * Read the span from the backend. `null` for every answer that is not one.
 *
 * A 404 is `null` here rather than "missing", which is the opposite of what it
 * means to the entity probes: this endpoint answers a question ABOUT the URL
 * space rather than about one URL, so an API that does not carry the route yet
 * has told us nothing about the month being asked for. That is also the deploy
 * skew, which the frontend reaches first on every release: Vercel finishes
 * building well before Go builds, migrates and passes a healthcheck, and for
 * that window this is the running API's honest answer.
 */
async function fetchShowsCalendarRange(): Promise<ShowsCalendarRange | null> {
  try {
    const res = await fetch(SHOWS_CALENDAR_RANGE_URL, {
      redirect: 'manual',
      // Without this a slow-but-alive backend pins one proxy invocation per
      // request; the abort lands in the catch below, which is the fail-open
      // path, so the page still renders.
      signal: AbortSignal.timeout(EXISTENCE_CHECK_TIMEOUT_MS),
    })
    if (!res.ok) {
      console.warn('shows_calendar_range_probe_unavailable', { status: res.status })
      return null
    }
    const range = parseShowsCalendarRange(await res.json())
    if (range === null) {
      console.warn('shows_calendar_range_probe_unparseable')
    }
    return range
  } catch (error) {
    console.warn('shows_calendar_range_probe_failed', {
      error: error instanceof Error ? error.message : String(error),
    })
    return null
  }
}

/**
 * The span, from this instance's cache when it is warm.
 *
 * A REFRESH THAT FAILS KEEPS THE SPAN IT HAD. Dropping to "unknown" would fail
 * open on every dated URL at the moment the backend is least able to serve
 * them: each one would then render, and a rendered window costs three backend
 * reads where a 404 costs none, which is the amplification loop a 429 storm
 * feeds on. A span minutes old is a better answer than no span, and only a cold
 * instance genuinely has none.
 */
function readShowsCalendarRange(): Promise<ShowsCalendarRange | null> {
  const now = Date.now()
  const cached = showsCalendarRangeCache
  if (cached && cached.expiresAt > now) {
    return cached.range
  }

  const probe = fetchShowsCalendarRange()
  const entry = {
    range: probe.then(async range => range ?? (cached ? await cached.range : null)),
    expiresAt: now + SHOWS_CALENDAR_RANGE_UNKNOWN_TTL_MS,
  }
  showsCalendarRangeCache = entry
  // Stamped from the PROBE rather than from what the entry serves: a span kept
  // because the refresh failed is still an unanswered probe, and it is retried
  // on the short window rather than held for the full one.
  void probe.then(range => {
    entry.expiresAt =
      Date.now() +
      (range === null
        ? SHOWS_CALENDAR_RANGE_UNKNOWN_TTL_MS
        : SHOWS_CALENDAR_RANGE_TTL_MS)
  })
  return entry.range
}

/**
 * Whether a dated shows window names a month the list can be asked about.
 *
 * MONTH resolution on both edges, day segment included: a day is addressable
 * exactly when its month is. The list runs forward from tonight, so the only
 * question a day adds is which month it falls in, and asking the backend per
 * day would turn a span every window shares into a probe per URL.
 *
 * A RUN is judged on its ANCHOR month alone, which is the month its URL names
 * and the month its canonical points at. A hand-built run anchored in a month
 * outside the span but reaching into one inside it is therefore a 404; nothing
 * on the site produces one, because every quick window anchors on today or
 * later and the span holds a week forward of now for exactly that reason.
 *
 * FAILS OPEN, like every other probe in this file and for a stronger reason. A
 * 404 produced from a span nobody answered for would take out every month and
 * day URL at once, including the ones the site's own chips link, where a soft
 * 404 on a quiet month is transient and recoverable.
 */
async function showsCalendarWindowCheck(
  request: NextRequest,
  year: number,
  month: number
): Promise<NextResponse> {
  const range = await readShowsCalendarRange()
  if (range === null) {
    return NextResponse.next()
  }
  const ordinal = showsCalendarMonthOrdinal(year, month)
  if (ordinal < range.first || ordinal > range.last) {
    return notFoundResponse(request)
  }
  return NextResponse.next()
}

export const config = {
  /**
   * Intercept ONLY the enumerated entity prefixes (`/shows/...`, `/venues/...`,
   * `/artists/...`, `/releases/...`, `/labels/...`, `/festivals/...`,
   * `/tags/...`, `/scenes/...`, `/charts/...`). Each prefix scopes its match away from
   * everything else — the homepage, out-of-scope routes (`/collections/...`,
   * `/blog/...`, `/dj-sets/...`), `api`, `_next/static`, `_next/image`,
   * metadata files, and the `/_psy-not-found` rewrite target — so none of those
   * can be blocked or re-intercepted. (A root-level matcher would need a
   * negative-lookahead to exclude `api`/`_next`/metadata; enumerating exact
   * prefixes makes that unnecessary.) The `proxy()` body further narrows each
   * to the exact `/<entity>/<slug>` detail shape — bare `/<entity>` list pages
   * and `/<entity>/<slug>/<sub>` routes pass through untouched. Charts is an
   * allowlist check (no backend probe), not an `ENTITY_CHECKS` entry.
   *
   * Each entry's `missing` excludes RSC prefetch requests (`next-router-
   * prefetch` / `purpose: prefetch` headers) so client-side route prefetches
   * don't fire a backend existence lookup on every hovered link.
   *
   * Keep entity sources in lockstep with `ENTITY_CHECKS` above: a source here with
   * no matching `ENTITY_CHECKS` entry would intercept the route only to fall
   * through `NextResponse.next()` (wasted match), and an `ENTITY_CHECKS` entry
   * with no source here would never run. Charts is the intentional exception.
   */
  matcher: [
    {
      source: '/shows/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/venues/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/artists/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/releases/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/labels/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/festivals/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/tags/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/scenes/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
    {
      source: '/charts/:path*',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
  ],
}
