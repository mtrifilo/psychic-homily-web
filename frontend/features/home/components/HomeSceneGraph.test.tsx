import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { HomeSceneGraph } from './HomeSceneGraph'
import type { SceneListItem } from '@/features/scenes/types'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

const push = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push }),
}))

const captureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureException(...args),
}))

// The dynamic(ssr:false) chunk never resolves in jsdom's sync render pass —
// stub the module so the section's own logic is what's under test. The
// static-viewport behavior itself is covered in
// components/graph/ForceGraphView.staticViewport.test.tsx.
vi.mock('@/components/graph/ForceGraphView', () => ({
  ForceGraphView: (props: {
    ariaLabel: string
    staticViewport?: boolean
    nodes: Array<{ id: number; name: string; slug: string }>
    onNodeClick: (node: { id: number; name: string; slug: string }) => void
    onBackgroundClick?: () => void
  }) => (
    <div
      data-testid="force-graph-view"
      data-static-viewport={String(props.staticViewport ?? false)}
      aria-label={props.ariaLabel}
    >
      {props.nodes.map(n => (
        <button key={n.id} type="button" onClick={() => props.onNodeClick(n)}>
          {`node-${n.slug}`}
        </button>
      ))}
      <button type="button" onClick={() => props.onBackgroundClick?.()}>
        canvas-background
      </button>
    </div>
  ),
}))

const useScenes = vi.fn()
const useSceneGraph = vi.fn()
// The component deep-imports the hooks module (bundle-size rationale in the
// component) — mock that module, not the '@/features/scenes' barrel.
vi.mock('@/features/scenes/hooks/useScenes', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/scenes/hooks/useScenes')>()
  return {
    ...actual,
    useScenes: () => useScenes(),
    useSceneGraph: (opts: { slug: string; enabled?: boolean }) => useSceneGraph(opts),
  }
})

const useArtistGraphCard = vi.fn()
vi.mock('@/features/artists/hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: (opts: { artistId: number | null; enabled?: boolean }) =>
    useArtistGraphCard(opts),
}))

// Geo default (PSY-1346) — mock so the default-scene pick is deterministic.
// Default: no geo (null) → liveliest scene, the pre-PSY-1346 behavior every
// existing test relies on. Geo-specific tests override the return.
const useGeoDefaultScene = vi.fn()
vi.mock('../hooks/useGeoDefaultScene', () => ({
  useGeoDefaultScene: () => useGeoDefaultScene(),
}))

function scene(overrides: Partial<SceneListItem> & { slug: string }): SceneListItem {
  return {
    city: 'Phoenix',
    state: 'AZ',
    venue_count: 3,
    upcoming_show_count: 4,
    total_show_count: 10,
    shows_this_week: 1,
    ...overrides,
  }
}

const SCENES = [
  scene({ slug: 'chicago-il', city: 'Chicago', state: 'IL', upcoming_show_count: 17 }),
  scene({ slug: 'phoenix-az', upcoming_show_count: 4 }),
]

const GRAPH = {
  scene: {
    slug: 'chicago-il',
    city: 'Chicago',
    state: 'IL',
    artist_count: 2,
    edge_count: 1,
    metro_roster_total: 2,
    roster_truncated: false,
  },
  clusters: [],
  nodes: [
    { id: 1, name: 'Alpha', slug: 'alpha', upcoming_show_count: 1, cluster_id: 'other', is_isolate: false },
    { id: 2, name: 'Beta', slug: 'beta', upcoming_show_count: 0, cluster_id: 'other', is_isolate: false },
  ],
  links: [{ source_id: 1, target_id: 2, type: 'shared_bill' }],
}

// test/setup.ts installs a never-intersecting IntersectionObserver mock —
// replace it (plain writable assignment; defineProperty/stubGlobal can't
// redefine it) with one that intersects immediately so the lazy section
// mounts in tests. ResizeObserver's setup no-op is fine as-is; the initial
// getBoundingClientRect measure drives useContainerWidth.
class ImmediateIntersectionObserver {
  constructor(private cb: IntersectionObserverCallback) {}
  observe() {
    this.cb(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    )
  }
  disconnect() {}
  unobserve() {}
  takeRecords(): IntersectionObserverEntry[] {
    return []
  }
}

function setContainerWidth(px: number) {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    width: px, height: 0, top: 0, left: 0, bottom: 0, right: 0, x: 0, y: 0,
    toJSON: () => ({}),
  } as DOMRect)
}

