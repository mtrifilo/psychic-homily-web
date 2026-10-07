import { vi } from 'vitest'
import { render } from '@testing-library/react'
import { VenueMiniAtlasPane } from '@/features/venues/components/VenueMiniAtlasPane'
import type { VenueWithShowCount } from '@/features/venues/types'
import { ErrorProbe } from './ErrorProbe'

/** One mappable Phoenix room, enough for the pane to render its map. */
export const MINI_ATLAS_ROOMS: VenueWithShowCount[] = [
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

/**
 * Mounts the pane inside an {@link ErrorProbe} standing where the route's
 * error page would catch. `reachedRoute` collects anything that escaped the
 * pane.
 */
export function mountPaneInProbe(reachedRoute: unknown[] = []) {
  const view = render(
    <ErrorProbe onCaught={error => reachedRoute.push(error)}>
      <VenueMiniAtlasPane
        venues={MINI_ATLAS_ROOMS}
        scopeCity={{ city: 'Phoenix', state: 'AZ' }}
        hoveredVenueId={null}
        onHoverVenue={vi.fn()}
        onSelectVenue={vi.fn()}
      />
    </ErrorProbe>,
  )
  return { ...view, reachedRoute }
}
