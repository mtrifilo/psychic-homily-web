import { describe, it, expect } from 'vitest'
import { Evented } from 'maplibre-gl'
import maplibrePackage from 'maplibre-gl/package.json'
import { PH_BASEMAP_SOURCE_ID } from './phBasemap'

/**
 * Pins the MapLibre INTERNALS basemapTelemetry depends on (PSY-1568).
 *
 * The whole failure signal rests on one undocumented behaviour: an error fired
 * by a source arrives at the map carrying `sourceId`. That field is NOT on
 * MapLibre's published `ErrorEvent` type (`maplibre-gl.d.ts` declares only
 * `error: ErrorLike`). It exists because `Style.addSource` hands
 * `setEventedParent` a data FUNCTION returning `{ …, sourceId: id }`, and
 * `Evented.fire` merges that into the event as it bubbles.
 *
 * If a MapLibre upgrade changes either half, `sourceId` becomes `undefined`,
 * the filter in basemapTelemetry stops matching, and EVERY basemap failure
 * goes unreported — a silent death of the very signal that exists to stop
 * silent failures. Nothing else would notice: the unit tests fabricate the
 * event shape, and the style-JSON guards say nothing about runtime plumbing.
 *
 * So this exercises the REAL `Evented` bubble rather than a fixture, and pins
 * the version alongside it. Same intent as maplibreVendored.test.ts: an
 * upgrade fails a test instead of shipping a quietly dead map.
 *
 * On failure: re-verify that a source error still reaches `map.on('error')`
 * with `sourceId` (`Style.addSource` → `setEventedParent`, and `Evented.fire`
 * → `extend(event, eventedParentData)`), then bump the version below.
 */

const VERIFIED_MAPLIBRE_VERSION = '6.0.0'

describe('maplibre error-event contract', () => {
  it('is pinned to the verified maplibre-gl version', () => {
    expect(
      maplibrePackage.version,
      're-verify the sourceId bubble (see file doc), then bump this',
    ).toBe(VERIFIED_MAPLIBRE_VERSION)
  })

  it('merges the parent data (sourceId) onto an error as it bubbles', () => {
    // The same wiring Style.addSource sets up: child fires, parent supplies
    // the per-source data, listener on the parent sees them merged.
    // `Evented` is abstract, so stand in the two ends of the real chain.
    class Node extends Evented {}
    const map = new Node()
    const tileManager = new Node()
    tileManager.setEventedParent(map, () => ({
      sourceId: PH_BASEMAP_SOURCE_ID,
    }))

    const seen: unknown[] = []
    map.on('error', (event: unknown) => seen.push(event))

    // Fire a real error-shaped event through the child.
    tileManager.fire({
      type: 'error',
      error: new Error('boom'),
    } as never)

    const errorEvent = seen.at(-1) as { sourceId?: unknown }
    expect(errorEvent.sourceId).toBe(PH_BASEMAP_SOURCE_ID)
  })
})
