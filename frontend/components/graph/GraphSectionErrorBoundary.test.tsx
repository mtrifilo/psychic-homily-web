import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

const captureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureException(...args),
}))

import { GraphSectionErrorBoundary } from './GraphSectionErrorBoundary'

function Boom(): never {
  throw new Error('chunk boom')
}

describe('GraphSectionErrorBoundary', () => {
  beforeEach(() => {
    captureException.mockReset()
  })

  it('renders children when nothing throws', () => {
    render(
      <GraphSectionErrorBoundary sentryTag="explore-inline-graph">
        <div>graph</div>
      </GraphSectionErrorBoundary>,
    )
    expect(screen.getByText('graph')).toBeInTheDocument()
    expect(captureException).not.toHaveBeenCalled()
  })

  it('self-hides (renders nothing) with no fallback, and reports to Sentry with the tag', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { container } = render(
      <GraphSectionErrorBoundary sentryTag="home-scene-graph">
        <Boom />
      </GraphSectionErrorBoundary>,
    )
    expect(container).toBeEmptyDOMElement()
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { section: 'home-scene-graph' } }),
    )
    spy.mockRestore()
  })

  it('renders the static fallback on error, and reports with the tag', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <GraphSectionErrorBoundary
        sentryTag="explore-inline-graph"
        fallback={<div role="alert">graph unavailable</div>}
      >
        <Boom />
      </GraphSectionErrorBoundary>,
    )
    expect(screen.getByRole('alert')).toHaveTextContent('graph unavailable')
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { section: 'explore-inline-graph' } }),
    )
    spy.mockRestore()
  })

  // Recovery is deliberately NOT a boundary "reset": a reset would re-render the
  // same React.lazy, which permanently caches a rejected chunk import and just
  // re-throws. A working retry lives at the surface (InlineGraph), which remounts
  // this boundary with a fresh key around a freshly-created lazy component — see
  // InlineGraph.errorBoundary.test.tsx.
})
