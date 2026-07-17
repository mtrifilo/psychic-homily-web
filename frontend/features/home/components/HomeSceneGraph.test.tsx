import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { HomeSceneGraph } from './HomeSceneGraph'
import type { SceneListItem } from '@/features/scenes/types'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
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
    nodeLabelStyles?: ReadonlyMap<
      number,
      { fontSize: number; fontWeight: number }
    >
    forceNodeLabels?: boolean
    nodeOverlays?: ReadonlyMap<number, React.ReactNode>
    showAccessibleNodeControls?: boolean
    onNodeClick: (node: { id: number; name: string; slug: string }) => void
    onBackgroundClick?: () => void
    showIsolateShelfLabel?: boolean
    labelTiers?: readonly { fontSize: number }[]
  }) => (
    <div
      data-testid="force-graph-view"
      data-static-viewport={String(props.staticViewport ?? false)}
      data-force-labels={String(props.forceNodeLabels ?? false)}
      data-accessible-node-controls={String(
        props.showAccessibleNodeControls ?? false
      )}
      data-isolate-shelf-label={String(props.showIsolateShelfLabel ?? false)}
      data-has-label-tiers={String(props.labelTiers !== undefined)}
      data-label-sizes={JSON.stringify(
        [...(props.nodeLabelStyles?.values() ?? [])].map(
          style => style.fontSize
        )
      )}
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
      {[...(props.nodeOverlays?.entries() ?? [])].map(([id, overlay]) => (
        <div key={id}>{overlay}</div>
      ))}
    </div>
  ),
}))

