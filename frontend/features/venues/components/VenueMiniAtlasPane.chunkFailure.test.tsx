import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import * as Sentry from '@sentry/nextjs'
import { mountPaneInProbe } from '@/test/miniAtlasPaneFixture'

// A map module that fails to load (a deploy rotated its hashed chunk, a
// network drop) must leave the reader on the table: the pane's box says the
// map is unavailable and nothing reaches the route's error page (the probe
// around the pane stands in for it). `next/dynamic` is the App Router loader
// here (vitest.config.mts), which throws a failed import to the nearest error
// boundary.
vi.mock('./VenueMiniAtlas', () =>
  Promise.reject(new TypeError('Failed to fetch dynamically imported module')),
)

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
    const first = mountPaneInProbe()
    const { reachedRoute } = first

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
    mountPaneInProbe(reachedRoute)
    expect(
      await screen.findByTestId('venue-mini-atlas-unavailable'),
    ).toBeInTheDocument()
    expect(reachedRoute).toEqual([])
  })
})
