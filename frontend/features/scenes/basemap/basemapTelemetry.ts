/**
 * Sentry visibility for a failing Atlas basemap tile source (PSY-1568, widened
 * to the NASA GIBS raster in PSY-1936).
 *
 * WHY THIS EXISTS
 *
 * Both halves of the Atlas basemap come from third-party public instances:
 * best effort, no SLA, no contract with us. OpenFreeMap serves the street
 * vector tiles; NASA GIBS serves the Black Marble night-lights raster that IS
 * the globe. When either errors, MapLibre ISOLATES the failure — the scene
 * dots, the rings and the DOM labels all keep working — and the failing half
 * degrades to the style's flat near-black background: a dark rectangle at
 * street zoom, an unlit sphere at globe zoom. Both degraded states are
 * indistinguishable from deliberate design (the whole Atlas palette is
 * near-black), so nobody notices, and nothing reported it: MapLibre's only
 * default for an unhandled `error` event is a `console.error` in the user's
 * own tab.
 *
 * WHAT THIS DOES AND DOES NOT CATCH
 *
 * This is an `error`-EVENT listener, so it sees exactly what MapLibre chooses
 * to fire. Verified against maplibre-gl 6.0.0:
 *
 *   CAUGHT — the TileJSON fetch failing (`/planet` 5xx, DNS failure, blocked,
 *   CORS), and any tile response whose status is not 404.
 *
 *   NOT CAUGHT — a STALL. MapLibre sets no timeout on its tile requests, so a
 *   host that accepts the connection and never answers leaves the fetch
 *   pending forever: nothing rejects, no event fires, and this handler is
 *   never called. The user still sees the black rectangle.
 *
 *   NOT CAUGHT — a blanket tile 404 while the TileJSON still serves 200.
 *   MapLibre swallows 404 inside its tile loader without firing an event (it
 *   treats it as the normal "no data at this tile" answer), so a pyramid that
 *   404s everywhere is silent here. The raster source shares this gap: GIBS
 *   answering 404 across the pyramid leaves an unlit globe and no event.
 *
 * Both gaps need a WATCHDOG (a timer that asks "is this source's layer visible
 * with still no tiles painted?") rather than an error listener, and a watchdog
 * needs a timeout threshold nobody has chosen yet. Deliberately left as a
 * follow-up rather than guessed at — but stated here so the next reader does
 * not mistake this module for full coverage. Widening the filter to the raster
 * source does not narrow these gaps; it only means a raster STALL or blanket
 * 404 is now the only silent GIBS failure rather than one of several.
 *
 * ONE SIGNAL PER SESSION PER SOURCE
 *
 * A basemap outage is not one error; it is one error per tile per pan, which
 * would be hundreds of near-identical events for a single incident. The guard
 * below therefore reports a source AT MOST ONCE and then stays quiet for the
 * rest of the page session, however many tiles keep failing.
 *
 * The guard is MODULE-LEVEL on purpose. Atlas creates a fresh MapLibre map on
 * every show (Cache Components hide/show, see GlobeCanvas), so anything held
 * in component state or a ref would reset on the first nav-away and report
 * again on the way back — one signal per navigation, which is the flood this
 * exists to prevent. Module scope is the page session: it survives every
 * remount and client-side navigation and resets on a hard reload. A reload
 * during a sustained outage reporting once more is correct, not a leak.
 *
 * NOTE for whoever reads this issue in Sentry: this window is NARROWER than a
 * Sentry session. `browserSessionIntegration` is on by default and starts a
 * new session on every client-side navigation, so one page load that visits
 * three routes is three Sentry sessions but at most one event from here.
 * "Sessions affected" and crash-free rate therefore UNDERSTATE a sustained
 * outage, by roughly however many in-app navigations affected users make.
 * Count distinct users or events, not sessions.
 *
 * SCOPE: THE TWO TILE-HOSTING SOURCES
 *
 * Only errors carrying one of the two basemap tile sources' `sourceId` report:
 * the OpenFreeMap vector source and the NASA GIBS raster. Everything else is
 * ignored — the GeoJSON sources (`scenes`, `scene-rings`, `venues`) are fed
 * from local data and have no host to be down, and a map-level error carries no
 * `sourceId` at all. Each source is tagged with its own `basemap_source` and
 * throttled in its own slot, so a total GIBS outage and a total OpenFreeMap
 * outage are two distinct issues in Sentry rather than one ambiguous one.
 *
 * Glyphs are a THIRD gap, and not a filtered one. A failed glyph range never
 * reaches this handler at all: `GlyphManager` catches the fetch rejection,
 * renders the codepoint locally via TinySDF and emits only a `warnOnce`, so
 * labels quietly fall back to a substitute face rather than going missing.
 * The one path where a glyph failure does escape (through the worker's tile
 * parse) arrives carrying the VECTOR SOURCE's `sourceId`, so it reports here
 * as a tile-source failure — mis-attributed, but at least not silent.
 *
 * SCRUBBING
 *
 * MapLibre's AJAXError puts the FULL request URL in `error.message`. Nothing
 * secret rides on a tile URL today, but the house rule (see
 * lib/rate-limit-telemetry) is that full URLs do not leave the process, and a
 * rule with an exception is not a rule. So the event is a fixed
 * `captureMessage` string — stable grouping, no URL in the title — and the
 * diagnosis is carried by structured fields: HTTP status, host, and a
 * URL-stripped message.
 *
 * The console line needs the same care, and this is the non-obvious part.
 * Sentry's console breadcrumb keeps BOTH a joined `message` AND the raw
 * `data.arguments` array, and `normalizeEvent` expands an Error argument into
 * its own properties — so `console.error(ajaxError)` would ship `.url` and the
 * unscrubbed `.message` on the very event captured a line later, whatever the
 * breadcrumb `message` said. Rather than try to scrub the SDK's breadcrumb
 * (app-wide surgery, and a separate concern from this ticket), the raw object
 * simply never reaches `console.error` in production. Development still logs
 * it in full, because that is where the stack is actually read and where no
 * breadcrumb is being shipped.
 */

