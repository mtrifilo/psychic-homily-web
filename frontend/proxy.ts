import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { API_BASE_URL } from '@/lib/api-base'

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
      return existenceCheck(request, `${API_BASE_URL}/scenes/${scene}/week`)
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
      return existenceCheck(request, ENTITY_CHECKS.scenes(slug))
    }
    // File-convention OG card on the rolling detail URL. Without this branch
    // `opengraph-image` is a junk period and 404s before Next sees the route
    // (PSY-1785). Probe scene existence, same as `/tonight`: the card's own
    // fetch then draws the current week. The PAGE does not advertise this URL
    // (unfurl caches key on it forever); it stays addressable on its own.
    if (sub === 'opengraph-image') {
      return existenceCheck(request, ENTITY_CHECKS.scenes(slug))
    }
    if (CALENDAR_DATE_SEGMENT.test(sub)) {
      if (!periodYearInRange(sub) || !isRealCalendarDate(sub)) {
        return notFoundResponse(request)
      }
      return existenceCheck(request, ENTITY_CHECKS.scenes(slug))
    }
    // The WEEK key still goes to its own endpoint: `2025-W53` is well-formed
    // and unreal, and unlike a calendar date that verdict is ISO-8601 week
    // arithmetic the backend owns and this file deliberately does not copy.
    if (ISO_WEEK_SEGMENT.test(sub) && periodYearInRange(sub)) {
      return existenceCheck(
        request,
        `${API_BASE_URL}/scenes/${scene}/week/${encodeURIComponent(sub)}`
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
  // Membership is asked of the venue's PAST year histogram, which is the SAME
  // authority the page renders from and the `venue_years` sitemap family is
  // built from. Three surfaces, one source: a year in the sitemap resolves, and
  // a year that resolves is in the strip.
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

  return existenceCheck(request, buildCheckUrl(slug))
}

/**
 * How long an existence probe may take before the proxy gives up and lets the
 * page render. Generous relative to the endpoints it calls (all indexed
 * lookups) and far below any platform function timeout, so the budget that
 * binds is this one rather than the invocation's.
 */
const EXISTENCE_CHECK_TIMEOUT_MS = 2_500

/**
 * Probe a backend URL and turn the result into a pass-through or a real 404.
 *
 * ONE fail-open policy, for every branch above. Extracted originally so the
 * scene-week routes could reuse the generic `/<entity>/<slug>` semantics rather
 * than restate them — a second copy would drift.
 *
 * HEAD, and the STATUS is the whole answer. PSY-1756 briefly widened this with a
 * `verdict` callback, for the one probe that had to read a body because its
 * endpoint answered 200 for any venue that existed; PSY-1770 gave that probe a
 * status-bearing endpoint of its own and the callback went with it. A new caller
 * that finds itself wanting a body should get its endpoint an honest status
 * instead — the backend can answer the question in one indexed row, and a body
 * this function parses is a body every OTHER caller pays to receive.
 *
 * The fail-open rules are the load-bearing part: a backend 404 is the only
 * "missing", and anything else (5xx, 403, 429, a network error) lets the page
 * render. Producing a 404 on a transient blip would mask a real outage as
 * "not found".
 */
async function existenceCheck(
  request: NextRequest,
  url: string
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

    // 404 from the backend = slug genuinely does not exist → real 404.
    if (res.status === 404) {
      return notFoundResponse(request)
    }

    // Any other non-ok (5xx, 403, 429, opaqueredirect, …): fail OPEN — let the
    // page render and apply its own handling (each page's server fetch reports
    // 5xx to Sentry, renders its own not-found on null, etc.).
    if (!res.ok) {
      return NextResponse.next()
    }

    // Backend reachable and the slug resolves.
    return NextResponse.next()
  } catch {
    // Network error reaching the backend: fail OPEN. The proxy must never take
    // a route down when the check itself fails.
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
 * it still paid for a COUNT over the year plus a hydrated row, and shipped a
 * JSON body back over a link that exists to carry one bit. The backend's probe
 * is `venueShowsBaseQuery(venue, "past", year)` with LIMIT 1: the SAME predicate
 * the page renders from and the `venue_years` sitemap family is projected from,
 * so the three still cannot drift.
 *
 * DEPLOY THE BACKEND FIRST. A frontend live ahead of its backend probes a path
 * chi does not know, gets a 404, and cannot tell that from "this year has no
 * shows" — so every venue year archive would 404 for the length of the skew
 * window. That is fail-CLOSED, unlike the scene branches above, and it is the
 * one thing about this branch worth remembering at deploy time.
 */
function venueArchiveYearCheck(
  request: NextRequest,
  slug: string,
  year: number
): Promise<NextResponse> {
  return existenceCheck(
    request,
    `${API_BASE_URL}/venues/${encodeURIComponent(slug)}/shows/${year}/exists`
  )
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
