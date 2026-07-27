import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { installImmediateResizeObserver } from '@/test/mocks/resizeObserver'
import type { ArtistGraph } from '../types'

// PSY-1575: at desktop width the Connections count line promises "click a name
// to see how it connects", but the canvas below it sits in a
// GraphSectionErrorBoundary mounted with NO fallback — it self-hides. A failed
// react-force-graph-2d chunk fetch (the dominant trigger: a deploy rotates the
// hashed chunk while the tab is open) therefore used to leave the instruction
// pointing at nothing. The clause must retract with the canvas.

const captureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureException(...args),
}))

const mockGraph: ArtistGraph = {
  center: {
    id: 100,
    name: 'Center Artist',
    slug: 'center-artist',
    upcoming_show_count: 0,
    has_playable_audio: false,
  },
  nodes: [1, 2].map(id => ({
    id,
    name: `Artist ${id}`,
    slug: `artist-${id}`,
    upcoming_show_count: 0,
    has_playable_audio: false,
  })),
  links: [1, 2].map(id => ({
    source_id: 100,
    target_id: id,
    type: 'shared_bills' as const,
    score: 1 - id * 0.1,
    votes_up: 0,
    votes_down: 0,
  })),
}

vi.mock('../hooks/useArtistGraph', () => ({
  useArtistGraph: () => ({ data: mockGraph, isLoading: false }),
}))

vi.mock('../hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: () => ({ data: undefined, isError: false }),
}))

// The canvas stands in for the dynamic chunk: throwing at render is exactly
// what a rejected react-force-graph-2d import does under the App Router.
// Mutable so a test can model "the next artist's graph is healthy".
let chunkFails = true

vi.mock('./ArtistGraph', () => ({
  ArtistGraphVisualization: () => {
    if (chunkFails) throw new Error('chunk fetch failed')
    return <div data-testid="connections-canvas" />
  },
}))

import { ArtistConnectionsSection } from './ArtistConnectionsSection'

const CLAUSE = /click a name to see how it connects/

/** The count line is the only <p> the section renders directly. */
function countLineText(): string {
  const el = document.querySelector('section > p')
  if (!el) throw new Error('count line not rendered')
  return el.textContent ?? ''
}

describe('ArtistConnectionsSection — graph failure (PSY-1575)', () => {
  let resizeObserver: ReturnType<typeof installImmediateResizeObserver>
  let consoleError: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    chunkFails = true
    captureException.mockReset()
    resizeObserver = installImmediateResizeObserver()
    // React logs the caught error tree; the throw is the point of these tests.
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    resizeObserver.restore()
    consoleError.mockRestore()
  })

  it('drops the interaction clause when the graph subtree throws, keeping header and count', () => {
    renderWithProviders(
      <ArtistConnectionsSection
        artistId={100}
        artistName="Center Artist"
        onExpand={vi.fn()}
      />
    )

    // The section survives: a graph failure must not dent the artist page.
    expect(
      screen.getByRole('heading', { name: 'Connections' })
    ).toBeInTheDocument()
    // Count still discloses scale — but WITHOUT the promise of interaction.
    // Raw textContent so a stray separator can't slip past the normalizer.
    expect(countLineText()).toBe('2 connected artists')
    expect(screen.queryByText(CLAUSE)).not.toBeInTheDocument()

    // Boundary still self-hides (no fallback) and still reports to Sentry.
    expect(screen.queryByTestId('connections-canvas')).not.toBeInTheDocument()
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({
        tags: { section: 'artist-connections-section' },
      })
    )
  })

  it('does not latch the failure across an artist change — a healthy graph gets its clause back', () => {
    const { rerender } = renderWithProviders(
      <ArtistConnectionsSection
        artistId={100}
        artistName="Center Artist"
        onExpand={vi.fn()}
      />
    )
    expect(countLineText()).toBe('2 connected artists')

    // Navigate to another artist whose graph loads fine. Neither the
    // per-artist `graphFailed` flag nor the boundary's latched `failed` state
    // may survive: both are keyed to the artist they were observed for.
    chunkFails = false
    rerender(
      <ArtistConnectionsSection
        artistId={101}
        artistName="Other Artist"
        onExpand={vi.fn()}
      />
    )

    expect(countLineText()).toBe(
      '2 connected artists · click a name to see how it connects'
    )
    expect(screen.getByTestId('connections-canvas')).toBeInTheDocument()
  })
})