beforeEach(() => {
  vi.restoreAllMocks()
  push.mockReset()
  captureException.mockReset()
  window.IntersectionObserver =
    ImmediateIntersectionObserver as unknown as typeof IntersectionObserver
  useScenes.mockReset().mockReturnValue({ data: { scenes: SCENES, count: SCENES.length }, isLoading: false, isError: false })
  useSceneGraph.mockReset().mockReturnValue({ data: GRAPH, isLoading: false, isError: false })
  useArtistGraphCard.mockReset().mockReturnValue({ data: undefined, isError: false })
  useGeoDefaultScene.mockReset().mockReturnValue(null)
  setContainerWidth(1024)
})

describe('HomeSceneGraph', () => {
  it('renders the liveliest scene in the heading with the CTA link to its scene page', async () => {
    render(<HomeSceneGraph />)
    expect(
      await screen.findByRole('heading', { name: 'The Chicago scene, mapped' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /explore the full graph/i }),
    ).toHaveAttribute('href', '/scenes/chicago-il')
  })

  it('requests the graph in static-viewport (click-select only) mode with the shared count phrase in the aria-label', async () => {
    render(<HomeSceneGraph />)
    const graph = await screen.findByTestId('force-graph-view')
    expect(graph).toHaveAttribute('data-static-viewport', 'true')
    expect(graph).toHaveAttribute(
      'aria-label',
      expect.stringContaining('Knowledge graph of the Chicago scene: 2 artists'),
    )
  })

  it('defaults to the visitor’s own scene when geo matches one (PSY-1346)', async () => {
    // Chicago is the liveliest, but a Phoenix visitor should land on Phoenix
    // (Phoenix is active — upcoming_show_count 4 — so it clears the floor).
    useGeoDefaultScene.mockReturnValue({ city: 'Phoenix', state: 'AZ' })
    render(<HomeSceneGraph />)
    expect(
      await screen.findByRole('heading', { name: 'The Phoenix scene, mapped' }),
    ).toBeInTheDocument()
  })

  it('lets "Surprise me" win over the geo default (PSY-1346)', async () => {
    // Geo would default to Phoenix; a surprise rotation must still move off it.
    useGeoDefaultScene.mockReturnValue({ city: 'Phoenix', state: 'AZ' })
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'The Phoenix scene, mapped' })
    fireEvent.click(screen.getByRole('button', { name: /surprise me/i }))
    // Only one other scene exists, so the rotation is deterministic.
    expect(
      screen.getByRole('heading', { name: 'The Chicago scene, mapped' }),
    ).toBeInTheDocument()
  })

  it('"Surprise me" rotates to another scene', async () => {
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'The Chicago scene, mapped' })
    fireEvent.click(screen.getByRole('button', { name: /surprise me/i }))
    // Only one other scene exists, so the rotation is deterministic.
    expect(
      screen.getByRole('heading', { name: 'The Phoenix scene, mapped' }),
    ).toBeInTheDocument()
    expect(useSceneGraph).toHaveBeenLastCalledWith(
      expect.objectContaining({ slug: 'phoenix-az' }),
    )
  })

  it('self-hides entirely when the scenes list errors', async () => {
    useScenes.mockReturnValue({ data: undefined, isLoading: false, isError: true })
    const { container } = render(<HomeSceneGraph />)
    expect(screen.queryByRole('heading', { name: /scene, mapped/i })).toBeNull()
    // The wrapper stays (observer target) but carries no section content.
    expect(container.querySelector('section')).toBeNull()
  })

  it('self-hides when the scenes list is empty', () => {
    useScenes.mockReturnValue({ data: { scenes: [], count: 0 }, isLoading: false, isError: false })
    const { container } = render(<HomeSceneGraph />)
    expect(container.querySelector('section')).toBeNull()
  })

  it('renders the small-screen teaser (no canvas) below the graph breakpoint', async () => {
    setContainerWidth(500)
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'The Chicago scene, mapped' })
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
    expect(
      screen.getByRole('link', { name: /see the chicago scene/i }),
    ).toHaveAttribute('href', '/scenes/chicago-il')
  })

  it('treats keepPreviousData placeholder frames as loading — never renders the previous scene’s graph under a new heading', async () => {
    // After "Surprise me", useSceneGraph reports the OLD scene's payload
    // with isPlaceholderData: true until the new fetch settles.
    useSceneGraph.mockReturnValue({
      data: GRAPH, // Chicago's payload…
      isLoading: false,
      isError: false,
      isPlaceholderData: true, // …but stale for the current key
    })
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'The Chicago scene, mapped' })
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
    expect(screen.queryByText(/not enough connected artists/i)).toBeNull()
  })

  it('renders the error card (with a scene-page link) when the graph query fails with no settled data', async () => {
    useSceneGraph.mockReturnValue({ data: undefined, isLoading: false, isError: true })
    render(<HomeSceneGraph />)
    expect(await screen.findByText(/the graph couldn’t load/i)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /see the chicago scene/i }),
    ).toHaveAttribute('href', '/scenes/chicago-il')
  })

  it('keeps the settled canvas when a background refetch of the same scene errors (data retained)', async () => {
    useSceneGraph.mockReturnValue({ data: GRAPH, isLoading: false, isError: true })
    render(<HomeSceneGraph />)
    expect(await screen.findByTestId('force-graph-view')).toBeInTheDocument()
    expect(screen.queryByText(/the graph couldn’t load/i)).toBeNull()
  })

  it('does not fetch the graph payload below the canvas gate (teaser never reads it)', async () => {
    setContainerWidth(500)
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'The Chicago scene, mapped' })
    expect(useSceneGraph).toHaveBeenLastCalledWith(
      expect.objectContaining({ enabled: false }),
    )
  })

  it('node click opens the context panel and fetches that artist’s card; second click deselects (PSY-1345)', async () => {
    useArtistGraphCard.mockReturnValue({
      data: {
        id: 1, name: 'Alpha', slug: 'alpha', city: 'Chicago', state: 'IL',
        next_show: null, labels: [], radio: null,
        connections: { bills: 1, similar: 0, members: 0, radio: 0, shared_labels: 0 },
      },
      isError: false,
    })
    render(<HomeSceneGraph />)
    fireEvent.click(await screen.findByRole('button', { name: 'node-alpha' }))
    expect(screen.getByRole('region', { name: 'About Alpha' })).toBeInTheDocument()
    expect(useArtistGraphCard).toHaveBeenLastCalledWith(
      expect.objectContaining({ artistId: 1, enabled: true }),
    )
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute('href', '/artists/alpha')
    // Second click on the same node puts the panel away.
    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    expect(screen.queryByRole('region', { name: 'About Alpha' })).toBeNull()
  })

  it('returns focus to the canvas wrapper when the panel closes (PSY-1313 pattern)', async () => {
    render(<HomeSceneGraph />)
    fireEvent.click(await screen.findByRole('button', { name: 'node-alpha' }))
    fireEvent.click(screen.getByRole('button', { name: /close details/i }))
    const canvasWrap = screen.getByTestId('force-graph-view').parentElement
    expect(document.activeElement).toBe(canvasWrap)
  })

  it('canvas background click and scene rotation both dismiss the panel (PSY-1345)', async () => {
    render(<HomeSceneGraph />)
    fireEvent.click(await screen.findByRole('button', { name: 'node-alpha' }))
    expect(screen.getByRole('region', { name: 'About Alpha' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'canvas-background' }))
    expect(screen.queryByRole('region', { name: 'About Alpha' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    expect(screen.getByRole('region', { name: 'About Alpha' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /surprise me/i }))
    expect(screen.queryByRole('region', { name: 'About Alpha' })).toBeNull()
  })

  it('SectionErrorBoundary self-hides the section and reports to Sentry when rendering throws', async () => {
    // A hook-level throw stands in for any render/chunk failure inside the
    // section (the App Router surfaces failed dynamic chunks as throws).
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    useScenes.mockImplementation(() => {
      throw new Error('boom')
    })
    const { container } = render(<HomeSceneGraph />)
    expect(container.querySelector('section')).toBeNull()
    expect(captureException).toHaveBeenCalledWith(
      expect.any(Error),
      expect.objectContaining({ tags: { section: 'home-scene-graph' } }),
    )
    spy.mockRestore()
  })

  it('renders the empty-graph fallback instead of a canvas when the scene has no connected artists', async () => {
    useSceneGraph.mockReturnValue({
      data: { ...GRAPH, nodes: [], links: [] },
      isLoading: false,
      isError: false,
    })
    render(<HomeSceneGraph />)
    expect(
      await screen.findByText(/not enough connected artists in chicago/i),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
  })
})
