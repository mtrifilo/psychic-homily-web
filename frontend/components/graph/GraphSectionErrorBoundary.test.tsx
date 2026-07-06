import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

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

  it('renders the fallback with a working reset, and reports with the tag', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <GraphSectionErrorBoundary
        sentryTag="explore-inline-graph"
        fallback={reset => (
          <button type="button" onClick={reset}>
            Try again
          </button>
        )}
      >
        <Boom />
      </GraphSectionErrorBoundary>,
    )
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { section: 'explore-inline-graph' } }),
    )
    spy.mockRestore()
  })

  it('reset re-attempts the children — a transient failure recovers', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})

    // Throws while `throwing` is set (a transient chunk hiccup); the test clears
    // it before clicking reset, so the re-render succeeds.
    const state = { throwing: true }
    function TransientChild() {
      if (state.throwing) throw new Error('transient')
      return <div>graph</div>
    }

    render(
      <GraphSectionErrorBoundary
        sentryTag="explore-inline-graph"
        fallback={reset => (
          <button type="button" onClick={reset}>
            Try again
          </button>
        )}
      >
        <TransientChild />
      </GraphSectionErrorBoundary>,
    )

    // First render threw → fallback.
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
    expect(screen.queryByText('graph')).toBeNull()

    // Reset → children re-render → this time they succeed.
    state.throwing = false
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(screen.getByText('graph')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()

    spy.mockRestore()
  })
})