const useScenes = vi.fn()
const useSceneGraph = vi.fn()
// The component deep-imports the hooks module (bundle-size rationale in the
// component) — mock that module, not the '@/features/scenes' barrel.
vi.mock('@/features/scenes/hooks/useScenes', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/features/scenes/hooks/useScenes')>()
  return {
    ...actual,
    useScenes: () => useScenes(),
    useSceneGraph: (opts: { slug: string; enabled?: boolean }) =>
      useSceneGraph(opts),
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

function scene(
  overrides: Partial<SceneListItem> & { slug: string }
): SceneListItem {
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
  scene({
    slug: 'chicago-il',
    city: 'Chicago',
    state: 'IL',
    upcoming_show_count: 17,
  }),
  scene({ slug: 'phoenix-az', upcoming_show_count: 4 }),
]

const GRAPH = {
  scene: {
    slug: 'chicago-il',
    city: 'Chicago',
    state: 'IL',
    artist_count: 4,
    edge_count: 2,
    metro_roster_total: 4,
    roster_truncated: false,
  },
  clusters: [],
  // 3 connected nodes (the homepage's MIN_CONNECTED_NODES floor) plus one
  // isolate the teaser must filter out (PSY-1444).
  nodes: [
    {
      id: 1,
      name: 'Alpha',
      slug: 'alpha',
      upcoming_show_count: 1,
      cluster_id: 'other',
      is_isolate: false,
      next_show: {
        id: 91,
        event_date: '2026-07-17T20:00:00Z',
        venue_name: 'Crescent Ballroom',
        venue_city: 'Phoenix',
        venue_state: 'AZ',
        venue_timezone: 'America/Phoenix',
      },
    },
    {
      id: 2,
      name: 'Beta',
      slug: 'beta',
      upcoming_show_count: 0,
      cluster_id: 'other',
      is_isolate: false,
    },
    {
      id: 3,
      name: 'Gamma',
      slug: 'gamma',
      upcoming_show_count: 0,
      cluster_id: 'other',
      is_isolate: false,
    },
    {
      id: 4,
      name: 'Delta',
      slug: 'delta',
      upcoming_show_count: 0,
      cluster_id: 'other',
      is_isolate: true,
    },
  ],
  links: [
    { source_id: 1, target_id: 2, type: 'shared_bill' },
    { source_id: 2, target_id: 3, type: 'shared_bill' },
  ],
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
      this as unknown as IntersectionObserver
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
    width: px,
    height: 0,
    top: 0,
    left: 0,
    bottom: 0,
    right: 0,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect)
}

beforeEach(() => {
  vi.restoreAllMocks()
  push.mockReset()
  captureException.mockReset()
  window.IntersectionObserver =
    ImmediateIntersectionObserver as unknown as typeof IntersectionObserver
  useScenes
    .mockReset()
    .mockReturnValue({
      data: { scenes: SCENES, count: SCENES.length },
      isLoading: false,
      isError: false,
    })
  useSceneGraph
    .mockReset()
    .mockReturnValue({ data: GRAPH, isLoading: false, isError: false })
  useArtistGraphCard
    .mockReset()
    .mockReturnValue({ data: undefined, isError: false })
  useGeoDefaultScene.mockReset().mockReturnValue(null)
  setContainerWidth(1024)
})

describe('HomeSceneGraph', () => {
  it('renders the show-anchored heading, caption, CTA, tiered labels, and headline chip', async () => {
    render(<HomeSceneGraph />)
    expect(
      await screen.findByRole('heading', { name: 'Chicago, this week' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /open the graph/i })
    ).toHaveAttribute('href', '/scenes/chicago-il#graph')
    expect(
      screen.getByText(
        /the 3 most connected artists playing or tied to chicago/i
      )
    ).toBeInTheDocument()
    const graph = await screen.findByTestId('force-graph-view')
    expect(graph).toHaveAttribute('data-force-labels', 'true')
    expect(graph).toHaveAttribute('data-accessible-node-controls', 'true')
    expect(graph).toHaveAttribute('data-label-sizes', '[17,13,11]')
    expect(screen.getByText('Fri · Crescent Ballroom')).toBeInTheDocument()
    expect(screen.getByLabelText('Graph legend')).toHaveTextContent(
      'playing soon'
    )
    expect(screen.getByLabelText('Graph legend')).toHaveTextContent(
      'playable audio'
    )
  })

  it('requests the graph in static-viewport (click-select only) mode with the CONNECTED count in the aria-label', async () => {
    render(<HomeSceneGraph />)
    const graph = await screen.findByTestId('force-graph-view')
    expect(graph).toHaveAttribute('data-static-viewport', 'true')
    // 3, not the payload's artist_count of 4 — the isolate is off-canvas.
    expect(graph).toHaveAttribute(
      'aria-label',
      expect.stringContaining(
        'Knowledge graph of the Chicago scene: 3 connected artists'
      )
    )
  })

  it('filters isolate nodes out of the homepage canvas (PSY-1444)', async () => {
    render(<HomeSceneGraph />)
    await screen.findByTestId('force-graph-view')
    expect(
      screen.getByRole('button', { name: 'node-alpha' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'node-gamma' })
    ).toBeInTheDocument()
    // Delta is is_isolate: true — it must not reach ForceGraphView.
    expect(screen.queryByRole('button', { name: 'node-delta' })).toBeNull()
    // PSY-1454 negative pin: the homepage teaser must NOT opt into the
    // labeled isolate shelf (its payload excludes isolates entirely).
    expect(screen.getByTestId('force-graph-view')).toHaveAttribute(
      'data-isolate-shelf-label',
      'false'
    )
    // PSY-1456 negative pin: the homepage teaser keeps its curated EMBED
    // ladder (17/13/11 via nodeLabelStyles) and must NOT opt into the
    // degree-tiered `labelTiers` prop.
    expect(screen.getByTestId('force-graph-view')).toHaveAttribute(
      'data-has-label-tiers',
      'false'
    )
  })

  it('shows the empty card when the scene has fewer than 3 CONNECTED artists, even with isolates present (PSY-1444)', async () => {
    useSceneGraph.mockReturnValue({
      data: {
        ...GRAPH,
        // 2 connected + 2 isolates: below the MIN_CONNECTED_NODES floor.
        nodes: [
          {
            id: 1,
            name: 'Alpha',
            slug: 'alpha',
            upcoming_show_count: 1,
            cluster_id: 'other',
            is_isolate: false,
          },
          {
            id: 2,
            name: 'Beta',
            slug: 'beta',
            upcoming_show_count: 0,
            cluster_id: 'other',
            is_isolate: false,
          },
          {
            id: 3,
            name: 'Gamma',
            slug: 'gamma',
            upcoming_show_count: 0,
            cluster_id: 'other',
            is_isolate: true,
          },
          {
            id: 4,
            name: 'Delta',
            slug: 'delta',
            upcoming_show_count: 0,
            cluster_id: 'other',
            is_isolate: true,
          },
        ],
        links: [{ source_id: 1, target_id: 2, type: 'shared_bill' }],
      },
      isLoading: false,
      isError: false,
    })
    render(<HomeSceneGraph />)
    expect(
      await screen.findByText(/not enough connected artists in chicago/i)
    ).toBeInTheDocument()
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
    // The legend/payoff caption only rides with the canvas.
    expect(screen.queryByText(/lines connect artists/i)).toBeNull()
  })

  it('defaults to the visitor’s own scene when geo matches one (PSY-1346)', async () => {
    // Chicago is the liveliest, but a Phoenix visitor should land on Phoenix
    // (Phoenix is active — upcoming_show_count 4 — so it clears the floor).
    useGeoDefaultScene.mockReturnValue({ city: 'Phoenix', state: 'AZ' })
    render(<HomeSceneGraph />)
    expect(
      await screen.findByRole('heading', { name: 'Phoenix, this week' })
    ).toBeInTheDocument()
  })

  it('pins the scene the visitor is exploring so a late geo resolution cannot swap it (PSY-1346)', async () => {
    // Cold cache: geo resolves to null first → the liveliest scene (Chicago).
    useGeoDefaultScene.mockReturnValue(null)
    const { rerender } = render(<HomeSceneGraph />)
    fireEvent.click(await screen.findByRole('button', { name: 'node-alpha' }))
    expect(
      screen.getByRole('region', { name: 'About Alpha' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Chicago, this week' })
    ).toBeInTheDocument()

    // Geo resolves LATE to Phoenix — but the visitor already engaged a node, so
    // the scene must stay Chicago and the panel must remain open (the ticket's
    // "geo must never override user interaction" rule).
    useGeoDefaultScene.mockReturnValue({ city: 'Phoenix', state: 'AZ' })
    rerender(<HomeSceneGraph />)
    expect(
      screen.getByRole('heading', { name: 'Chicago, this week' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'About Alpha' })
    ).toBeInTheDocument()
  })

  it('lets "Surprise me" win over the geo default (PSY-1346)', async () => {
    // Geo would default to Phoenix; a surprise rotation must still move off it.
    useGeoDefaultScene.mockReturnValue({ city: 'Phoenix', state: 'AZ' })
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'Phoenix, this week' })
    fireEvent.click(screen.getByRole('button', { name: /surprise me/i }))
    // Only one other scene exists, so the rotation is deterministic.
    expect(
      screen.getByRole('heading', { name: 'Chicago, this week' })
    ).toBeInTheDocument()
  })

  it('"Surprise me" rotates to another scene', async () => {
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'Chicago, this week' })
    fireEvent.click(screen.getByRole('button', { name: /surprise me/i }))
    // Only one other scene exists, so the rotation is deterministic.
    expect(
      screen.getByRole('heading', { name: 'Phoenix, this week' })
    ).toBeInTheDocument()
    expect(useSceneGraph).toHaveBeenLastCalledWith(
      expect.objectContaining({ slug: 'phoenix-az' })
    )
  })

  it('self-hides entirely when the scenes list errors', async () => {
    useScenes.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })
    const { container } = render(<HomeSceneGraph />)
    expect(screen.queryByRole('heading', { name: /this week/i })).toBeNull()
    // The wrapper stays (observer target) but carries no section content.
    expect(container.querySelector('section')).toBeNull()
  })

  it('self-hides when the scenes list is empty', () => {
    useScenes.mockReturnValue({
      data: { scenes: [], count: 0 },
      isLoading: false,
      isError: false,
    })
    const { container } = render(<HomeSceneGraph />)
    expect(container.querySelector('section')).toBeNull()
  })

  it('renders the small-screen teaser (no canvas) below the graph breakpoint', async () => {
    setContainerWidth(500)
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'Chicago, this week' })
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
    expect(
      screen.getByRole('link', { name: /see the chicago scene/i })
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
    await screen.findByRole('heading', { name: 'Chicago, this week' })
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
    expect(screen.queryByText(/not enough connected artists/i)).toBeNull()
  })

  it('renders the error card (with a scene-page link) when the graph query fails with no settled data', async () => {
    useSceneGraph.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    })
    render(<HomeSceneGraph />)
    expect(
      await screen.findByText(/the graph couldn’t load/i)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /see the chicago scene/i })
    ).toHaveAttribute('href', '/scenes/chicago-il')
  })

  it('keeps the settled canvas when a background refetch of the same scene errors (data retained)', async () => {
    useSceneGraph.mockReturnValue({
      data: GRAPH,
      isLoading: false,
      isError: true,
    })
    render(<HomeSceneGraph />)
    expect(await screen.findByTestId('force-graph-view')).toBeInTheDocument()
    expect(screen.queryByText(/the graph couldn’t load/i)).toBeNull()
  })

  it('closes a selected artist panel when a same-scene refresh removes that artist', async () => {
    const { rerender } = render(<HomeSceneGraph />)
    fireEvent.click(await screen.findByRole('button', { name: 'node-alpha' }))
    expect(screen.getByRole('region', { name: 'About Alpha' })).toBeInTheDocument()

    useSceneGraph.mockReturnValue({
      data: {
        ...GRAPH,
        nodes: GRAPH.nodes.filter(node => node.id !== 1),
        links: [{ source_id: 2, target_id: 3, type: 'shared_bill' }],
      },
      isLoading: false,
      isError: false,
    })
    rerender(<HomeSceneGraph />)
    expect(screen.queryByRole('region', { name: 'About Alpha' })).toBeNull()
  })

  it('refreshes selected artist identity from same-scene graph data', async () => {
    const { rerender } = render(<HomeSceneGraph />)
    fireEvent.click(await screen.findByRole('button', { name: 'node-alpha' }))

    useSceneGraph.mockReturnValue({
      data: {
        ...GRAPH,
        nodes: GRAPH.nodes.map(node =>
          node.id === 1
            ? { ...node, name: 'Alpha Renamed', slug: 'alpha-renamed' }
            : node,
        ),
      },
      isLoading: false,
      isError: false,
    })
    rerender(<HomeSceneGraph />)

    expect(
      screen.getByRole('region', { name: 'About Alpha Renamed' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/alpha-renamed',
    )
  })

  it('does not fetch the graph payload below the canvas gate (teaser never reads it)', async () => {
    setContainerWidth(500)
    render(<HomeSceneGraph />)
    await screen.findByRole('heading', { name: 'Chicago, this week' })
    expect(useSceneGraph).toHaveBeenLastCalledWith(
      expect.objectContaining({ enabled: false })
    )
    expect(screen.queryByText(/the 0 most connected artists/i)).toBeNull()
  })

  it('node click opens the context panel and fetches that artist’s card; second click deselects (PSY-1345)', async () => {
    useArtistGraphCard.mockReturnValue({
      data: {
        id: 1,
        name: 'Alpha',
        slug: 'alpha',
        city: 'Chicago',
        state: 'IL',
        bandcamp_embed_url: null,
        spotify: null,
        next_show: null,
        labels: [],
        radio: null,
        connections: {
          bills: 1,
          similar: 0,
          members: 0,
          radio: 0,
          shared_labels: 0,
        },
      },
      isError: false,
    })
    render(<HomeSceneGraph />)
    fireEvent.click(await screen.findByRole('button', { name: 'node-alpha' }))
    expect(
      screen.getByRole('region', { name: 'About Alpha' })
    ).toBeInTheDocument()
    expect(useArtistGraphCard).toHaveBeenLastCalledWith(
      expect.objectContaining({ artistId: 1, enabled: true })
    )
    expect(screen.getByRole('link', { name: /open page/i })).toHaveAttribute(
      'href',
      '/artists/alpha'
    )
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
    expect(
      screen.getByRole('region', { name: 'About Alpha' })
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'canvas-background' }))
    expect(screen.queryByRole('region', { name: 'About Alpha' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'node-alpha' }))
    expect(
      screen.getByRole('region', { name: 'About Alpha' })
    ).toBeInTheDocument()
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
      expect.objectContaining({ tags: { section: 'home-scene-graph' } })
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
      await screen.findByText(/not enough connected artists in chicago/i)
    ).toBeInTheDocument()
    expect(screen.queryByTestId('force-graph-view')).toBeNull()
  })
})
