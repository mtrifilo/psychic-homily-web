/**
 * Sentry visibility for a failing Atlas street basemap (PSY-1568).
 *
 * WHY THIS EXISTS
 *
 * The street basemap is served by OpenFreeMap's public instance: best effort,
 * no SLA, no contract with us. When it stalls, rate-limits, or 500s, MapLibre
 * ISOLATES the failure — the Black Marble raster, the scene dots, the rings and
 * the DOM labels all keep working — and street zoom degrades to the style's
 * flat near-black background. That degraded state is indistinguishable from
 * deliberate design, so nobody notices, and nothing reported it: MapLibre's
 * only default for an unhandled `error` event is a `console.error` in the
 * user's own tab.
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
 * covered:
 *   - the NASA GIBS raster (`nightEarth`), a separate host with a separate
 *     failure mode — a follow-up, not a silent inclusion;
 *   - glyph fetches, which share the OpenFreeMap host but fire without a
 *     `sourceId` and degrade differently (missing labels, streets intact);
 *   - every other MapLibre error, which has nothing to do with tile hosting.
 * Widening the filter is a decision, so it has to be made in this file rather
 * than fall out of a loose `catch (anything)`.
 *
 * A 404 on an individual tile never arrives here at all: MapLibre swallows
 * that case inside its tile loader without firing an event (it is the normal
 * "no data at this tile" answer). What does arrive is the TileJSON fetch
 * failing, and any non-404 tile status.
 *
 * SCRUBBING
 *
 * MapLibre's AJAXError puts the FULL request URL in `error.message`. Nothing
 * secret rides on a tile URL today, but the house rule (see
 * lib/rate-limit-telemetry) is that full URLs do not leave the process, and a
 * rule with an exception is not a rule. So the event is a fixed
 * `captureMessage` string — stable Sentry grouping, no URL in the title — and
 * the diagnosis is carried by structured fields: HTTP status, host, and the
 * tile path reduced by the same `toTelemetryPath` the rest of the app uses.
 * Any message that does ship has URLs stripped outright.
 */

import * as Sentry from '@sentry/nextjs'
import type { ErrorEvent } from 'maplibre-gl'
import { toTelemetryPath } from '@/lib/rate-limit-telemetry'
import { PH_BASEMAP_SOURCE_ID, PH_BASEMAP_TILE_HOST } from './phBasemap'

/**
 * Source ids already reported this page session. Keyed by source rather than
 * held as a single boolean so that adding the GIBS raster later is a filter
 * change, not a rewrite of the throttle.
 */
const reportedSources = new Set<string>()

/** Longest error message we will ship, after URL stripping. */
const MAX_MESSAGE_LENGTH = 200

/**
 * MapLibre types the error event's payload as `{ message: string }` and merges
 * the owning source's id into the event on its way up to the map. Neither the
 * id nor AJAXError's HTTP fields are in the published types, so they are read
 * defensively: a MapLibre upgrade that stops attaching them must degrade to a
 * less detailed event, never throw inside an error handler.
 */
type SourceErrorEvent = ErrorEvent & { sourceId?: unknown }
type AjaxErrorFields = { status?: unknown; url?: unknown; name?: unknown }

function readString(value: unknown): string | undefined {
  return typeof value === 'string' && value !== '' ? value : undefined
}

function hostOf(url: string | undefined): string {
  if (!url) return PH_BASEMAP_TILE_HOST
  try {
    return new URL(url).hostname
  } catch {
    // A relative or malformed URL tells us nothing about the host; fall back
    // to the host the style is configured against rather than shipping the
    // raw string (which is exactly what must not leave the process).
    return PH_BASEMAP_TILE_HOST
  }
}

/**
 * Strip whole URLs out of a message before it is sent. Total and blunt on
 * purpose: it cannot be defeated by a URL shape nobody anticipated, and the
 * structured fields alongside it carry the routing information a stripped URL
 * would have given.
 */
function stripUrls(message: string): string {
  const stripped = message.replace(/\b[a-z][a-z0-9+.-]*:\/\/\S+/gi, '<url>')
  return stripped.length > MAX_MESSAGE_LENGTH
    ? `${stripped.slice(0, MAX_MESSAGE_LENGTH)}...`
    : stripped
}

/**
 * Handle one MapLibre `error` event.
 *
 * Also restores the console output MapLibre would have produced on its own.
 * Registering ANY `error` listener suppresses its built-in `console.error`
 * (it only fires when nothing is listening), so without this line adding
 * telemetry would have made local debugging strictly worse for every error
 * this module deliberately does not report.
 */
export function handleBasemapError(event: ErrorEvent): void {
  console.error(event.error)

  try {
    reportBasemapSourceFailure(event)
  } catch {
    // A telemetry failure must never escape into MapLibre's event loop, where
    // it would surface as an unrelated error about the thing that was trying
    // to report an error. There is nowhere useful to report this.
  }
}

function reportBasemapSourceFailure(event: ErrorEvent): void {
  const sourceId = (event as SourceErrorEvent).sourceId
  if (sourceId !== PH_BASEMAP_SOURCE_ID) return
  if (reportedSources.has(PH_BASEMAP_SOURCE_ID)) return
  reportedSources.add(PH_BASEMAP_SOURCE_ID)

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
      basemap_source: PH_BASEMAP_SOURCE_ID,
      basemap_host: hostOf(url),
      basemap_status: status ?? 'none',
    },
    extra: {
      tilePath: url ? toTelemetryPath(url) : undefined,
      errorName: readString(error?.name),
      errorMessage: message ? stripUrls(message) : undefined,
    },
  })
}