import * as Sentry from '@sentry/nextjs'
import type { ErrorEvent } from 'maplibre-gl'
import {
  NIGHT_EARTH_SOURCE_ID,
  NIGHT_EARTH_TILE_HOST,
} from './nightEarthRaster'
import { PH_BASEMAP_SOURCE_ID, PH_BASEMAP_STYLE_HOST } from './phBasemap'

/**
 * The tile sources this module reports, each mapped to the host it is
 * CONFIGURED against — the `basemap_host` fallback for an error that arrives
 * without a URL of its own (a style/worker error, or an AJAXError whose `url`
 * MapLibre did not attach).
 *
 * A map, not two ifs: the host fallback has to be picked per source, and a
 * single lookup makes it impossible to widen the filter without also deciding
 * what host the new source degrades to. Adding a third tile host means adding
 * one entry here and nothing else.
 */
const REPORTED_SOURCE_HOSTS = new Map<string, string>([
  [PH_BASEMAP_SOURCE_ID, PH_BASEMAP_STYLE_HOST],
  [NIGHT_EARTH_SOURCE_ID, NIGHT_EARTH_TILE_HOST],
])

/**
 * Source ids already reported this page session, keyed by the id that actually
 * passed the filter (never by a constant). That is what makes the collection a
 * real per-source throttle: each source gets its own slot, so a GIBS failure
 * early in the session cannot silence the OpenFreeMap signal (or the reverse)
 * — the two hosts fail independently and a single-slot guard would report
 * whichever lost the race and hide the other.
 */
const reportedSources = new Set<string>()

/** Length budget for the one free-text field on the event. */
const MAX_MESSAGE_LENGTH = 200

/**
 * MapLibre types the error event's payload as `{ message: string }` and merges
 * the owning source's id into the event on its way up to the map. Neither the
 * id nor AJAXError's HTTP fields are in the published types, so they are read
 * defensively: a MapLibre upgrade that stops attaching them must degrade to a
 * less detailed event, never throw inside an error handler.
 */
type SourceErrorEvent = ErrorEvent & { sourceId?: unknown }
// No `name`: MapLibre's AJAXError never sets one, and `ensureError` wraps
// non-Errors in a plain Error, so it is always the constant 'Error' in
// production — a field that reads as diagnostic and carries nothing.
type AjaxErrorFields = { status?: unknown; url?: unknown }

function readString(value: unknown): string | undefined {
  return typeof value === 'string' && value !== '' ? value : undefined
}

/**
 * Replace whole URLs in a free-text message with `<url>`, then cap the length.
 *
 * The terminator set is the load-bearing detail, and it has to fail CLOSED.
 * Running to the next SPACE over-eats: a URL inside a quoted or braced token
 * (`{"url":"https://…","status":503}`) swallows the diagnostic fields after
 * it. But excluding every character that merely LOOKS un-URL-ish under-scrubs,
 * which is far worse: `,` `;` `(` `[` are all legal in a query string, so
 * stopping at them leaves `<url>,2,3,4&api_key=SECRET` — the credential
 * shipped, past a control that reads as if it prevented exactly that. Only
 * structural JSON/markup delimiters, which cannot appear unencoded in a URL
 * this would be embedded in, terminate the match.
 *
 * The cap is applied to the INPUT as well, before the replace. The pattern
 * backtracks quadratically on long dotted or hyphenated runs (~4s on 120KB),
 * and this runs on the main thread inside an error handler; capping only the
 * output would bound the payload while leaving the work unbounded.
 */
