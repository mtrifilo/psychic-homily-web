import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { InlineGraph } from './InlineGraph'

// Stub the heavy ForceGraphView so tests don't pull in d3-force / canvas.
vi.mock('@/components/graph/ForceGraphView', () => ({
  ForceGraphView: ({ ariaLabel }: { ariaLabel: string }) => (
    <div role="img" aria-label={ariaLabel} data-testid="force-graph">
      graph
    </div>
  ),
}))

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

const mockUseShow = vi.fn()
vi.mock('@/features/shows', () => ({
  useShow: (slug: string) => mockUseShow(slug),
}))

const mockUseArtistGraph = vi.fn()
vi.mock('@/features/artists/hooks/useArtistGraph', () => ({
  useArtistGraph: (opts: unknown) => mockUseArtistGraph(opts),
}))

// The test/setup.ts file installs a no-op IntersectionObserver. Override
// it with one that records the callback so the test can fire visibility
// events on demand.
type ObserverInstance = {
  callback: IntersectionObserverCallback
  observed: Element[]
}
const observers: ObserverInstance[] = []

class FiringIntersectionObserver implements IntersectionObserver {
  readonly root = null
  readonly rootMargin = ''
  readonly thresholds = []
  private callback: IntersectionObserverCallback
  private observed: Element[] = []

  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback
    observers.push({ callback, observed: this.observed })
  }
  observe(target: Element) {
    this.observed.push(target)
  }
  unobserve() {}
  disconnect() {
    this.observed.length = 0
  }
  takeRecords(): IntersectionObserverEntry[] {
    return []
  }
}

function fireAllVisible() {
  for (const obs of observers) {
    const entries: IntersectionObserverEntry[] = obs.observed.map(target => ({
      isIntersecting: true,
      target,
      time: 0,
      boundingClientRect: {} as DOMRectReadOnly,
      intersectionRatio: 1,
      intersectionRect: {} as DOMRectReadOnly,
      rootBounds: null,
    }))
    obs.callback(entries, {} as IntersectionObserver)
  }
}

// setup.ts installs a no-op IntersectionObserver via
// `Object.defineProperty(window, ..., { writable: true })` — assignable
// but NOT configurable, so we mutate via assignment and restore the
// same way in afterEach.
let originalObserver: typeof IntersectionObserver | undefined

beforeEach(() => {
  observers.length = 0
  originalObserver = window.IntersectionObserver
  ;(window as unknown as { IntersectionObserver: unknown }).IntersectionObserver =
    FiringIntersectionObserver
  mockUseShow.mockReset()
  mockUseArtistGraph.mockReset()
})

afterEach(() => {
  if (originalObserver) {
    ;(window as unknown as { IntersectionObserver: unknown }).IntersectionObserver =
      originalObserver
  }
})

describe('InlineGraph', () => {
  it('renders a skeleton placeholder before IntersectionObserver fires', () => {
    mockUseShow.mockReturnValue({ data: undefined })
    mockUseArtistGraph.mockReturnValue({ data: undefined, isLoading: false })

    const { container } = render(
      <InlineGraph
        billSlug="featured-bill"
        billTitle="Big Show"
        billHref="/shows/featured-bill"
      />,
    )

    expect(container.querySelector('.animate-pulse')).toBeTruthy()
    expect(screen.queryByTestId('force-graph')).toBeNull()
  })

  it('passes the headliner artist id to the graph hook once mounted', () => {
    mockUseShow.mockReturnValue({
      data: {
        artists: [
          { id: 11, is_headliner: false },
          { id: 22, is_headliner: true },
        ],
      },
    })
    mockUseArtistGraph.mockReturnValue({
      data: { center: { id: 22 }, nodes: [], links: [] },
      isLoading: false,
    })

    render(
      <InlineGraph
        billSlug="featured-bill"
        billTitle="Big Show"
        billHref="/shows/featured-bill"
      />,
    )

    act(() => {
      fireAllVisible()
    })

    const calls = mockUseArtistGraph.mock.calls
    const lastCall = calls[calls.length - 1][0] as {
      artistId: number
      enabled: boolean
    }
    expect(lastCall.artistId).toBe(22)
    expect(lastCall.enabled).toBe(true)
  })
})
