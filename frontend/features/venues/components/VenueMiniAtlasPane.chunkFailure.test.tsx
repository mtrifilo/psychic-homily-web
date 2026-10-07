import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import * as Sentry from '@sentry/nextjs'
import { ErrorProbe } from '@/test/ErrorProbe'
import { VenueMiniAtlasPane } from './VenueMiniAtlasPane'
import type { VenueWithShowCount } from '../types'

// A map module that fails to load (a deploy rotated its hashed chunk, a
// network drop) must leave the reader on the table: the pane's box says the
// map is unavailable and nothing reaches the route's error page (the probe
// around the pane stands in for it). `next/dynamic` is the App Router loader
// here (vitest.config.mts), which throws a failed import to the nearest error
// boundary.
vi.mock('./VenueMiniAtlas', () =>
  Promise.reject(new TypeError('Failed to fetch dynamically imported module')),
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

function renderPane(reachedRoute: unknown[]) {
  return render(
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
}

describe('VenueMiniAtlasPane when the map module fails to load', () => {
  let quiet: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    // React logs every error a boundary catches.
    quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    quiet.mockRestore()
  })

  it('says the map is unavailable in its box and keeps the page', async () => {
    const reachedRoute: unknown[] = []
    const first = renderPane(reachedRoute)

    expect(
      await screen.findByTestId('venue-mini-atlas-unavailable'),
    ).toHaveTextContent(/every room is in the table/i)
    expect(screen.getByTestId('venue-mini-atlas-open')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(reachedRoute).toEqual([])
    expect(Sentry.captureException).toHaveBeenCalledWith(expect.any(Error), {
      tags: { section: 'venue-mini-atlas' },
    })

    // React.lazy keeps the rejected import, so a later mount in the same
    // page load falls back the same way.
    first.unmount()
    renderPane(reachedRoute)
    expect(
      await screen.findByTestId('venue-mini-atlas-unavailable'),
    ).toBeInTheDocument()
    expect(reachedRoute).toEqual([])
  })
})