const URL_IN_TEXT = /\b[a-z][a-z0-9+.-]*:\/\/[^\s"'`<>{}]+/gi

function scrubbedMessage(message: string): string {
  const capped =
    message.length > MAX_MESSAGE_LENGTH * 2
      ? message.slice(0, MAX_MESSAGE_LENGTH * 2)
      : message
  const stripped = capped.replace(URL_IN_TEXT, '<url>')
  return stripped.length > MAX_MESSAGE_LENGTH
    ? `${stripped.slice(0, MAX_MESSAGE_LENGTH)}...`
    : stripped
}

function hostOf(url: string | undefined, configuredHost: string): string {
  if (!url) return configuredHost
  try {
    return new URL(url).hostname
  } catch {
    // A relative or malformed URL tells us nothing about the host; fall back
    // to the host the failing source is configured against rather than
    // shipping the raw string (which is exactly what must not leave the
    // process).
    return configuredHost
  }
}

/**
 * Handle one MapLibre `error` event.
 *
 * Also restores the console output MapLibre would have produced on its own.
 * Registering ANY `error` listener suppresses its built-in `console.error`
 * (it only fires when nothing is listening), so without this line adding
 * telemetry would have made local debugging strictly worse for every error
 * this module deliberately does not report. See the module doc for why
 * production logs the scrubbed string rather than the raw error.
 */
export function handleBasemapError(event: ErrorEvent): void {
  try {
    // Gated on whether Sentry is COLLECTING, not on NODE_ENV. The invariant
    // being protected is "no raw error reaches a breadcrumb that ships", and a
    // developer who sets a DSN locally to debug Sentry would otherwise
    // silently reinstate the leak while a comment claimed it could not happen.
    // No DSN, no breadcrumb, so the console keeps the object and its stack.
    if (process.env.NEXT_PUBLIC_SENTRY_DSN) {
      const text = readString(event.error?.message) ?? String(event.error)
      console.error(scrubbedMessage(text))
    } else {
      console.error(event.error)
    }

    reportBasemapSourceFailure(event)
  } catch {
    // A telemetry failure must never escape into MapLibre's event loop, where
    // it would surface as an unrelated error about the thing that was trying
    // to report an error. There is nowhere useful to report this.
  }
}

function reportBasemapSourceFailure(event: ErrorEvent): void {
  const sourceId = readString((event as SourceErrorEvent).sourceId)
  if (sourceId === undefined) return
  const configuredHost = REPORTED_SOURCE_HOSTS.get(sourceId)
  if (configuredHost === undefined) return
  if (reportedSources.has(sourceId)) return

  // A browser that KNOWS it is offline tells us nothing about the provider.
  // Without this, every tunnel and wifi-to-cellular handoff on mobile reports
  // "the tile host is down" at error level, and that class of false positive
  // would swamp the real signal this exists to make visible. Deliberately does
  // NOT stamp the throttle: the session has not spent its one report, so a
  // genuine outage later in the same session still gets through.
  if (typeof navigator !== 'undefined' && navigator.onLine === false) return

  const error = event.error as (AjaxErrorFields & { message?: unknown }) | null
  const status = typeof error?.status === 'number' ? error.status : undefined
  const url = readString(error?.url)
  const message = readString(error?.message)

  Sentry.captureMessage('Atlas basemap tile source failed', {
    level: 'error',
    tags: {
      service: 'atlas-basemap',
      error_type: 'basemap_source_failed',
      // Low-cardinality and searchable: "which source, on which host, failing
      // how" is the whole triage question for a third-party tile outage.
      // `basemap_source` is what separates the two halves of the basemap —
      // 'openmaptiles' (streets are gone) from 'nightEarth' (the globe is
      // unlit) — which are different user-visible failures on different
      // providers and should never group into one issue.
      // `basemap_status` is 0 for a network-level failure (DNS, blocked, or a
      // client connection that dropped without `navigator.onLine` catching
      // it) and an HTTP status otherwise, so it also separates the AJAX cases
      // from a style or worker error, which report 'none'. Triage a spike of
      // status 0 as "could be either end" rather than a confirmed outage.
      basemap_source: sourceId,
      basemap_host: hostOf(url, configuredHost),
      basemap_status: status ?? 'none',
    },
    extra: {
      // No tile path. `toTelemetryPath` placeholders every all-digit segment,
      // which on a `/z/x/y.pbf` URL destroys the ZOOM — the one field worth
      // triaging on for a ticket about street zoom degrading — while keeping
      // the raw `y` coordinate, because `6101.pbf` is not purely numeric.
      // Backwards on both counts, so it is left out rather than shipped
      // looking informative. Status, host and the message carry the diagnosis.
      errorMessage: message ? scrubbedMessage(message) : undefined,
    },
  })

  // Stamped after the call, which guards only against a SYNCHRONOUS throw
  // (a patched or misconfigured SDK). It is deliberately not a delivery
  // guarantee: `captureMessage` queues the event and returns an id
  // immediately, so a transport that later fails — offline, ad blocker,
  // Sentry down — rejects asynchronously where nothing here can see it, and
  // that session's slot is spent regardless. Making the stamp conditional on
  // real delivery would mean subscribing to `afterSendEvent`, which buys
  // little: the sessions that lose the event are largely the ones that could
  // not have reported anyway.
  reportedSources.add(sourceId)
}
