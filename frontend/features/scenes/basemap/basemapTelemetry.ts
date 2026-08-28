/**
 * Sentry visibility for a failing Atlas street basemap (PSY-1568).
 *
 * WHY THIS EXISTS
 *
 * The street basemap is served by OpenFreeMap's public instance: best effort,
 * no SLA, no contract with us. When it errors, MapLibre ISOLATES the failure —
 * the Black Marble raster, the scene dots, the rings and the DOM labels all
 * keep working — and street zoom degrades to the style's flat near-black
 * background. That degraded state is indistinguishable from deliberate design,
 * so nobody notices, and nothing reported it: MapLibre's only default for an
 * unhandled `error` event is a `console.error` in the user's own tab.
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
 *   404s everywhere is silent here.
 *
 * Both gaps need a WATCHDOG (a timer that asks "are we past street zoom with
 * still no tiles painted?") rather than an error listener, and a watchdog
 * needs a timeout threshold nobody has chosen yet. Deliberately left as a
 * follow-up rather than guessed at — but stated here so the next reader does
 * not mistake this module for full coverage.
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
 * remount and client-side navigation and resets on a hard reload, which is
 * also where Sentry starts a new session. A reload during a sustained outage
 * reporting once more is correct, not a leak.
 *
 * SCOPE: THE VECTOR SOURCE ONLY
 *
 * Only errors carrying the basemap's `sourceId` report. Deliberately NOT
 * covered: the NASA GIBS raster (`nightEarth`), a separate host with a
 * separate failure mode — a follow-up, not a silent inclusion; glyph fetches,
 * which share the host but fire without a `sourceId` and degrade differently
 * (missing labels, streets intact); and every other MapLibre error, which has
 * nothing to do with tile hosting.
 *
 * SCRUBBING
 *
 * MapLibre's AJAXError puts the FULL request URL in `error.message`. Nothing
 * secret rides on a tile URL today, but the house rule (see
 * lib/rate-limit-telemetry) is that full URLs do not leave the process, and a
 * rule with an exception is not a rule. So the event is a fixed
 * `captureMessage` string — stable grouping, no URL in the title — and the
 * diagnosis is carried by structured fields: HTTP status, host, and the tile
 * path reduced by the same `toTelemetryPath` the rest of the app uses.
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
import { toTelemetryPath } from '@/lib/rate-limit-telemetry'
import { PH_BASEMAP_SOURCE_ID, PH_BASEMAP_STYLE_HOST } from './phBasemap'

/**
 * Source ids already reported this page session, keyed by the id that actually
 * passed the filter (never by the constant). That is what makes the collection
 * a real per-source throttle: when the GIBS follow-up widens the filter, each
 * source gets its own slot instead of the first failure silencing the other.
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
 * The terminator is a set of URL-hostile characters rather than `\S`: a
 * greedy run to the next SPACE swallows everything after a URL embedded in a
 * quoted or braced token (`{"url":"https://…","status":503}` collapses to
 * `{"url":"<url>`), destroying the surrounding diagnostic fields along with
 * the URL. Stopping at a quote, bracket, brace or comma keeps them.
 */
function scrubbedMessage(message: string): string {
  const stripped = message.replace(
    /\b[a-z][a-z0-9+.-]*:\/\/[^\s"'`<>()[\]{},;]+/gi,
    '<url>',
  )
  return stripped.length > MAX_MESSAGE_LENGTH
    ? `${stripped.slice(0, MAX_MESSAGE_LENGTH)}...`
    : stripped
}

function hostOf(url: string | undefined): string {
  if (!url) return PH_BASEMAP_STYLE_HOST
  try {
    return new URL(url).hostname
  } catch {
    // A relative or malformed URL tells us nothing about the host; fall back
    // to the host the style is configured against rather than shipping the
    // raw string (which is exactly what must not leave the process).
    return PH_BASEMAP_STYLE_HOST
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
  if (process.env.NODE_ENV === 'production') {
    console.error(scrubbedMessage(String(event.error?.message ?? event.error)))
  } else {
    console.error(event.error)
  }

  try {
    reportBasemapSourceFailure(event)
  } catch {
    // A telemetry failure must never escape into MapLibre's event loop, where
    // it would surface as an unrelated error about the thing that was trying
    // to report an error. There is nowhere useful to report this.
  }
}

function reportBasemapSourceFailure(event: ErrorEvent): void {
  const sourceId = readString((event as SourceErrorEvent).sourceId)
  if (sourceId !== PH_BASEMAP_SOURCE_ID) return
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
      // `basemap_status` is 0 for a network-level failure (DNS, blocked, or a
      // client connection that dropped without `navigator.onLine` catching
      // it) and an HTTP status otherwise, so it also separates the AJAX cases
      // from a style or worker error, which report 'none'. Triage a spike of
      // status 0 as "could be either end" rather than a confirmed outage.
      basemap_source: sourceId,
      basemap_host: hostOf(url),
      basemap_status: status ?? 'none',
    },
    extra: {
      tilePath: url ? toTelemetryPath(url) : undefined,
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
