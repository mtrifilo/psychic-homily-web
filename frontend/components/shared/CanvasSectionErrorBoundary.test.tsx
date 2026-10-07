import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

const captureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureException(...args),
}))

import { CanvasSectionErrorBoundary } from './CanvasSectionErrorBoundary'

function Boom(): never {
  throw new Error('chunk boom')
}

describe('CanvasSectionErrorBoundary', () => {
  beforeEach(() => {
    captureException.mockReset()
  })

  it('renders children when nothing throws', () => {
    render(
      <CanvasSectionErrorBoundary sentryTag="explore-inline-graph">
        <div>graph</div>
      </CanvasSectionErrorBoundary>,
    )
    expect(screen.getByText('graph')).toBeInTheDocument()
    expect(captureException).not.toHaveBeenCalled()
  })

  it('self-hides (renders nothing) with no fallback, and reports to Sentry with the tag', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { container } = render(
      <CanvasSectionErrorBoundary sentryTag="home-scene-graph">
        <Boom />
      </CanvasSectionErrorBoundary>,
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
      <CanvasSectionErrorBoundary
        sentryTag="explore-inline-graph"
        fallback={<div role="alert">graph unavailable</div>}
      >
        <Boom />
      </CanvasSectionErrorBoundary>,
    )
    expect(screen.getByRole('alert')).toHaveTextContent('graph unavailable')
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { section: 'explore-inline-graph' } }),
    )
    spy.mockRestore()
  })

  it('adds the tags read from the caught error, keeping its own section tag', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const errorTags = vi.fn(() => ({ kind: 'boom', section: 'from-error' }))
    render(
      <CanvasSectionErrorBoundary sentryTag="atlas-map" errorTags={errorTags}>
        <Boom />
      </CanvasSectionErrorBoundary>,
    )
    expect(errorTags).toHaveBeenCalledWith(expect.objectContaining({ message: 'chunk boom' }))
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { kind: 'boom', section: 'atlas-map' } }),
    )
    spy.mockRestore()
  })

  it('still reports and notifies when reading the extra tags throws', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const onError = vi.fn()
    render(
      <CanvasSectionErrorBoundary
        sentryTag="atlas-map"
        onError={onError}
        errorTags={() => {
          throw new Error('tags boom')
        }}
      >
        <Boom />
      </CanvasSectionErrorBoundary>,
    )
    expect(captureException).toHaveBeenCalledWith(
      expect.objectContaining({ message: 'chunk boom' }),
      expect.objectContaining({ tags: { section: 'atlas-map' } }),
    )
    expect(onError).toHaveBeenCalledTimes(1)
    spy.mockRestore()
  })

  // Recovery is deliberately NOT a boundary "reset": a reset would re-render the
  // same React.lazy, which permanently caches a rejected chunk import and just
  // re-throws. A working retry lives at the surface (InlineGraph), which remounts
  // this boundary with a fresh key around a freshly-created lazy component — see
  // InlineGraph.errorBoundary.test.tsx.
})
