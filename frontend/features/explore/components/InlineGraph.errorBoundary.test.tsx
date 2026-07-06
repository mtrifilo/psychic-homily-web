import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

// PSY-1359: a failed ForceGraphView chunk fetch throws (App Router) into the
// GraphSectionErrorBoundary InlineGraph now wraps the mount in — verify the
// user sees the recoverable GraphLoadError card (not an uncaught bubble / an
// infinite skeleton), that it reports to Sentry, and that retry recovers.

const captureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureException(...args),
}))

// The lazy ForceGraphView throws while `shouldThrow` is set — standing in for a
// chunk-fetch failure at mount. createLazyForceGraphView is called at module
// scope, so the returned component is stable across renders.
const graphState = { shouldThrow: true }
vi.mock('@/components/graph/lazyForceGraphView', () => ({
  createLazyForceGraphView: () =>
    function LazyGraphStub({ ariaLabel }: { ariaLabel: string }) {
      if (graphState.shouldThrow) throw new Error('chunk fetch failed')
      return <div data-testid="force-graph" aria-label={ariaLabel} />
    },
}))

// Force the canvas-usable width gate open (jsdom measures 0) so the graph
// actually mounts; keep the real breakpoint constant.
vi.mock('@/components/graph/useContainerWidth', () => ({
  useContainerWidth: () => ({ refCallback: () => {}, containerWidth: 1024 }),
  GRAPH_BREAKPOINT_PX: 640,
}))

// Skip IntersectionObserver — mount immediately.
vi.mock('@/components/graph/useLazyGraphMount', () => ({
  useLazyGraphMount: () => ({ containerRef: { current: null }, isMounted: true }),
}))

vi.mock('next/navigation', () => ({ useRouter: () => ({ push: vi.fn() }) }))

vi.mock('@/features/shows', () => ({
  useShow: () => ({ data: { artists: [{ id: 22, is_headliner: true }] } }),
}))

vi.mock('@/features/artists/hooks/useArtistGraph', () => ({
  useArtistGraph: () => ({
    data: { nodes: [{ id: 22, slug: 'a', name: 'A' }], links: [] },
    isLoading: false,
  }),
}))

import { InlineGraph } from './InlineGraph'

describe('InlineGraph chunk-failure recovery (PSY-1359)', () => {
  beforeEach(() => {
    captureException.mockReset()
    graphState.shouldThrow = true
  })

  it('shows the recoverable GraphLoadError card and reports to Sentry, then retry recovers', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <InlineGraph billSlug="b" billTitle="Big Show" billHref="/shows/b" />,
    )

    // Perceivable, recoverable state — not an uncaught throw or an endless skeleton.
    expect(screen.getByRole('alert')).toHaveTextContent(/couldn.t load/i)
    expect(screen.queryByTestId('force-graph')).toBeNull()
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { section: 'explore-inline-graph' } }),
    )

    // Retry (boundary reset) after the transient failure clears → the graph mounts.
    graphState.shouldThrow = false
    fireEvent.click(screen.getByRole('button', { name: /try again/i }))
    expect(screen.getByTestId('force-graph')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).toBeNull()

    spy.mockRestore()
  })
})
