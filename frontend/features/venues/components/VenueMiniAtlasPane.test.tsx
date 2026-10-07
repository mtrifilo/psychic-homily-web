import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import * as Sentry from '@sentry/nextjs'
import {
  resetStubMaps,
  stubMapOptions,
  stubMaps,
  type StubMap,
} from '@/test/miniAtlasMapStub'
import { ErrorProbe } from '@/test/ErrorProbe'
import { ATLAS_CONTEXT_RESTORE_DEADLINE_MS } from '@/features/scenes/components/atlasMapHealth'
import { VenueMiniAtlasPane } from './VenueMiniAtlasPane'
import type { VenueWithShowCount } from '../types'

// The real map module, loaded through `next/dynamic` (the App Router loader,
// vitest.config.mts), on a MapLibre stub. A map that cannot draw must leave
// the page standing: the box says the map is unavailable, the rest of the pane
// stays, and nothing reaches the route's error page (the probe around the
// pane stands in for it).
vi.mock('maplibre-gl', async () =>
  (await import('@/test/miniAtlasMapStub')).maplibreStubModule(),
)

const ROOMS: VenueWithShowCount[] = [
  {
    id: 1,
    slug: 'busy-room',
    name: 'Busy Room',
    city: 'Phoenix',
    state: 'AZ',
    address: '1 Main St',
    verified: true,
    upcoming_show_count: 20,
    latitude: 33.4,
    longitude: -112.0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
]

async function renderPane() {
  const reachedRoute: unknown[] = []
  render(
    <ErrorProbe onCaught={error => reachedRoute.push(error)}>
      <VenueMiniAtlasPane
        venues={ROOMS}
        scopeCity={{ city: 'Phoenix', state: 'AZ' }}
        hoveredVenueId={null}
        onHoverVenue={vi.fn()}
        onSelectVenue={vi.fn()}
      />
    </ErrorProbe>,
  )
  await screen.findByTestId('venue-mini-atlas-canvas')
  expect(stubMaps).toHaveLength(1)
  return { map: stubMaps[0] as StubMap, reachedRoute }
}

function expectContainedFailure(reachedRoute: unknown[], failureClass: string) {
  expect(screen.getByTestId('venue-mini-atlas-unavailable')).toHaveTextContent(
    /every room is in the table/i,
  )
  expect(screen.queryByTestId('venue-mini-atlas-canvas')).not.toBeInTheDocument()
  expect(screen.getByTestId('venue-mini-atlas-open')).toBeInTheDocument()
  expect(reachedRoute).toEqual([])
  expect(Sentry.captureException).toHaveBeenCalledTimes(1)
  expect(Sentry.captureException).toHaveBeenCalledWith(expect.any(Error), {
    tags: { atlas_map_failure: failureClass, section: 'venue-mini-atlas' },
  })
}

describe('VenueMiniAtlasPane when its map cannot draw', () => {
  let quiet: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    resetStubMaps()
    // React logs every error a boundary catches.
    quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.useRealTimers()
    quiet.mockRestore()
  })

  it('replaces a map whose lost context never comes back', async () => {
    const { map, reachedRoute } = await renderPane()
    vi.useFakeTimers()
    act(() => {
      map.fire('style.load')
      map.fire('load')
    })

    act(() => {
      map.fire('webglcontextlost')
    })
    act(() => {
      vi.advanceTimersByTime(ATLAS_CONTEXT_RESTORE_DEADLINE_MS)
    })

    expectContainedFailure(reachedRoute, 'context-lost')
    expect(map.remove).toHaveBeenCalledTimes(1)
  })

  it('replaces a map whose style fails before it loads', async () => {
    const { map, reachedRoute } = await renderPane()

    act(() => {
      map.fire('error', { error: new Error('style is invalid') })
    })

    expectContainedFailure(reachedRoute, 'style-load-failed')
  })

  it('replaces a map that was refused a WebGL2 context', async () => {
    stubMapOptions.grantContext = false
    const reachedRoute: unknown[] = []
    render(
      <ErrorProbe onCaught={error => reachedRoute.push(error)}>
        <VenueMiniAtlasPane
          venues={ROOMS}
          scopeCity={{ city: 'Phoenix', state: 'AZ' }}
          hoveredVenueId={null}
          onHoverVenue={vi.fn()}
          onSelectVenue={vi.fn()}
        />
      </ErrorProbe>,
    )

    await screen.findByTestId('venue-mini-atlas-unavailable')
    expectContainedFailure(reachedRoute, 'context-refused')
  })

  it('keeps a map that draws when one source fails', async () => {
    const { map, reachedRoute } = await renderPane()
    act(() => {
      map.fire('error', { error: new Error('tiles are down'), sourceId: 'openmaptiles' })
      map.fire('style.load')
      map.fire('load')
    })

    expect(screen.getByTestId('venue-mini-atlas-canvas')).toBeInTheDocument()
    expect(screen.queryByTestId('venue-mini-atlas-unavailable')).not.toBeInTheDocument()
    expect(reachedRoute).toEqual([])
  })
})
